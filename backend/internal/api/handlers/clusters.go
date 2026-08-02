package handlers

import (
	"context"
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

// ClusterSyncer triggers an immediate collection pass for a cluster.
// Implemented by collector.CollectorManager; kept as an interface here to avoid
// a hard dependency from the API layer on the collector package.
type ClusterSyncer interface {
	TriggerSync(clusterID string) bool
}

// ClusterOps is everything NewRouter needs from *collector.CollectorManager:
// triggering a sync (ClusterSyncer) and handing out a live client for
// in-request writes (ClientsetProvider, defined in workloads.go). One
// interface so a single value satisfies both without the api package ever
// importing the collector package.
type ClusterOps interface {
	ClusterSyncer
	ClientsetProvider
	RESTConfigProvider
}

// ImageChecker triggers an immediate single-image recheck against its
// registry rather than waiting for the next periodic image-watcher pass —
// used right after a real fix (see FindingHandler.RemediateFinding) so the
// corresponding finding can resolve within seconds instead of up to
// WORKER_INTERVAL_SECONDS later. Implemented by *watcher.ImageWatcher
// (CheckImage is already exported for its own per-tick loop); kept as an
// interface here so the api package never imports internal/watcher.
type ImageChecker interface {
	CheckImage(ctx context.Context, image *models.ContainerImage) error
}

// HelmChecker is ImageChecker's Helm-release equivalent, implemented by
// *watcher.HelmWatcher (CheckRelease).
type HelmChecker interface {
	CheckRelease(ctx context.Context, release *models.HelmRelease) error
}

// ClusterHandler handles cluster CRUD endpoints.
type ClusterHandler struct {
	store  *store.Store
	syncer ClusterSyncer
	logger *zap.Logger
}

// NewClusterHandler creates a new ClusterHandler.
func NewClusterHandler(s *store.Store, syncer ClusterSyncer, logger *zap.Logger) *ClusterHandler {
	return &ClusterHandler{store: s, syncer: syncer, logger: logger}
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

	// Connection mode and SSH parameters.
	ConnectionMode    string `json:"connection_mode"`
	SSHHost           string `json:"ssh_host"`
	SSHPort           int    `json:"ssh_port"`
	SSHUser           string `json:"ssh_user"`
	SSHPassword       string `json:"ssh_password"`
	SSHKubeconfigPath string `json:"ssh_kubeconfig_path"`
	SSHSudo           bool   `json:"ssh_sudo"`
}

// CreateCluster registers a new cluster.
// POST /api/v1/clusters
func (h *ClusterHandler) CreateCluster(c *gin.Context) {
	var req createClusterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	mode := req.ConnectionMode
	if mode == "" {
		mode = models.ClusterConnKubeconfig
	}

	if mode == models.ClusterConnSSH {
		if req.SSHHost == "" || req.SSHUser == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ssh_host and ssh_user are required for ssh connection mode"})
			return
		}
		if req.SSHPort == 0 {
			req.SSHPort = 22
		}
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

		ConnectionMode:    mode,
		SSHHost:           req.SSHHost,
		SSHPort:           req.SSHPort,
		SSHUser:           req.SSHUser,
		SSHPassword:       req.SSHPassword,
		SSHKubeconfigPath: req.SSHKubeconfigPath,
		SSHSudo:           req.SSHSudo,
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

	ConnectionMode    string `json:"connection_mode"`
	SSHHost           string `json:"ssh_host"`
	SSHPort           int    `json:"ssh_port"`
	SSHUser           string `json:"ssh_user"`
	SSHPassword       string `json:"ssh_password"`
	SSHKubeconfigPath string `json:"ssh_kubeconfig_path"`
	SSHSudo           *bool  `json:"ssh_sudo"`
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
	if req.ConnectionMode != "" {
		cluster.ConnectionMode = req.ConnectionMode
	}
	if req.SSHHost != "" {
		cluster.SSHHost = req.SSHHost
	}
	if req.SSHPort != 0 {
		cluster.SSHPort = req.SSHPort
	}
	if req.SSHUser != "" {
		cluster.SSHUser = req.SSHUser
	}
	if req.SSHPassword != "" {
		cluster.SSHPassword = req.SSHPassword
	}
	if req.SSHKubeconfigPath != "" {
		cluster.SSHKubeconfigPath = req.SSHKubeconfigPath
	}
	if req.SSHSudo != nil {
		cluster.SSHSudo = *req.SSHSudo
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

	// Kick off an immediate collection pass if a collector is active for this cluster.
	triggered := false
	if h.syncer != nil {
		triggered = h.syncer.TriggerSync(id)
	}

	h.logger.Info("cluster sync triggered", zap.String("id", id), zap.Bool("collector_active", triggered))
	c.JSON(http.StatusAccepted, gin.H{"message": "sync triggered", "cluster_id": id, "collector_active": triggered})
}

// GetClusterResources returns the cluster-wide capacity vs live usage summary.
// GET /api/v1/clusters/:id/resources
func (h *ClusterHandler) GetClusterResources(c *gin.Context) {
	id := c.Param("id")
	if _, err := h.store.GetCluster(c.Request.Context(), id); err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "cluster not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get cluster"})
		return
	}

	res, err := h.store.SystemResources(c.Request.Context(), id)
	if err != nil {
		h.logger.Error("cluster resources", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get cluster resources"})
		return
	}
	c.JSON(http.StatusOK, res)
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
