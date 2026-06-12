package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// WorkloadHandler handles workload endpoints.
type WorkloadHandler struct {
	store  *store.Store
	logger *zap.Logger
}

// NewWorkloadHandler creates a new WorkloadHandler.
func NewWorkloadHandler(s *store.Store, logger *zap.Logger) *WorkloadHandler {
	return &WorkloadHandler{store: s, logger: logger}
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
