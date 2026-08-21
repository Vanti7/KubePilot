package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
)

// TestUpsertWorkload_StablePrimaryKeyAcrossPasses is a regression guard for
// the 2026-08-01 incident (see CHANGELOG "Aucun finding d'image n'était
// jamais créé"): ON CONFLICT DO UPDATE does not report the surviving row's
// id back to Go, so a collector pass that builds a fresh Workload struct
// (fresh random ID, no memory of the previous pass) must still end up
// pointing at the one real row — otherwise every container_images row
// attached by this pass targets a workload ID that doesn't exist.
func TestUpsertWorkload_StablePrimaryKeyAcrossPasses(t *testing.T) {
	s := newFindingsTestStore(t)
	ctx := context.Background()
	cluster := mustCreateCluster(t, s, "c1")

	w1 := &models.Workload{
		ID: uuid.New(), ClusterID: cluster.ID, NamespaceName: "default", Name: "api", Kind: "Deployment",
		ReplicasDesired: 1, ReplicasReady: 1,
	}
	if err := s.UpsertWorkload(ctx, w1); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	firstID := w1.ID

	// A second collection pass: same logical workload, but a brand-new
	// in-memory struct with its own random ID, exactly like a fresh
	// collector.List() result.
	w2 := &models.Workload{
		ID: uuid.New(), ClusterID: cluster.ID, NamespaceName: "default", Name: "api", Kind: "Deployment",
		ReplicasDesired: 3, ReplicasReady: 3,
	}
	if w2.ID == firstID {
		t.Fatal("test setup: expected a different random ID for the second pass")
	}
	if err := s.UpsertWorkload(ctx, w2); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	if w2.ID != firstID {
		t.Errorf("UpsertWorkload left w2.ID = %s, want it rewritten to the existing row's %s", w2.ID, firstID)
	}

	var count int64
	if err := s.DB.Model(&models.Workload{}).
		Where("cluster_id = ? AND namespace_name = ? AND name = ? AND kind = ?", cluster.ID, "default", "api", "Deployment").
		Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 workload row after two upserts, got %d", count)
	}

	var reloaded models.Workload
	if err := s.DB.First(&reloaded, "id = ?", firstID).Error; err != nil {
		t.Fatalf("reload by original id: %v", err)
	}
	if reloaded.ReplicasDesired != 3 {
		t.Errorf("ReplicasDesired = %d, want 3 (second pass's value should have applied to the original row)", reloaded.ReplicasDesired)
	}
}

func TestUpsertWorkload_NewWorkloadGetsAssignedID(t *testing.T) {
	s := newFindingsTestStore(t)
	ctx := context.Background()
	cluster := mustCreateCluster(t, s, "c1")

	w := &models.Workload{ClusterID: cluster.ID, NamespaceName: "default", Name: "fresh", Kind: "Deployment"}
	if err := s.UpsertWorkload(ctx, w); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if w.ID == uuid.Nil {
		t.Error("expected UpsertWorkload to assign an ID for a brand-new workload")
	}
}
