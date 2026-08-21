package scoring

import (
	"context"
	"encoding/json"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
	"gorm.io/datatypes"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.OpenSQLite(dbPath, false, zap.NewNop())
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Environment{},
		&models.Cluster{},
		&models.Workload{},
		&models.UpdateFinding{},
		&models.RiskScore{},
		&models.MaintenanceWindow{},
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
	return store.NewStore(db, nil, zap.NewNop())
}

func newFinding(t *testing.T, s *store.Store, clusterID uuid.UUID, updateType string, firstDetected time.Time, status string) *models.UpdateFinding {
	t.Helper()
	f := &models.UpdateFinding{
		ID:              uuid.New(),
		ClusterID:       clusterID,
		Kind:            models.FindingKindImage,
		UpdateType:      updateType,
		Severity:        models.SeverityInfo,
		Status:          status,
		CurrentVersion:  "1.0.0",
		LatestVersion:   "2.0.0",
		Title:           "test finding",
		FirstDetectedAt: firstDetected,
		LastObservedAt:  time.Now(),
	}
	if err := s.DB.WithContext(context.Background()).Create(f).Error; err != nil {
		t.Fatalf("create finding: %v", err)
	}
	return f
}

func TestScoreToSeverity(t *testing.T) {
	cases := []struct {
		score float64
		want  string
	}{
		{0, models.SeverityInfo},
		{19.9, models.SeverityInfo},
		{20, models.SeverityLow},
		{39.9, models.SeverityLow},
		{40, models.SeverityMedium},
		{59.9, models.SeverityMedium},
		{60, models.SeverityHigh},
		{79.9, models.SeverityHigh},
		{80, models.SeverityCritical},
		{100, models.SeverityCritical},
	}
	for _, tc := range cases {
		if got := scoreToSeverity(tc.score); got != tc.want {
			t.Errorf("scoreToSeverity(%v) = %q, want %q", tc.score, got, tc.want)
		}
	}
}

// TestScoreFinding_Defaults pins the behavior with no workload and no cluster:
// every factor that reads from workload/cluster must fall back to its
// documented default (docs/scoring.md §4) rather than zero out silently.
func TestScoreFinding_Defaults(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	se := NewScoringEngine(s, zap.NewNop())

	clusterID := uuid.New()
	finding := newFinding(t, s, clusterID, models.UpdateTypeMajor, time.Now().Add(-10*24*time.Hour), models.FindingStatusOpen)

	rs, err := se.ScoreFinding(ctx, finding, nil, nil)
	if err != nil {
		t.Fatalf("ScoreFinding: %v", err)
	}

	var factors ScoreFactors
	if err := json.Unmarshal(rs.Factors, &factors); err != nil {
		t.Fatalf("unmarshal factors: %v", err)
	}

	if factors.UpdateType != 25 {
		t.Errorf("UpdateType factor = %v, want 25 (major)", factors.UpdateType)
	}
	if factors.Age != 8 {
		t.Errorf("Age factor = %v, want 8 (7-30 days)", factors.Age)
	}
	if factors.ServiceCriticality != 5 {
		t.Errorf("ServiceCriticality factor = %v, want 5 (default medium, no workload)", factors.ServiceCriticality)
	}
	if factors.HealthStatus != 0 {
		t.Errorf("HealthStatus factor = %v, want 0 (no workload)", factors.HealthStatus)
	}
	if factors.RollbackPossible != 3 {
		t.Errorf("RollbackPossible factor = %v, want 3 (default unknown, no workload)", factors.RollbackPossible)
	}
	if factors.MaintenanceWindowActive != 5 {
		t.Errorf("MaintenanceWindowActive factor = %v, want 5 (default outside, no cluster)", factors.MaintenanceWindowActive)
	}
	if factors.EnvMultiplier != 1.0 {
		t.Errorf("EnvMultiplier = %v, want 1.0 (no cluster)", factors.EnvMultiplier)
	}
	if factors.ExposureMultiplier != 1.0 {
		t.Errorf("ExposureMultiplier = %v, want 1.0 (no workload)", factors.ExposureMultiplier)
	}

	// weighted = 25*.25 + 8*.20 + 5*.15 + 0*.10 + 3*.05 + 5*.05 + 0*.20 = 9.0
	// score = 9.0 / 21.6 * 100 = 41.7 (medium)
	wantScore := math.Round((9.0/maxPossibleSum)*100*10) / 10
	if math.Abs(rs.Score-wantScore) > 0.01 {
		t.Errorf("Score = %v, want %v", rs.Score, wantScore)
	}
	if rs.Severity != models.SeverityMedium {
		t.Errorf("Severity = %q, want %q", rs.Severity, models.SeverityMedium)
	}

	// The finding row itself must be updated to match, not just the RiskScore.
	var reloaded models.UpdateFinding
	if err := s.DB.WithContext(ctx).First(&reloaded, "id = ?", finding.ID).Error; err != nil {
		t.Fatalf("reload finding: %v", err)
	}
	if reloaded.Severity != models.SeverityMedium {
		t.Errorf("finding.Severity in DB = %q, want %q", reloaded.Severity, models.SeverityMedium)
	}
}

