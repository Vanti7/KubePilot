package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/backend/internal/k8sops"
	"github.com/kubepilot/backend/internal/models"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ManifestHandler handles raw-manifest deploy endpoints.
type ManifestHandler struct {
	store   *store.Store
	cluster ClusterOps
	logger  *zap.Logger
}

// NewManifestHandler creates a new ManifestHandler.
func NewManifestHandler(s *store.Store, cluster ClusterOps, logger *zap.Logger) *ManifestHandler {
	return &ManifestHandler{store: s, cluster: cluster, logger: logger}
}

type applyManifestRequest struct {
	Manifest  string `json:"manifest" binding:"required"`
	Namespace string `json:"namespace"`
	DryRun    bool   `json:"dry_run"`
	Force     bool   `json:"force"`
}

// ApplyManifest server-side applies a (possibly multi-document) YAML
// manifest against the target cluster.
// POST /api/v1/clusters/:id/manifests/apply
func (h *ManifestHandler) ApplyManifest(c *gin.Context) {
	id := c.Param("id")

	var req applyManifestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if _, err := h.store.GetCluster(c.Request.Context(), id); err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "cluster not found"})
			return
		}
		h.logger.Error("get cluster", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get cluster"})
		return
	}

	restConfig, ok := h.cluster.GetRESTConfig(id)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cluster is not currently connected"})
		return
	}

	dynamicClient, mapper, err := k8sops.BuildDynamicClient(restConfig)
	if err != nil {
		h.logger.Error("build dynamic client", zap.String("cluster_id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build cluster client"})
		return
	}

	results, err := k8sops.ApplyManifest(c.Request.Context(), dynamicClient, mapper, req.Manifest, req.Namespace, req.DryRun, req.Force)
	if err != nil {
		// A structurally invalid manifest — nothing was sent to the cluster.
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	allSucceeded := true
	for _, r := range results {
		if r.Operation == k8sops.ApplyOperationError {
			allSucceeded = false
			break
		}
	}

	if !req.DryRun {
		status := models.ActionLogStatusSuccess
		if !allSucceeded {
			status = models.ActionLogStatusFailure
		}
		details := map[string]interface{}{"results": results, "namespace": req.Namespace}
		RecordAction(c, h.store, h.logger, models.ActionLogDeployManifest, "cluster", id, status, details)

		// Same gotcha as scale/restart and Helm upgrade/rollback: workloads
		// and Helm releases are only re-polled every 60s, no live Watch —
		// kick an immediate pass so a deployed workload shows up in seconds.
		h.cluster.TriggerSync(id)
	}

	c.JSON(http.StatusOK, gin.H{"results": results, "dry_run": req.DryRun, "ok": allSucceeded})
}
