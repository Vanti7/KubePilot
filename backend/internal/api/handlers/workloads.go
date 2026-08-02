package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/backend/internal/k8sops"
	"github.com/kubepilot/backend/internal/models"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// ClientsetProvider exposes a live Kubernetes client for a cluster with an
// active collector. Implemented by collector.CollectorManager; kept as an
// interface here (like ClusterSyncer below) to avoid a hard dependency from
// the API layer on the collector package.
type ClientsetProvider interface {
	GetClientset(clusterID string) (kubernetes.Interface, bool)
}

// RESTConfigProvider exposes the raw *rest.Config for a cluster with an
// active collector, for callers that need more than the typed clientset
// (e.g. helmops, which builds a discovery client and RESTMapper from it).
// Implemented by collector.CollectorManager.
type RESTConfigProvider interface {
	GetRESTConfig(clusterID string) (*rest.Config, bool)
}

// WorkloadHandler handles workload endpoints.
type WorkloadHandler struct {
	store   *store.Store
	cluster ClusterOps
	logger  *zap.Logger
}

// NewWorkloadHandler creates a new WorkloadHandler.
func NewWorkloadHandler(s *store.Store, cluster ClusterOps, logger *zap.Logger) *WorkloadHandler {
	return &WorkloadHandler{store: s, cluster: cluster, logger: logger}
}

// ListWorkloads returns paginated workloads with optional filters.
// GET /api/v1/workloads
func (h *WorkloadHandler) ListWorkloads(c *gin.Context) {
	filter := store.WorkloadFilter{
		ClusterID:     c.Query("cluster_id"),
		NamespaceID:   c.Query("namespace_id"),
		NamespaceName: c.Query("namespace"),
		Kind:          c.Query("kind"),
		HealthStatus:  c.Query("health_status"),
	}

	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			filter.Limit = n
		}
	}
	if o := c.Query("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil {
			filter.Offset = n
		}
	}

	workloads, total, err := h.store.ListWorkloads(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("list workloads", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list workloads"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":   workloads,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

// GetWorkload returns a single workload with its container images.
// GET /api/v1/workloads/:id
func (h *WorkloadHandler) GetWorkload(c *gin.Context) {
	id := c.Param("id")

	workload, err := h.store.GetWorkload(c.Request.Context(), id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "workload not found"})
			return
		}
		h.logger.Error("get workload", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get workload"})
		return
	}

	images, err := h.store.ListContainerImages(c.Request.Context(), id)
	if err != nil {
		h.logger.Warn("list container images", zap.String("workload_id", id), zap.Error(err))
	}

	c.JSON(http.StatusOK, gin.H{
		"workload": workload,
		"images":   images,
	})
}

type scaleWorkloadRequest struct {
	// A pointer, not a plain int32: 0 is a legitimate replica count (pausing
	// a workload) and must be distinguishable from an omitted field.
	// go-playground/validator's "required" checks pointer-nilness, not the
	// pointed-to value, so this still rejects a missing/omitted field.
	Replicas *int32 `json:"replicas" binding:"required"`
}

// ScaleWorkload changes a Deployment or StatefulSet's replica count.
// PATCH /api/v1/workloads/:id/scale
func (h *WorkloadHandler) ScaleWorkload(c *gin.Context) {
	id := c.Param("id")

	var req scaleWorkloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if *req.Replicas < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "replicas must be >= 0"})
		return
	}

	workload, err := h.store.GetWorkload(c.Request.Context(), id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "workload not found"})
			return
		}
		h.logger.Error("get workload", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get workload"})
		return
	}

	if workload.Kind == models.WorkloadKindDaemonSet {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scale is not supported for DaemonSet"})
		return
	}

	clientset, ok := h.cluster.GetClientset(workload.ClusterID.String())
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cluster is not currently connected"})
		return
	}

	replicas := *req.Replicas
	err = k8sops.ScaleWorkload(c.Request.Context(), clientset, workload.Kind, workload.NamespaceName, workload.Name, replicas)

	status := models.ActionLogStatusSuccess
	details := map[string]interface{}{"replicas": replicas, "kind": workload.Kind, "namespace": workload.NamespaceName, "name": workload.Name}
	if err != nil {
		status = models.ActionLogStatusFailure
		details["error"] = err.Error()
	}
	RecordAction(c, h.store, h.logger, models.ActionLogScale, "workload", id, status, details)

	if err != nil {
		h.logger.Error("scale workload", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to scale workload"})
		return
	}

	// Workloads are only polled every 60s otherwise (no live Watch, unlike
	// namespaces/nodes/secrets) — kick an immediate pass so replicas_desired
	// reflects the change within seconds instead of up to a minute later.
	h.cluster.TriggerSync(workload.ClusterID.String())

	c.JSON(http.StatusOK, gin.H{"id": id, "replicas": replicas})
}

// RestartWorkload triggers a rolling restart of a Deployment, StatefulSet or
// DaemonSet, the same way `kubectl rollout restart` does.
// POST /api/v1/workloads/:id/restart
func (h *WorkloadHandler) RestartWorkload(c *gin.Context) {
	id := c.Param("id")

	workload, err := h.store.GetWorkload(c.Request.Context(), id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "workload not found"})
			return
		}
		h.logger.Error("get workload", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get workload"})
		return
	}

	clientset, ok := h.cluster.GetClientset(workload.ClusterID.String())
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cluster is not currently connected"})
		return
	}

	err = k8sops.RestartWorkload(c.Request.Context(), clientset, workload.Kind, workload.NamespaceName, workload.Name)

	status := models.ActionLogStatusSuccess
	details := map[string]interface{}{"kind": workload.Kind, "namespace": workload.NamespaceName, "name": workload.Name}
	if err != nil {
		status = models.ActionLogStatusFailure
		details["error"] = err.Error()
	}
	RecordAction(c, h.store, h.logger, models.ActionLogRestart, "workload", id, status, details)

	if err != nil {
		h.logger.Error("restart workload", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to restart workload"})
		return
	}

	// So replicas_ready reflects the rollout in progress instead of waiting
	// for the next periodic pass (see the same comment in ScaleWorkload).
	h.cluster.TriggerSync(workload.ClusterID.String())

	c.JSON(http.StatusOK, gin.H{"id": id})
}