// TestScoreFinding_WorkloadAnnotationsAndEnvironment exercises the
// annotation-driven overrides and the environment/exposure multipliers
// together — this is the combination the "production, externally-exposed,
// critical service" case in docs/scoring.md §8 relies on.
func TestScoreFinding_WorkloadAnnotationsAndEnvironment(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	se := NewScoringEngine(s, zap.NewNop())

	env := &models.Environment{ID: uuid.New(), Name: "Development", Slug: "dev", CriticalityWeight: 0.3}
	if err := s.DB.Create(env).Error; err != nil {
		t.Fatalf("create environment: %v", err)
	}
	cluster := &models.Cluster{ID: uuid.New(), Name: "c1", Slug: "c1", EnvironmentID: &env.ID, Environment: env}
	if err := s.DB.Create(cluster).Error; err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	annotations, _ := json.Marshal(map[string]string{
		"kubepilot/criticality": "critical",
		"kubepilot/rollback":    "true",
		"kubepilot/exposure":    "external",
	})
	workload := &models.Workload{
		ID:            uuid.New(),
		ClusterID:     cluster.ID,
		NamespaceName: "default",
		Name:          "api",
		Kind:          "Deployment",
		HealthStatus:  models.WorkloadHealthDegraded,
		Annotations:   datatypes.JSON(annotations),
	}
	if err := s.DB.Create(workload).Error; err != nil {
		t.Fatalf("create workload: %v", err)
	}

	finding := newFinding(t, s, cluster.ID, models.UpdateTypePatch, time.Now().Add(-100*24*time.Hour), models.FindingStatusOpen)

	rs, err := se.ScoreFinding(ctx, finding, workload, cluster)
	if err != nil {
		t.Fatalf("ScoreFinding: %v", err)
	}

	var factors ScoreFactors
	if err := json.Unmarshal(rs.Factors, &factors); err != nil {
		t.Fatalf("unmarshal factors: %v", err)
	}

	if factors.UpdateType != 5 {
		t.Errorf("UpdateType factor = %v, want 5 (patch)", factors.UpdateType)
	}
	if factors.Age != 20 {
		t.Errorf("Age factor = %v, want 20 (>90 days)", factors.Age)
	}
	if factors.ServiceCriticality != 15 {
		t.Errorf("ServiceCriticality factor = %v, want 15 (critical annotation)", factors.ServiceCriticality)
	}
	if factors.HealthStatus != 10 {
		t.Errorf("HealthStatus factor = %v, want 10 (degraded)", factors.HealthStatus)
	}
	if factors.RollbackPossible != 0 {
		t.Errorf("RollbackPossible factor = %v, want 0 (rollback=true annotation)", factors.RollbackPossible)
	}
	if factors.EnvMultiplier != 0.3 {
		t.Errorf("EnvMultiplier = %v, want 0.3 (dev environment)", factors.EnvMultiplier)
	}
	if factors.ExposureMultiplier != 1.2 {
		t.Errorf("ExposureMultiplier = %v, want 1.2 (external annotation)", factors.ExposureMultiplier)
	}

	// weighted = 5*.25 + 20*.20 + 15*.15 + 10*.10 + 0*.05 + 5*.05 + 0*.20 = 8.75
	// preNorm = 8.75 * 0.3 * 1.2 = 3.15 ; score = 3.15/21.6*100 = 14.6 (info)
	wantScore := math.Round((3.15/maxPossibleSum)*100*10) / 10
	if math.Abs(rs.Score-wantScore) > 0.01 {
		t.Errorf("Score = %v, want %v", rs.Score, wantScore)
	}
	if rs.Severity != models.SeverityInfo {
		t.Errorf("Severity = %q, want %q (dev multiplier should suppress urgency despite degraded health)", rs.Severity, models.SeverityInfo)
	}
}

