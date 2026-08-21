package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
)

func TestClusterCRUD(t *testing.T) {
	s := newFullTestStore(t)
	ctx := context.Background()

	env, err := s.GetOrCreateEnvironment(ctx, "Production", "production")
	if err != nil {
		t.Fatalf("get or create environment: %v", err)
	}

	cluster := &models.Cluster{Name: "b-cluster", Slug: "b-cluster", EnvironmentID: &env.ID}
	if err := s.CreateCluster(ctx, cluster); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	if cluster.ID == uuid.Nil {
		t.Fatal("expected CreateCluster to assign an ID")
	}

	other := &models.Cluster{Name: "a-cluster", Slug: "a-cluster"}
	if err := s.CreateCluster(ctx, other); err != nil {
		t.Fatalf("create other cluster: %v", err)
	}

	t.Run("get preloads environment", func(t *testing.T) {
		got, err := s.GetCluster(ctx, cluster.ID.String())
		if err != nil {
			t.Fatalf("get cluster: %v", err)
		}
		if got.Environment == nil || got.Environment.Slug != "production" {
			t.Errorf("expected Environment preloaded with slug production, got %+v", got.Environment)
		}
	})

	t.Run("list orders by name", func(t *testing.T) {
		clusters, err := s.ListClusters(ctx)
		if err != nil {
			t.Fatalf("list clusters: %v", err)
		}
		if len(clusters) != 2 || clusters[0].Name != "a-cluster" || clusters[1].Name != "b-cluster" {
			t.Fatalf("unexpected order: %v", clusters)
		}
	})

	t.Run("get by slug", func(t *testing.T) {
		got, err := s.GetClusterBySlug(ctx, "b-cluster")
		if err != nil {
			t.Fatalf("get by slug: %v", err)
		}
		if got.ID != cluster.ID {
			t.Errorf("got cluster %s, want %s", got.ID, cluster.ID)
		}
	})

	t.Run("update persists all fields", func(t *testing.T) {
		cluster.Region = "eu-west-1"
		cluster.K8sVersion = "v1.30.2"
		if err := s.UpdateCluster(ctx, cluster); err != nil {
			t.Fatalf("update cluster: %v", err)
		}
		got, _ := s.GetCluster(ctx, cluster.ID.String())
		if got.Region != "eu-west-1" || got.K8sVersion != "v1.30.2" {
			t.Errorf("update did not persist: %+v", got)
		}
	})

	t.Run("update status", func(t *testing.T) {
		now := time.Now().Truncate(time.Second)
		if err := s.UpdateClusterStatus(ctx, cluster.ID.String(), models.ClusterStatusHealthy, now); err != nil {
			t.Fatalf("update status: %v", err)
		}
		got, _ := s.GetCluster(ctx, cluster.ID.String())
		if got.Status != models.ClusterStatusHealthy {
			t.Errorf("Status = %q, want %q", got.Status, models.ClusterStatusHealthy)
		}
		if got.LastSeenAt == nil || got.LastSeenAt.Unix() != now.Unix() {
			t.Errorf("LastSeenAt = %v, want %v", got.LastSeenAt, now)
		}
	})

	t.Run("update k8s version only", func(t *testing.T) {
		if err := s.UpdateK8sVersion(ctx, cluster.ID.String(), "v1.31.0"); err != nil {
			t.Fatalf("update k8s version: %v", err)
		}
		got, _ := s.GetCluster(ctx, cluster.ID.String())
		if got.K8sVersion != "v1.31.0" {
			t.Errorf("K8sVersion = %q, want v1.31.0", got.K8sVersion)
		}
		if got.Region != "eu-west-1" {
			t.Errorf("UpdateK8sVersion should not touch other fields, Region = %q", got.Region)
		}
	})

	t.Run("delete", func(t *testing.T) {
		if err := s.DeleteCluster(ctx, other.ID.String()); err != nil {
			t.Fatalf("delete cluster: %v", err)
		}
		clusters, _ := s.ListClusters(ctx)
		if len(clusters) != 1 {
			t.Fatalf("expected 1 cluster after delete, got %d", len(clusters))
		}
	})
}

func TestListClustersForEnv(t *testing.T) {
	s := newFullTestStore(t)
	ctx := context.Background()

	prod, _ := s.GetOrCreateEnvironment(ctx, "Production", "production")
	dev, _ := s.GetOrCreateEnvironment(ctx, "Development", "dev")

	mustCreateClusterWithEnv(t, s, "prod-1", &prod.ID)
	mustCreateClusterWithEnv(t, s, "prod-2", &prod.ID)
	mustCreateClusterWithEnv(t, s, "dev-1", &dev.ID)

	prodClusters, err := s.ListClustersForEnv(ctx, prod.ID.String())
	if err != nil {
		t.Fatalf("list for env: %v", err)
	}
	if len(prodClusters) != 2 {
		t.Fatalf("expected 2 prod clusters, got %d", len(prodClusters))
	}
}

func TestGetOrCreateEnvironment(t *testing.T) {
	s := newFullTestStore(t)
	ctx := context.Background()

	env1, err := s.GetOrCreateEnvironment(ctx, "Staging", "staging")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	env2, err := s.GetOrCreateEnvironment(ctx, "Staging (renamed ignored)", "staging")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if env1.ID != env2.ID {
		t.Errorf("expected the same environment row, got %s and %s", env1.ID, env2.ID)
	}

	envs, err := s.ListEnvironments(ctx)
	if err != nil {
		t.Fatalf("list environments: %v", err)
	}
	if len(envs) != 1 {
		t.Fatalf("expected exactly 1 environment (no duplicate), got %d", len(envs))
	}
}

