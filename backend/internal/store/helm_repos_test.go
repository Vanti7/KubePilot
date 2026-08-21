package store

import (
	"context"
	"testing"

	"github.com/kubepilot/backend/internal/models"
)

func TestHelmRepositoryCRUD(t *testing.T) {
	s := newFullTestStore(t)
	ctx := context.Background()

	repo := &models.HelmRepository{Name: "bitnami", URL: "https://charts.bitnami.com/bitnami"}
	if err := s.CreateHelmRepository(ctx, repo); err != nil {
		t.Fatalf("create: %v", err)
	}

	t.Run("get by id", func(t *testing.T) {
		got, err := s.GetHelmRepository(ctx, repo.ID.String())
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Name != "bitnami" {
			t.Errorf("Name = %q, want bitnami", got.Name)
		}
	})

	t.Run("get by url", func(t *testing.T) {
		got, err := s.GetHelmRepositoryByURL(ctx, "https://charts.bitnami.com/bitnami")
		if err != nil {
			t.Fatalf("get by url: %v", err)
		}
		if got.ID != repo.ID {
			t.Errorf("got %s, want %s", got.ID, repo.ID)
		}
	})

	t.Run("get by url, unknown returns an error", func(t *testing.T) {
		if _, err := s.GetHelmRepositoryByURL(ctx, "https://unknown.example.com"); err == nil {
			t.Error("expected an error for an unconfigured URL")
		}
	})

	t.Run("update", func(t *testing.T) {
		repo.TLSInsecure = true
		if err := s.UpdateHelmRepository(ctx, repo); err != nil {
			t.Fatalf("update: %v", err)
		}
		got, _ := s.GetHelmRepository(ctx, repo.ID.String())
		if !got.TLSInsecure {
			t.Error("expected TLSInsecure to persist")
		}
	})

	t.Run("list orders by name", func(t *testing.T) {
		if err := s.CreateHelmRepository(ctx, &models.HelmRepository{Name: "argo", URL: "https://argoproj.github.io/argo-helm"}); err != nil {
			t.Fatalf("create second repo: %v", err)
		}
		repos, err := s.ListHelmRepositories(ctx)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(repos) != 2 || repos[0].Name != "argo" {
			t.Fatalf("expected alphabetical order, got %v", repos)
		}
	})
}

// TestDeleteHelmRepository_ClearsRepoURLFromReleases guards the documented
// behavior: deleting a repository must clear repo_url on any release that
// resolved to it, so the watcher re-resolves it next cycle instead of
// leaving releases pointing at a repository that no longer exists.
func TestDeleteHelmRepository_ClearsRepoURLFromReleases(t *testing.T) {
	s := newFullTestStore(t)
	ctx := context.Background()
	cluster := mustCreateCluster(t, s, "c1")

	repo := &models.HelmRepository{Name: "bitnami", URL: "https://charts.bitnami.com/bitnami"}
	if err := s.CreateHelmRepository(ctx, repo); err != nil {
		t.Fatalf("create repo: %v", err)
	}

	release := &models.HelmRelease{ClusterID: cluster.ID, NamespaceName: "default", Name: "redis", ChartName: "redis", ChartVersion: "1.0.0"}
	if err := s.UpsertHelmRelease(ctx, release); err != nil {
		t.Fatalf("create release: %v", err)
	}
	if err := s.SetHelmReleaseRepoURL(ctx, release.ID, repo.URL); err != nil {
		t.Fatalf("set repo url: %v", err)
	}

	if err := s.DeleteHelmRepository(ctx, repo.ID.String()); err != nil {
		t.Fatalf("delete repo: %v", err)
	}

	got, err := s.GetHelmRelease(ctx, release.ID.String())
	if err != nil {
		t.Fatalf("get release: %v", err)
	}
	if got.RepoURL != "" {
		t.Errorf("RepoURL = %q, want cleared after the repository was deleted", got.RepoURL)
	}
}
