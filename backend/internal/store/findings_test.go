package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
	"go.uber.org/zap"
)

func newFindingsTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := OpenSQLite(dbPath, false, zap.NewNop())
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Cluster{},
		&models.Workload{},
		&models.ContainerImage{},
		&models.HelmRelease{},
		&models.UpdateFinding{},
		&models.RiskScore{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	t.Cleanup(func() {
		// Windows can't remove the TempDir's db file on cleanup while the
		// pool still holds it open.
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return NewStore(db, nil, zap.NewNop())
}

func mustCreateCluster(t *testing.T, s *Store, name string) *models.Cluster {
	t.Helper()
	c := &models.Cluster{ID: uuid.New(), Name: name, Slug: name}
	if err := s.DB.Create(c).Error; err != nil {
		t.Fatalf("create cluster %s: %v", name, err)
	}
	return c
}

func mustCreateContainerImage(t *testing.T, s *Store, clusterID uuid.UUID, containerName string) (*models.Workload, *models.ContainerImage) {
	t.Helper()
	w := &models.Workload{
		ID: uuid.New(), ClusterID: clusterID, NamespaceName: "default",
		Name: "api-" + containerName, Kind: "Deployment",
	}
	if err := s.DB.Create(w).Error; err != nil {
		t.Fatalf("create workload: %v", err)
	}
	ci := &models.ContainerImage{
		ID: uuid.New(), WorkloadID: w.ID, ContainerName: containerName,
		Image: "nginx:1.24", Registry: "docker.io", Repository: "nginx", Tag: "1.24",
	}
	if err := s.DB.Create(ci).Error; err != nil {
		t.Fatalf("create container image: %v", err)
	}
	return w, ci
}

// TestUpsertFinding_PreservesFirstDetectedAt_OnConflict guards the exact
// mechanism behind the age scoring factor: re-observing an already-known
// image update must refresh latest_version/severity/title but must NOT push
// first_detected_at forward, or the "how overdue is this" signal resets on
// every collection pass.
func TestUpsertFinding_PreservesFirstDetectedAt_OnConflict(t *testing.T) {
	s := newFindingsTestStore(t)
	ctx := context.Background()

	cluster := mustCreateCluster(t, s, "c1")
	_, ci := mustCreateContainerImage(t, s, cluster.ID, "app")

	firstSeen := time.Now().Add(-30 * 24 * time.Hour)
	f1 := &models.UpdateFinding{
		ClusterID: cluster.ID, ContainerImageID: &ci.ID, Kind: models.FindingKindImage,
		UpdateType: models.UpdateTypeMinor, Severity: models.SeverityLow, Status: models.FindingStatusOpen,
		CurrentVersion: "1.24", LatestVersion: "1.25", Title: "v1",
		FirstDetectedAt: firstSeen,
	}
	if err := s.UpsertFinding(ctx, f1); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	// Second observation: a newer tag has since appeared, and the caller
	// (the watcher) sets FirstDetectedAt to "now" just like the first call —
	// it must be ignored on conflict.
	f2 := &models.UpdateFinding{
		ClusterID: cluster.ID, ContainerImageID: &ci.ID, Kind: models.FindingKindImage,
		UpdateType: models.UpdateTypeMajor, Severity: models.SeverityHigh, Status: models.FindingStatusOpen,
		CurrentVersion: "1.24", LatestVersion: "2.0", Title: "v2",
		FirstDetectedAt: time.Now(),
	}
	if err := s.UpsertFinding(ctx, f2); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	var count int64
	if err := s.DB.Model(&models.UpdateFinding{}).Where("container_image_id = ?", ci.ID).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 finding row after two upserts, got %d", count)
	}

	var reloaded models.UpdateFinding
	if err := s.DB.Where("container_image_id = ?", ci.ID).First(&reloaded).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !reloaded.FirstDetectedAt.Equal(firstSeen) {
		t.Errorf("FirstDetectedAt = %v, want unchanged from first upsert (%v)", reloaded.FirstDetectedAt, firstSeen)
	}
	if reloaded.LatestVersion != "2.0" || reloaded.Severity != models.SeverityHigh || reloaded.Title != "v2" {
		t.Errorf("expected fields from the second upsert to win, got LatestVersion=%q Severity=%q Title=%q",
			reloaded.LatestVersion, reloaded.Severity, reloaded.Title)
	}
}

func TestResolveActiveFindingForImage(t *testing.T) {
	s := newFindingsTestStore(t)
	ctx := context.Background()

	cluster := mustCreateCluster(t, s, "c1")
	_, openImage := mustCreateContainerImage(t, s, cluster.ID, "open-app")
	_, ignoredImage := mustCreateContainerImage(t, s, cluster.ID, "ignored-app")

	openFinding := &models.UpdateFinding{
		ClusterID: cluster.ID, ContainerImageID: &openImage.ID, Kind: models.FindingKindImage,
		UpdateType: models.UpdateTypePatch, Severity: models.SeverityLow, Status: models.FindingStatusOpen,
		CurrentVersion: "1.0", LatestVersion: "1.1", Title: "open",
	}
	if err := s.UpsertFinding(ctx, openFinding); err != nil {
		t.Fatalf("create open finding: %v", err)
	}
	ignoredFinding := &models.UpdateFinding{
		ClusterID: cluster.ID, ContainerImageID: &ignoredImage.ID, Kind: models.FindingKindImage,
		UpdateType: models.UpdateTypePatch, Severity: models.SeverityLow, Status: models.FindingStatusIgnored,
		CurrentVersion: "1.0", LatestVersion: "1.1", Title: "ignored",
	}
	if err := s.UpsertFinding(ctx, ignoredFinding); err != nil {
		t.Fatalf("create ignored finding: %v", err)
	}

	if err := s.ResolveActiveFindingForImage(ctx, openImage.ID); err != nil {
		t.Fatalf("resolve open image: %v", err)
	}
	if err := s.ResolveActiveFindingForImage(ctx, ignoredImage.ID); err != nil {
		t.Fatalf("resolve ignored image: %v", err)
	}

	var reloadedOpen models.UpdateFinding
	s.DB.First(&reloadedOpen, "id = ?", openFinding.ID)
	if reloadedOpen.Status != models.FindingStatusResolved {
		t.Errorf("open finding status = %q, want %q", reloadedOpen.Status, models.FindingStatusResolved)
	}
	if reloadedOpen.ResolvedAt == nil {
		t.Error("open finding ResolvedAt is nil, want set")
	}

	var reloadedIgnored models.UpdateFinding
	s.DB.First(&reloadedIgnored, "id = ?", ignoredFinding.ID)
	if reloadedIgnored.Status != models.FindingStatusIgnored {
		t.Errorf("ignored finding status changed to %q, want it to stay %q (not an active status)",
			reloadedIgnored.Status, models.FindingStatusIgnored)
	}
}

func TestIsActiveFindingStatus(t *testing.T) {
	cases := map[string]bool{
		models.FindingStatusOpen:     true,
		models.FindingStatusPlanned:  true,
		models.FindingStatusApproved: true,
		models.FindingStatusIgnored:  false,
		models.FindingStatusBlocked:  false,
		models.FindingStatusResolved: false,
		"":                           false,
		"bogus":                      false,
	}
	for status, want := range cases {
		if got := IsActiveFindingStatus(status); got != want {
			t.Errorf("IsActiveFindingStatus(%q) = %v, want %v", status, got, want)
		}
	}
}

func TestListFindings_FiltersAndJoins(t *testing.T) {
	s := newFindingsTestStore(t)
	ctx := context.Background()

	cluster := mustCreateCluster(t, s, "c1")
	workload, ci := mustCreateContainerImage(t, s, cluster.ID, "app")

	imageFinding := &models.UpdateFinding{
		ClusterID: cluster.ID, ContainerImageID: &ci.ID, WorkloadID: &workload.ID, Kind: models.FindingKindImage,
		UpdateType: models.UpdateTypeMajor, Severity: models.SeverityHigh, Status: models.FindingStatusOpen,
		CurrentVersion: "1.0", LatestVersion: "2.0", Title: "image finding",
	}
	if err := s.UpsertFinding(ctx, imageFinding); err != nil {
		t.Fatalf("create image finding: %v", err)
	}

	release := &models.HelmRelease{
		ID: uuid.New(), ClusterID: cluster.ID, NamespaceName: "default", Name: "redis",
		ChartName: "redis", ChartVersion: "18.6.1",
	}
	if err := s.DB.Create(release).Error; err != nil {
		t.Fatalf("create helm release: %v", err)
	}
	helmFinding := &models.UpdateFinding{
		ClusterID: cluster.ID, HelmReleaseID: &release.ID, Kind: models.FindingKindHelm,
		UpdateType: models.UpdateTypeMinor, Severity: models.SeverityMedium, Status: models.FindingStatusPlanned,
		CurrentVersion: "18.6.1", LatestVersion: "18.7.0", Title: "helm finding",
	}
	if err := s.UpsertFinding(ctx, helmFinding); err != nil {
		t.Fatalf("create helm finding: %v", err)
	}

	t.Run("no filter returns both with total", func(t *testing.T) {
		_, total, err := s.ListFindings(ctx, FindingFilter{})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 2 {
			t.Fatalf("total = %d, want 2", total)
		}
	})

	t.Run("kind=image joins workload name", func(t *testing.T) {
		rows, total, err := s.ListFindings(ctx, FindingFilter{Kind: models.FindingKindImage})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 1 || len(rows) != 1 {
			t.Fatalf("total=%d len=%d, want 1/1", total, len(rows))
		}
		if rows[0].WorkloadName != workload.Name {
			t.Errorf("WorkloadName = %q, want %q", rows[0].WorkloadName, workload.Name)
		}
		if rows[0].ContainerName != "app" {
			t.Errorf("ContainerName = %q, want %q", rows[0].ContainerName, "app")
		}
	})

	t.Run("kind=helm joins helm release name", func(t *testing.T) {
		rows, total, err := s.ListFindings(ctx, FindingFilter{Kind: models.FindingKindHelm})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 1 || len(rows) != 1 {
			t.Fatalf("total=%d len=%d, want 1/1", total, len(rows))
		}
		if rows[0].HelmReleaseName != release.Name {
			t.Errorf("HelmReleaseName = %q, want %q", rows[0].HelmReleaseName, release.Name)
		}
	})

	t.Run("status filter", func(t *testing.T) {
		_, total, err := s.ListFindings(ctx, FindingFilter{Status: models.FindingStatusPlanned})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 1 {
			t.Fatalf("total = %d, want 1", total)
		}
	})

	t.Run("severity filter", func(t *testing.T) {
		_, total, err := s.ListFindings(ctx, FindingFilter{Severity: models.SeverityHigh})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 1 {
			t.Fatalf("total = %d, want 1", total)
		}
	})
}