// TestScoreFinding_MaintenanceWindowPlaceholder pins a known limitation:
// isWindowActive is a hardcoded placeholder that always returns false (see
// its doc comment), so an active MaintenanceWindow row currently has no
// effect on the score. If isWindowActive is ever implemented for real, this
// test should start failing and needs updating alongside it — that's the
// point of pinning it explicitly rather than leaving it unverified.
func TestScoreFinding_MaintenanceWindowPlaceholder(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	se := NewScoringEngine(s, zap.NewNop())

	cluster := &models.Cluster{ID: uuid.New(), Name: "c1", Slug: "c1"}
	if err := s.DB.Create(cluster).Error; err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	window := &models.MaintenanceWindow{
		ID:        uuid.New(),
		ClusterID: &cluster.ID,
		Name:      "always-on test window",
		CronExpr:  "* * * * *",
		IsActive:  true,
	}
	if err := s.DB.Create(window).Error; err != nil {
		t.Fatalf("create maintenance window: %v", err)
	}

	finding := newFinding(t, s, cluster.ID, models.UpdateTypeMajor, time.Now(), models.FindingStatusOpen)
	rs, err := se.ScoreFinding(ctx, finding, nil, cluster)
	if err != nil {
		t.Fatalf("ScoreFinding: %v", err)
	}

	var factors ScoreFactors
	if err := json.Unmarshal(rs.Factors, &factors); err != nil {
		t.Fatalf("unmarshal factors: %v", err)
	}
	if factors.MaintenanceWindowActive != 5 {
		t.Errorf("MaintenanceWindowActive = %v, want 5 (isWindowActive is a stub that always returns false)", factors.MaintenanceWindowActive)
	}
}

// TestScoreAll_SkipsInactiveFindings verifies ScoreAll only scores findings
// still in an active status (open/planned/approved) — a resolved or ignored
// finding must not get a fresh RiskScore row.
func TestScoreAll_SkipsInactiveFindings(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	se := NewScoringEngine(s, zap.NewNop())

	clusterID := uuid.New()
	open := newFinding(t, s, clusterID, models.UpdateTypeMinor, time.Now(), models.FindingStatusOpen)
	planned := newFinding(t, s, clusterID, models.UpdateTypeMinor, time.Now(), models.FindingStatusPlanned)
	resolved := newFinding(t, s, clusterID, models.UpdateTypeMinor, time.Now(), models.FindingStatusResolved)

	if err := se.ScoreAll(ctx); err != nil {
		t.Fatalf("ScoreAll: %v", err)
	}

	for _, f := range []*models.UpdateFinding{open, planned} {
		rs, err := s.GetRiskScore(ctx, f.ID.String())
		if err != nil {
			t.Fatalf("get risk score for %s: %v", f.Status, err)
		}
		if rs == nil {
			t.Errorf("expected a risk score for %s finding, got none", f.Status)
		}
	}

	rs, err := s.GetRiskScore(ctx, resolved.ID.String())
	if err != nil {
		t.Fatalf("get risk score for resolved: %v", err)
	}
	if rs != nil {
		t.Errorf("expected no risk score for a resolved finding, got %+v", rs)
	}
}
