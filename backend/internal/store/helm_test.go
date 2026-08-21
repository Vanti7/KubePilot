package store

import (
	"context"
	"testing"
	"time"

	"github.com/kubepilot/backend/internal/models"
)

// TestUpsertHelmRelease_PreservesRepoURL_OnConflict guards the exact
// regression documented on UpsertHelmRelease itself: repo_url is resolved by
// the Helm watcher, not the collector, so a re-collection pass (which never
// knows the repo URL) must not wipe out what the watcher already resolved.
func TestUpsertHelmRelease_PreservesRepoURL_OnConflict(t *testing.T) {
	s := newFullTestStore(t)
	ctx := context.Background()
	cluster := mustCreateCluster(t, s, "c1")

	release := &models.HelmRelease{
		ClusterID: cluster.ID, NamespaceName: "default", Name: "redis",
		ChartName: "redis", ChartVersion: "18.6.1", Revision: 1,
	}
	if err := s.UpsertHelmRelease(ctx, release); err != nil {
		t.Fatalf("initial collector upsert: %v", err)
	}

	// The watcher resolves the repository out of band.
	if err := s.SetHelmReleaseRepoURL(ctx, release.ID, "https://charts.bitnami.com/bitnami"); err != nil {
		t.Fatalf("set repo url: %v", err)
	}

	// A later collector pass builds a fresh struct — it has no idea what the
	// repo URL is, so it's zero-valued here, exactly like the real caller.
	next := &models.HelmRelease{
		ClusterID: cluster.ID, NamespaceName: "default", Name: "redis",
		ChartName: "redis", ChartVersion: "18.7.0", Revision: 2,
	}
	if err := s.UpsertHelmRelease(ctx, next); err != nil {
		t.Fatalf("second collector upsert: %v", err)
	}

	got, err := s.GetHelmRelease(ctx, release.ID.String())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.RepoURL != "https://charts.bitnami.com/bitnami" {
		t.Errorf("RepoURL = %q, want it preserved across the collector's re-upsert", got.RepoURL)
	}
	if got.ChartVersion != "18.7.0" {
		t.Errorf("ChartVersion = %q, want the second pass's value (18.7.0)", got.ChartVersion)
	}
}

func TestListHelmReleases_Filters(t *testing.T) {
	s := newFullTestStore(t)
	ctx := context.Background()
	cluster := mustCreateCluster(t, s, "c1")

	seed := []models.HelmRelease{
		{ClusterID: cluster.ID, NamespaceName: "default", Name: "redis", ChartName: "redis", ChartVersion: "1.0.0", Status: "deployed"},
		{ClusterID: cluster.ID, NamespaceName: "default", Name: "postgres", ChartName: "postgres", ChartVersion: "1.0.0", Status: "failed"},
		{ClusterID: cluster.ID, NamespaceName: "monitoring", Name: "prometheus", ChartName: "prometheus", ChartVersion: "1.0.0", Status: "deployed"},
	}
	for i := range seed {
		if err := s.UpsertHelmRelease(ctx, &seed[i]); err != nil {
			t.Fatalf("seed %s: %v", seed[i].Name, err)
		}
	}

	t.Run("by namespace", func(t *testing.T) {
		releases, total, err := s.ListHelmReleases(ctx, HelmFilter{NamespaceName: "default"})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 2 || len(releases) != 2 {
			t.Fatalf("total=%d len=%d, want 2/2", total, len(releases))
		}
		if releases[0].Cluster == nil {
			t.Error("expected Cluster preloaded")
		}
	})

	t.Run("by status", func(t *testing.T) {
		_, total, err := s.ListHelmReleases(ctx, HelmFilter{Status: "failed"})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 1 {
			t.Fatalf("total = %d, want 1", total)
		}
	})

	t.Run("GetHelmReleaseByClusterAndName", func(t *testing.T) {
		got, err := s.GetHelmReleaseByClusterAndName(ctx, cluster.ID.String(), "monitoring", "prometheus")
		if err != nil {
			t.Fatalf("get by cluster+name: %v", err)
		}
		if got.ChartName != "prometheus" {
			t.Errorf("ChartName = %q, want prometheus", got.ChartName)
		}
	})

	t.Run("ListAllHelmReleases returns everything regardless of filter", func(t *testing.T) {
		all, err := s.ListAllHelmReleases(ctx)
		if err != nil {
			t.Fatalf("list all: %v", err)
		}
		if len(all) != 3 {
			t.Fatalf("expected 3 releases total, got %d", len(all))
		}
	})
}

func TestDeleteHelmReleasesNotSeenSince(t *testing.T) {
	s := newFullTestStore(t)
	ctx := context.Background()
	cluster := mustCreateCluster(t, s, "c1")

	stale := &models.HelmRelease{ClusterID: cluster.ID, NamespaceName: "default", Name: "stale", ChartName: "x", ChartVersion: "1.0"}
	if err := s.UpsertHelmRelease(ctx, stale); err != nil {
		t.Fatalf("upsert stale: %v", err)
	}
	old := time.Now().Add(-1 * time.Hour)
	if err := s.DB.Model(&models.HelmRelease{}).Where("id = ?", stale.ID).Update("last_seen_at", old).Error; err != nil {
		t.Fatalf("backdate: %v", err)
	}

	fresh := &models.HelmRelease{ClusterID: cluster.ID, NamespaceName: "default", Name: "fresh", ChartName: "y", ChartVersion: "1.0"}
	if err := s.UpsertHelmRelease(ctx, fresh); err != nil {
		t.Fatalf("upsert fresh: %v", err)
	}

	if err := s.DeleteHelmReleasesNotSeenSince(ctx, cluster.ID.String(), time.Now().Add(-1*time.Minute)); err != nil {
		t.Fatalf("delete stale: %v", err)
	}

	all, _ := s.ListAllHelmReleases(ctx)
	if len(all) != 1 || all[0].Name != "fresh" {
		t.Errorf("expected only 'fresh' to survive, got %v", all)
	}
}
