package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
)

func TestListNamespaces(t *testing.T) {
	s := newFullTestStore(t)
	ctx := context.Background()

	c1 := mustCreateCluster(t, s, "c1")
	c2 := mustCreateCluster(t, s, "c2")

	for _, n := range []struct {
		cluster uuid.UUID
		name    string
	}{
		{c1.ID, "zeta"}, {c1.ID, "alpha"}, {c2.ID, "beta"},
	} {
		ns := &models.Namespace{ID: uuid.New(), ClusterID: n.cluster, Name: n.name}
		if err := s.DB.Create(ns).Error; err != nil {
			t.Fatalf("create namespace %s: %v", n.name, err)
		}
	}

	t.Run("filtered by cluster, ordered by name", func(t *testing.T) {
		namespaces, err := s.ListNamespaces(ctx, c1.ID.String())
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(namespaces) != 2 || namespaces[0].Name != "alpha" || namespaces[1].Name != "zeta" {
			t.Fatalf("unexpected result: %v", namespaces)
		}
	})

	t.Run("no filter returns all clusters' namespaces", func(t *testing.T) {
		namespaces, err := s.ListNamespaces(ctx, "")
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(namespaces) != 3 {
			t.Fatalf("expected 3 namespaces total, got %d", len(namespaces))
		}
	})
}
