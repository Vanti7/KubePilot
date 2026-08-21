package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
)

func mustCreateNode(t *testing.T, s *Store, clusterID uuid.UUID, name, status, capCPU, capMem string) *models.Node {
	t.Helper()
	n := &models.Node{
		ID: uuid.New(), ClusterID: clusterID, Name: name, Status: status,
		CapacityCPU: capCPU, CapacityMemory: capMem,
	}
	if err := s.UpsertNode(context.Background(), n); err != nil {
		t.Fatalf("create node %s: %v", name, err)
	}
	return n
}

func TestInsertAndListNodeMetrics(t *testing.T) {
	s := newFullTestStore(t)
	ctx := context.Background()
	cluster := mustCreateCluster(t, s, "c1")
	node := mustCreateNode(t, s, cluster.ID, "node-1", "Ready", "4", "8Gi")

	base := time.Now().Add(-1 * time.Hour)
	for i, offset := range []time.Duration{0, 10 * time.Minute, 20 * time.Minute} {
		m := &models.NodeMetric{
			ID: uuid.New(), ClusterID: cluster.ID, NodeID: node.ID, NodeName: node.Name,
			Timestamp: base.Add(offset), CPUUsageNanoCores: int64(i+1) * 1_000_000_000,
		}
		if err := s.InsertNodeMetric(ctx, m); err != nil {
			t.Fatalf("insert metric %d: %v", i, err)
		}
	}

	metrics, err := s.ListNodeMetrics(ctx, node.ID.String(), base.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(metrics) != 2 {
		t.Fatalf("expected 2 metrics after the 'since' cutoff, got %d", len(metrics))
	}
	if !metrics[0].Timestamp.Before(metrics[1].Timestamp) {
		t.Error("expected metrics ordered oldest first")
	}
}

func TestLatestNodeMetricsByCluster(t *testing.T) {
	s := newFullTestStore(t)
	ctx := context.Background()
	c1 := mustCreateCluster(t, s, "c1")
	c2 := mustCreateCluster(t, s, "c2")
	n1 := mustCreateNode(t, s, c1.ID, "n1", "Ready", "4", "8Gi")
	n2 := mustCreateNode(t, s, c2.ID, "n2", "Ready", "2", "4Gi")

	now := time.Now()
	seed := []struct {
		node     *models.Node
		cluster  uuid.UUID
		ts       time.Time
		cpuCores int64
	}{
		{n1, c1.ID, now.Add(-20 * time.Minute), 1},
		{n1, c1.ID, now.Add(-10 * time.Minute), 2}, // latest for n1
		{n2, c2.ID, now.Add(-5 * time.Minute), 3},  // latest for n2
	}
	for _, sd := range seed {
		m := &models.NodeMetric{
			ID: uuid.New(), ClusterID: sd.cluster, NodeID: sd.node.ID, NodeName: sd.node.Name,
			Timestamp: sd.ts, CPUUsageNanoCores: sd.cpuCores * 1_000_000_000,
		}
		if err := s.InsertNodeMetric(ctx, m); err != nil {
			t.Fatalf("insert metric: %v", err)
		}
	}

	t.Run("across all clusters", func(t *testing.T) {
		latest, err := s.LatestNodeMetricsByCluster(ctx, "")
		if err != nil {
			t.Fatalf("latest: %v", err)
		}
		if len(latest) != 2 {
			t.Fatalf("expected latest sample for 2 nodes, got %d", len(latest))
		}
		if latest[n1.ID.String()].CPUUsageNanoCores != 2_000_000_000 {
			t.Errorf("n1 latest = %d, want the -10min sample (2 cores), not the -20min one",
				latest[n1.ID.String()].CPUUsageNanoCores)
		}
	})

	t.Run("filtered by cluster", func(t *testing.T) {
		latest, err := s.LatestNodeMetricsByCluster(ctx, c2.ID.String())
		if err != nil {
			t.Fatalf("latest: %v", err)
		}
		if len(latest) != 1 {
			t.Fatalf("expected 1 node for c2, got %d", len(latest))
		}
		if _, ok := latest[n1.ID.String()]; ok {
			t.Error("n1 (cluster c1) should not appear when filtering by c2")
		}
	})
}

func TestDeleteNodeMetricsBefore(t *testing.T) {
	s := newFullTestStore(t)
	ctx := context.Background()
	cluster := mustCreateCluster(t, s, "c1")
	node := mustCreateNode(t, s, cluster.ID, "n1", "Ready", "4", "8Gi")

	old := &models.NodeMetric{ID: uuid.New(), ClusterID: cluster.ID, NodeID: node.ID, NodeName: node.Name, Timestamp: time.Now().Add(-48 * time.Hour)}
	recent := &models.NodeMetric{ID: uuid.New(), ClusterID: cluster.ID, NodeID: node.ID, NodeName: node.Name, Timestamp: time.Now()}
	if err := s.InsertNodeMetric(ctx, old); err != nil {
		t.Fatalf("insert old: %v", err)
	}
	if err := s.InsertNodeMetric(ctx, recent); err != nil {
		t.Fatalf("insert recent: %v", err)
	}

	if err := s.DeleteNodeMetricsBefore(ctx, time.Now().Add(-24*time.Hour)); err != nil {
		t.Fatalf("purge: %v", err)
	}

	remaining, err := s.ListNodeMetrics(ctx, node.ID.String(), time.Now().Add(-72*time.Hour))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(remaining) != 1 || remaining[0].ID != recent.ID {
		t.Errorf("expected only the recent sample to survive retention, got %v", remaining)
	}
}

func TestSystemResources(t *testing.T) {
	s := newFullTestStore(t)
	ctx := context.Background()
	cluster := mustCreateCluster(t, s, "c1")

	ready := mustCreateNode(t, s, cluster.ID, "ready-node", "Ready", "4", "8Gi")
	mustCreateNode(t, s, cluster.ID, "notready-node", "NotReady", "4", "8Gi")
	// A node with no metric sample yet must not crash SystemResources.
	mustCreateNode(t, s, cluster.ID, "uncollected-node", "Ready", "2", "4Gi")

	m := &models.NodeMetric{
		ID: uuid.New(), ClusterID: cluster.ID, NodeID: ready.ID, NodeName: ready.Name,
		Timestamp: time.Now(), CPUUsageNanoCores: 2_000_000_000, // 2 of 4 cores
		MemoryWorkingSetBytes: 4 * 1024 * 1024 * 1024, // 4 of 8Gi
		PodsRunning:           12,
	}
	if err := s.InsertNodeMetric(ctx, m); err != nil {
		t.Fatalf("insert metric: %v", err)
	}

	sum, err := s.SystemResources(ctx, cluster.ID.String())
	if err != nil {
		t.Fatalf("system resources: %v", err)
	}

	if sum.Nodes != 3 {
		t.Errorf("Nodes = %d, want 3", sum.Nodes)
	}
	if sum.NodesReady != 2 {
		t.Errorf("NodesReady = %d, want 2 (NotReady node excluded)", sum.NodesReady)
	}
	// Capacity sums across all 3 nodes (4+4+2 cores = 10), usage only from the one sample.
	if sum.CPUCapacityCores != 10 {
		t.Errorf("CPUCapacityCores = %v, want 10", sum.CPUCapacityCores)
	}
	if sum.CPUUsedCores != 2 {
		t.Errorf("CPUUsedCores = %v, want 2", sum.CPUUsedCores)
	}
	if sum.CPUUsagePercent != 20 {
		t.Errorf("CPUUsagePercent = %v, want 20 (2/10)", sum.CPUUsagePercent)
	}
	if sum.PodsRunning != 12 {
		t.Errorf("PodsRunning = %d, want 12", sum.PodsRunning)
	}
}