func TestUpsertNamespace(t *testing.T) {
	s := newFullTestStore(t)
	ctx := context.Background()
	cluster := mustCreateCluster(t, s, "c1")

	ns := &models.Namespace{ClusterID: cluster.ID, Name: "payments", Status: "Active"}
	if err := s.UpsertNamespace(ctx, ns); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	firstID := ns.ID

	ns2 := &models.Namespace{ClusterID: cluster.ID, Name: "payments", Status: "Terminating"}
	if err := s.UpsertNamespace(ctx, ns2); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	if ns2.ID != firstID {
		t.Errorf("expected the same namespace row (id %s), got %s", firstID, ns2.ID)
	}

	got, err := s.GetNamespaceByName(ctx, cluster.ID.String(), "payments")
	if err != nil {
		t.Fatalf("get by name: %v", err)
	}
	if got == nil || got.Status != "Terminating" {
		t.Errorf("expected status Terminating after upsert, got %+v", got)
	}

	missing, err := s.GetNamespaceByName(ctx, cluster.ID.String(), "does-not-exist")
	if err != nil {
		t.Fatalf("get missing namespace should not error: %v", err)
	}
	if missing != nil {
		t.Errorf("expected nil for a missing namespace, got %+v", missing)
	}
}

// TestUpsertNode_StablePrimaryKeyAcrossPasses guards the exact class of bug
// UpsertNode's own doc comment describes fixing: "a plain FirstOrCreate
// duplicated rows because the freshly-generated ID poisoned the lookup
// condition" — same shape as the historical UpsertWorkload incident
// (workloads_test.go), applied here to nodes.
func TestUpsertNode_StablePrimaryKeyAcrossPasses(t *testing.T) {
	s := newFullTestStore(t)
	ctx := context.Background()
	cluster := mustCreateCluster(t, s, "c1")

	n1 := &models.Node{ID: uuid.New(), ClusterID: cluster.ID, Name: "node-1", Status: "Ready"}
	if err := s.UpsertNode(ctx, n1); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	firstID := n1.ID

	n2 := &models.Node{ID: uuid.New(), ClusterID: cluster.ID, Name: "node-1", Status: "NotReady"}
	if n2.ID == firstID {
		t.Fatal("test setup: expected a different random ID for the second pass")
	}
	if err := s.UpsertNode(ctx, n2); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	if n2.ID != firstID {
		t.Errorf("UpsertNode left n2.ID = %s, want it rewritten to %s", n2.ID, firstID)
	}

	nodes, err := s.ListNodes(ctx, cluster.ID.String())
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected exactly 1 node after two upserts, got %d", len(nodes))
	}
	if nodes[0].Status != "NotReady" {
		t.Errorf("Status = %q, want NotReady (second pass's value)", nodes[0].Status)
	}
}

func TestGetNode(t *testing.T) {
	s := newFullTestStore(t)
	ctx := context.Background()
	cluster := mustCreateCluster(t, s, "c1")
	node := &models.Node{ID: uuid.New(), ClusterID: cluster.ID, Name: "node-1"}
	if err := s.UpsertNode(ctx, node); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := s.GetNode(ctx, node.ID.String())
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if got.Cluster == nil || got.Cluster.ID != cluster.ID {
		t.Errorf("expected Cluster preloaded, got %+v", got.Cluster)
	}
}

func TestDeleteNodesNotSeenSince(t *testing.T) {
	s := newFullTestStore(t)
	ctx := context.Background()
	cluster := mustCreateCluster(t, s, "c1")

	stale := &models.Node{ID: uuid.New(), ClusterID: cluster.ID, Name: "stale"}
	if err := s.UpsertNode(ctx, stale); err != nil {
		t.Fatalf("upsert stale: %v", err)
	}
	// Force an old updated_at so the cutoff below reliably catches it.
	old := time.Now().Add(-1 * time.Hour)
	if err := s.DB.Model(&models.Node{}).Where("id = ?", stale.ID).Update("updated_at", old).Error; err != nil {
		t.Fatalf("backdate stale node: %v", err)
	}

	fresh := &models.Node{ID: uuid.New(), ClusterID: cluster.ID, Name: "fresh"}
	if err := s.UpsertNode(ctx, fresh); err != nil {
		t.Fatalf("upsert fresh: %v", err)
	}

	cutoff := time.Now().Add(-1 * time.Minute)
	if err := s.DeleteNodesNotSeenSince(ctx, cluster.ID.String(), cutoff); err != nil {
		t.Fatalf("delete stale: %v", err)
	}

	nodes, _ := s.ListNodes(ctx, cluster.ID.String())
	if len(nodes) != 1 || nodes[0].Name != "fresh" {
		t.Errorf("expected only 'fresh' to survive, got %v", nodes)
	}
}

func mustCreateClusterWithEnv(t *testing.T, s *Store, name string, envID *uuid.UUID) *models.Cluster {
	t.Helper()
	c := &models.Cluster{Name: name, Slug: name, EnvironmentID: envID}
	if err := s.CreateCluster(context.Background(), c); err != nil {
		t.Fatalf("create cluster %s: %v", name, err)
	}
	return c
}
