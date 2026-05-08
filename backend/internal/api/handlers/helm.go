package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// HelmHandler handles Helm release endpoints.
type HelmHandler struct {
	store  *store.Store
	logger *zap.Logger
}

// NewHelmHandler creates a new HelmHandler.
func NewHelmHandler(s *store.Store, logger *zap.Logger) *HelmHandler {
	return &HelmHandler{store: s, logger: logger}
}

// ListHelmReleases returns paginated Helm releases.
// GET /api/v1/helm
func (h *HelmHandler) ListHelmReleases(c *gin.Context) {
	filter := store.HelmFilter{
		ClusterID:     c.Query("cluster_id"),
		NamespaceName: c.Query("namespace"),
		Status:        c.Query("status"),
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

	releases, total, err := h.store.ListHelmReleases(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("list helm releases", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list helm releases"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":   releases,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

// GetHelmRelease returns a single Helm release by ID.
// GET /api/v1/helm/:id
func (h *HelmHandler) GetHelmRelease(c *gin.Context) {
	id := c.Param("id")

	release, err := h.store.GetHelmRelease(c.Request.Context(), id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "helm release not found"})
			return
		}
		h.logger.Error("get helm release", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get helm release"})
		return
	}

	c.JSON(http.StatusOK, release)
}
