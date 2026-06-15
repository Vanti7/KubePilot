package store

import (
	"context"
	"math"
	"time"

	"github.com/kubepilot/backend/internal/models"
	"k8s.io/apimachinery/pkg/api/resource"
)

// SystemResourceSummary aggregates capacity and live usage across a set of nodes,
// for the dashboard's cluster-wide resource view (à la Proxmox/vCenter).
type SystemResourceSummary struct {
	Nodes               int     `json:"nodes"`
	NodesReady          int     `json:"nodes_ready"`
	PodsRunning         int     `json:"pods_running"`
	CPUCapacityCores    float64 `json:"cpu_capacity_cores"`
	CPUUsedCores        float64 `json:"cpu_used_cores"`
	CPUUsagePercent     float64 `json:"cpu_usage_percent"`
	MemoryCapacityBytes int64   `json:"memory_capacity_bytes"`
	MemoryUsedBytes     int64   `json:"memory_used_bytes"`
	MemoryUsagePercent  float64 `json:"memory_usage_percent"`
	DiskCapacityBytes   int64   `json:"disk_capacity_bytes"`
	DiskUsedBytes       int64   `json:"disk_used_bytes"`
	DiskUsagePercent    float64 `json:"disk_usage_percent"`
}

// InsertNodeMetric appends a single node metric sample (time-series, never upserted).
func (s *Store) InsertNodeMetric(ctx context.Context, m *models.NodeMetric) error {
	return s.DB.WithContext(ctx).Create(m).Error
}

// ListNodeMetrics returns the samples for a node since the given time, oldest first.
func (s *Store) ListNodeMetrics(ctx context.Context, nodeID string, since time.Time) ([]models.NodeMetric, error) {
	var metrics []models.NodeMetric
	err := s.DB.WithContext(ctx).
		Where("node_id = ? AND timestamp >= ?", nodeID, since).
		Order("timestamp ASC").
		Find(&metrics).Error
	return metrics, err
}

// LatestNodeMetricsByCluster returns the most recent sample for each node in a
// cluster, keyed by node ID. An empty clusterID returns the latest sample for
// every node across all clusters.
func (s *Store) LatestNodeMetricsByCluster(ctx context.Context, clusterID string) (map[string]models.NodeMetric, error) {
	var metrics []models.NodeMetric
	// Sub-query selects the max timestamp per node; the outer query then fetches
	// the matching rows. Works identically on PostgreSQL and SQLite.
	sub := s.DB.WithContext(ctx).
		Model(&models.NodeMetric{}).
		Select("node_id, MAX(timestamp) AS ts").
		Group("node_id")
	if clusterID != "" {
		sub = sub.Where("cluster_id = ?", clusterID)
	}

	q := s.DB.WithContext(ctx).
		Joins("JOIN (?) latest ON latest.node_id = node_metrics.node_id AND latest.ts = node_metrics.timestamp", sub)
	if err := q.Find(&metrics).Error; err != nil {
		return nil, err
	}

	out := make(map[string]models.NodeMetric, len(metrics))
	for _, m := range metrics {
		out[m.NodeID.String()] = m
	}
	return out, nil
}

// DeleteNodeMetricsBefore purges samples older than the given cutoff (retention).
func (s *Store) DeleteNodeMetricsBefore(ctx context.Context, before time.Time) error {
	return s.DB.WithContext(ctx).
		Where("timestamp < ?", before).
		Delete(&models.NodeMetric{}).Error
}

// SystemResources aggregates node capacity and the latest usage sample into a
// cluster-wide resource summary. An empty clusterID aggregates every cluster.
func (s *Store) SystemResources(ctx context.Context, clusterID string) (SystemResourceSummary, error) {
	nodes, err := s.ListNodes(ctx, clusterID)
	if err != nil {
		return SystemResourceSummary{}, err
	}
	latest, err := s.LatestNodeMetricsByCluster(ctx, clusterID)
	if err != nil {
		return SystemResourceSummary{}, err
	}

	sum := SystemResourceSummary{Nodes: len(nodes)}
	for _, n := range nodes {
		if n.Status == "Ready" {
			sum.NodesReady++
		}
		sum.CPUCapacityCores += parseQuantityCores(n.CapacityCPU)
		sum.MemoryCapacityBytes += parseQuantityBytes(n.CapacityMemory)

		m, ok := latest[n.ID.String()]
		if !ok {
			continue
		}
		sum.CPUUsedCores += float64(m.CPUUsageNanoCores) / 1e9
		sum.MemoryUsedBytes += m.MemoryWorkingSetBytes
		sum.DiskUsedBytes += m.FSUsedBytes
		sum.DiskCapacityBytes += m.FSCapacityBytes
		sum.PodsRunning += m.PodsRunning
	}

	if sum.CPUCapacityCores > 0 {
		sum.CPUUsagePercent = round2(sum.CPUUsedCores / sum.CPUCapacityCores * 100)
	}
	if sum.MemoryCapacityBytes > 0 {
		sum.MemoryUsagePercent = round2(float64(sum.MemoryUsedBytes) / float64(sum.MemoryCapacityBytes) * 100)
	}
	if sum.DiskCapacityBytes > 0 {
		sum.DiskUsagePercent = round2(float64(sum.DiskUsedBytes) / float64(sum.DiskCapacityBytes) * 100)
	}
	return sum, nil
}

// parseQuantityCores converts a Kubernetes CPU quantity (e.g. "4", "500m") to cores.
func parseQuantityCores(q string) float64 {
	if q == "" {
		return 0
	}
	qty, err := resource.ParseQuantity(q)
	if err != nil {
		return 0
	}
	return qty.AsApproximateFloat64()
}

// parseQuantityBytes converts a Kubernetes memory quantity (e.g. "16331252Ki") to bytes.
func parseQuantityBytes(q string) int64 {
	if q == "" {
		return 0
	}
	qty, err := resource.ParseQuantity(q)
	if err != nil {
		return 0
	}
	return qty.Value()
}

func round2(f float64) float64 {
	return math.Round(f*100) / 100
}
