package handlers

import (
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ClusterHandler handles cluster CRUD endpoints.
type ClusterHandler struct {
	store  *store.Store
	logger *zap.Logger
}

// NewClusterHandler creates a new ClusterHandler.
func NewClusterHandler(s *store.Store, logger *zap.Logger) *ClusterHandler {
	return &ClusterHandler{store: s, logger: logger}
}

// ListClusters returns all clusters.
// GET /api/v1/clusters
func (h *ClusterHandler) ListClusters(c *gin.Context) {
	clusters, err := h.store.ListClusters(c.Request.Context())
	if err != nil {
		h.logger.Error("list clusters", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list clusters"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": clusters, "total": len(clusters)})
}

// GetCluster returns a single cluster by ID.
// GET /api/v1/clusters/:id
func (h *ClusterHandler) GetCluster(c *gin.Context) {
	id := c.Param("id")
	cluster, err := h.store.GetCluster(c.Request.Context(), id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "cluster not found"})
			return
		}
		h.logger.Error("get cluster", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get cluster"})
		return
	}
	c.JSON(http.StatusOK, cluster)
}

type createClusterRequest struct {
	Name          string     `json:"name"           binding:"required"`
	EnvironmentID *uuid.UUID `json:"environment_id"`
	Provider      string     `json:"provider"`
	Region        string     `json:"region"`
	APIEndpoint   string     `json:"api_endpoint"`
	KubeconfigRef string     `json:"kubeconfig_ref"`
	TLSInsecure   bool       `json:"tls_insecure"`
}

// CreateCluster registers a new cluster.
// POST /api/v1/clusters
func (h *ClusterHandler) CreateCluster(c *gin.Context) {
	var req createClusterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cluster := models.Cluster{
		ID:            uuid.New(),
		Name:          req.Name,
		Slug:          slugify(req.Name),
		EnvironmentID: req.EnvironmentID,
		Provider:      req.Provider,
		Region:        req.Region,
		APIEndpoint:   req.APIEndpoint,
		KubeconfigRef: req.KubeconfigRef,
		TLSInsecure:   req.TLSInsecure,
		Status:        models.ClusterStatusUnknown,
	}

	if err := h.store.CreateCluster(c.Request.Context(), &cluster); err != nil {
		h.logger.Error("create cluster", zap.Error(err))
		c.JSON(http.StatusConflict, gin.H{"error": "cluster name already exists"})
		return
	}

	c.JSON(http.StatusCreated, cluster)
}

type updateClusterRequest struct {
	Name          string     `json:"name"`
	EnvironmentID *uuid.UUID `json:"environment_id"`
	Provider      string     `json:"provider"`
	Region        string     `json:"region"`
	APIEndpoint   string     `json:"api_endpoint"`
	KubeconfigRef string     `json:"kubeconfig_ref"`
	TLSInsecure   *bool      `json:"tls_insecure"`
}

// UpdateCluster updates cluster metadata.
// PUT /api/v1/clusters/:id
func (h *ClusterHandler) UpdateCluster(c *gin.Context) {
	id := c.Param("id")
	cluster, err := h.store.GetCluster(c.Request.Context(), id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "cluster not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get cluster"})
		return
	}

	var req updateClusterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != "" {
		cluster.Name = req.Name
		cluster.Slug = slugify(req.Name)
	}
	if req.EnvironmentID != nil {
		cluster.EnvironmentID = req.EnvironmentID
	}
	if req.Provider != "" {
		cluster.Provider = req.Provider
	}
	if req.Region != "" {
		cluster.Region = req.Region
	}
	if req.APIEndpoint != "" {
		cluster.APIEndpoint = req.APIEndpoint
	}
	if req.KubeconfigRef != "" {
		cluster.KubeconfigRef = req.KubeconfigRef
	}
	if req.TLSInsecure != nil {
		cluster.TLSInsecure = *req.TLSInsecure
	}

	if err := h.store.UpdateCluster(c.Request.Context(), cluster); err != nil {
		h.logger.Error("update cluster", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update cluster"})
		return
	}

	c.JSON(http.StatusOK, cluster)
}

// DeleteCluster removes a cluster by ID.
// DELETE /api/v1/clusters/:id
func (h *ClusterHandler) DeleteCluster(c *gin.Context) {
	id := c.Param("id")
	if _, err := h.store.GetCluster(c.Request.Context(), id); err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "cluster not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get cluster"})
		return
	}

	if err := h.store.DeleteCluster(c.Request.Context(), id); err != nil {
		h.logger.Error("delete cluster", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete cluster"})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

// SyncCluster triggers an immediate re-collection for a cluster.
// POST /api/v1/clusters/:id/sync
func (h *ClusterHandler) SyncCluster(c *gin.Context) {
	id := c.Param("id")
	_, err := h.store.GetCluster(c.Request.Context(), id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "cluster not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get cluster"})
		return
	}

	if err := h.store.UpdateClusterStatus(c.Request.Context(), id, models.ClusterStatusUnknown, time.Now()); err != nil {
		h.logger.Error("sync cluster status reset", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to trigger sync"})
		return
	}

	h.logger.Info("cluster sync triggered", zap.String("id", id))
	c.JSON(http.StatusAccepted, gin.H{"message": "sync triggered", "cluster_id": id})
}

// slugify converts a display name to a URL-safe slug.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	// Collapse multiple dashes.
	result := b.String()
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	return strings.Trim(result, "-")
}
