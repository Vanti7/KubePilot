package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/backend/internal/models"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
)

// OverviewHandler handles the dashboard overview endpoint.
type OverviewHandler struct {
	store  *store.Store
	logger *zap.Logger
}

// NewOverviewHandler creates a new OverviewHandler.
func NewOverviewHandler(s *store.Store, logger *zap.Logger) *OverviewHandler {
	return &OverviewHandler{store: s, logger: logger}
}

// ClusterStatusCount holds per-status counts for clusters.
type ClusterStatusCount struct {
	Total        int `json:"total"`
	Healthy      int `json:"healthy"`
	Degraded     int `json:"degraded"`
	Unreachable  int `json:"unreachable"`
	Unknown      int `json:"unknown"`
}

// FindingsPerCluster holds finding counts per cluster.
type FindingsPerCluster struct {
	ClusterID   string `json:"cluster_id"`
	ClusterName string `json:"cluster_name"`
	Status      string `json:"status"`
	Open        int64  `json:"open"`
	Critical    int64  `json:"critical"`
	High        int64  `json:"high"`
	Medium      int64  `json:"medium"`
}

// DataFreshness tracks when a cluster was last seen.
type DataFreshness struct {
	ClusterID   string     `json:"cluster_id"`
	ClusterName string     `json:"cluster_name"`
	LastSeenAt  *time.Time `json:"last_seen_at"`
	AgeSeconds  int64      `json:"age_seconds"`
	Status      string     `json:"status"`
}

// ClusterResources is a per-cluster resource summary for the dashboard.
type ClusterResources struct {
	ClusterID   string `json:"cluster_id"`
	ClusterName string `json:"cluster_name"`
	store.SystemResourceSummary
}

// OverviewResponse is the full dashboard payload.
type OverviewResponse struct {
	ClusterStatus     ClusterStatusCount   `json:"cluster_status"`
	FindingsSummary   store.FindingSummary `json:"findings_summary"`
	FindingsPerCluster []FindingsPerCluster `json:"findings_per_cluster"`
	TopFindings       []store.FindingWithScore `json:"top_findings"`
	DataFreshness     []DataFreshness      `json:"data_freshness"`
	SystemResources    store.SystemResourceSummary `json:"system_resources"`
	ResourcesPerCluster []ClusterResources         `json:"resources_per_cluster"`
	GeneratedAt       time.Time            `json:"generated_at"`
}

// GetOverview returns an aggregated dashboard view.
// GET /api/v1/overview
func (h *OverviewHandler) GetOverview(c *gin.Context) {
	ctx := c.Request.Context()

	clusters, err := h.store.ListClusters(ctx)
	if err != nil {
		h.logger.Error("overview: list clusters", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch clusters"})
		return
	}

	// Cluster status counts.
	clusterStatus := ClusterStatusCount{Total: len(clusters)}
	clusterIDs := make([]string, 0, len(clusters))
	for _, cl := range clusters {
		clusterIDs = append(clusterIDs, cl.ID.String())
		switch cl.Status {
		case models.ClusterStatusHealthy:
			clusterStatus.Healthy++
		case models.ClusterStatusDegraded:
			clusterStatus.Degraded++
		case models.ClusterStatusUnreachable:
			clusterStatus.Unreachable++
		default:
			clusterStatus.Unknown++
		}
	}

	// Global findings summary.
	summary, err := h.store.GetFindingSummary(ctx, clusterIDs)
	if err != nil {
		h.logger.Error("overview: finding summary", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch findings summary"})
		return
	}

	// Per-cluster finding counts.
	findingsPerCluster := make([]FindingsPerCluster, 0, len(clusters))
	for _, cl := range clusters {
		s, err := h.store.GetFindingSummary(ctx, []string{cl.ID.String()})
		if err != nil {
			h.logger.Warn("overview: per-cluster summary", zap.String("cluster", cl.Name), zap.Error(err))
			continue
		}
		findingsPerCluster = append(findingsPerCluster, FindingsPerCluster{
			ClusterID:   cl.ID.String(),
			ClusterName: cl.Name,
			Status:      string(cl.Status),
			Open:        s.Total,
			Critical:    s.Critical,
			High:        s.High,
			Medium:      s.Medium,
		})
	}

	// Top 5 critical findings.
	topFindings, err := h.store.TopCriticalFindings(ctx, 5)
	if err != nil {
		h.logger.Error("overview: top findings", zap.Error(err))
		topFindings = []store.FindingWithScore{}
	}

	// Data freshness.
	freshness := make([]DataFreshness, 0, len(clusters))
	now := time.Now()
	for _, cl := range clusters {
		df := DataFreshness{
			ClusterID:   cl.ID.String(),
			ClusterName: cl.Name,
			LastSeenAt:  cl.LastSeenAt,
			Status:      "stale",
		}
		if cl.LastSeenAt != nil {
			df.AgeSeconds = int64(now.Sub(*cl.LastSeenAt).Seconds())
			if df.AgeSeconds < 600 { // fresh within 10 minutes
				df.Status = "fresh"
			} else if df.AgeSeconds < 1800 { // 30 minutes
				df.Status = "aging"
			}
		}
		freshness = append(freshness, df)
	}

	// Cluster-wide system resources (capacity vs live usage), global + per-cluster.
	systemResources, err := h.store.SystemResources(ctx, "")
	if err != nil {
		h.logger.Error("overview: system resources", zap.Error(err))
		systemResources = store.SystemResourceSummary{}
	}

	resourcesPerCluster := make([]ClusterResources, 0, len(clusters))
	for _, cl := range clusters {
		res, err := h.store.SystemResources(ctx, cl.ID.String())
		if err != nil {
			h.logger.Warn("overview: per-cluster resources", zap.String("cluster", cl.Name), zap.Error(err))
			continue
		}
		resourcesPerCluster = append(resourcesPerCluster, ClusterResources{
			ClusterID:             cl.ID.String(),
			ClusterName:           cl.Name,
			SystemResourceSummary: res,
		})
	}

	c.JSON(http.StatusOK, OverviewResponse{
		ClusterStatus:       clusterStatus,
		FindingsSummary:     summary,
		FindingsPerCluster:  findingsPerCluster,
		TopFindings:         topFindings,
		DataFreshness:       freshness,
		SystemResources:     systemResources,
		ResourcesPerCluster: resourcesPerCluster,
		GeneratedAt:         now,
	})
}
