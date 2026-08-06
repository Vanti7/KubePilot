package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/backend/internal/api/middleware"
	"github.com/kubepilot/backend/internal/helmops"
	"github.com/kubepilot/backend/internal/models"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// hideValuesFromViewers blanks the Values payload for callers below operator
// role. Helm values routinely carry secrets (DB passwords, API keys, tokens)
// and a viewer has no legitimate use for them — upgrading is operator+ only.
func hideValuesFromViewers(c *gin.Context, release *models.HelmRelease) {
	role, _ := c.Get(middleware.ContextKeyRole)
	roleStr, _ := role.(string)
	if roleStr == models.RoleOperator || roleStr == models.RoleAdmin {
		return
	}
	release.Values = nil
}

// HelmHandler handles Helm release endpoints.
type HelmHandler struct {
	store   *store.Store
	cluster ClusterOps
	logger  *zap.Logger
}

// NewHelmHandler creates a new HelmHandler.
func NewHelmHandler(s *store.Store, cluster ClusterOps, logger *zap.Logger) *HelmHandler {
	return &HelmHandler{store: s, cluster: cluster, logger: logger}
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
	for i := range releases {
		hideValuesFromViewers(c, &releases[i])
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

	hideValuesFromViewers(c, release)
	c.JSON(http.StatusOK, release)
}

type upgradeHelmReleaseRequest struct {
	// Both optional: an empty ChartVersion keeps the currently installed
	// version; a nil Values keeps the currently stored values (see below) —
	// this lets the same endpoint serve "just edit the values" and "bump the
	// chart version" without a separate route for each.
	ChartVersion string                 `json:"chart_version"`
	Values       map[string]interface{} `json:"values"`
}

// UpgradeHelmRelease runs a real `helm upgrade` for a release: a new chart
// version, new values, or both. A Helm upgrade always re-renders the full
// chart, so even a values-only edit downloads the (same-version) chart.
// POST /api/v1/helm/:id/upgrade
func (h *HelmHandler) UpgradeHelmRelease(c *gin.Context) {
	id := c.Param("id")

	var req upgradeHelmReleaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

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

	if release.RepoURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chart repository not resolved yet for this release"})
		return
	}

	targetVersion := req.ChartVersion
	if targetVersion == "" {
		targetVersion = release.ChartVersion
	}

	values := req.Values
	if values == nil {
		// Preserve the currently deployed values instead of wiping them —
		// Helm upgrade always replaces the values wholesale (no
		// --reuse-values here), so a caller that only wants to bump the
		// chart version must not lose its values as a side effect.
		values = map[string]interface{}{}
		if len(release.Values) > 0 {
			_ = json.Unmarshal(release.Values, &values)
		}
	}

	repo, repoErr := h.store.GetHelmRepositoryByURL(c.Request.Context(), release.RepoURL)
	if repoErr != nil && repoErr != gorm.ErrRecordNotFound {
		h.logger.Warn("get helm repository", zap.String("repo_url", release.RepoURL), zap.Error(repoErr))
	}

	restConfig, ok := h.cluster.GetRESTConfig(release.ClusterID.String())
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cluster is not currently connected"})
		return
	}

	cfg, err := helmops.NewActionConfig(restConfig, release.NamespaceName, h.logger)
	if err != nil {
		h.logger.Error("build helm action config", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to connect to cluster for helm"})
		return
	}

	rel, upErr := helmops.UpgradeChart(c.Request.Context(), cfg, release.RepoURL, repo, release.Name, release.ChartName, targetVersion, values)

	status := models.ActionLogStatusSuccess
	details := map[string]interface{}{
		"chart_name":   release.ChartName,
		"from_version": release.ChartVersion,
		"to_version":   targetVersion,
		"namespace":    release.NamespaceName,
		"release_name": release.Name,
	}
	if upErr != nil {
		status = models.ActionLogStatusFailure
		details["error"] = upErr.Error()
	}
	RecordAction(c, h.store, h.logger, models.ActionLogHelmUpgrade, "helm_release", id, status, details)

	if upErr != nil {
		h.logger.Error("upgrade helm release", zap.String("id", id), zap.Error(upErr))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upgrade helm release: " + upErr.Error()})
		return
	}

	// collectHelmReleases only re-runs every 60s otherwise (no live Watch,
	// same limitation as workloads) — kick an immediate pass so the new
	// revision/values show up within seconds.
	h.cluster.TriggerSync(release.ClusterID.String())

	c.JSON(http.StatusOK, gin.H{"id": id, "revision": rel.Version, "chart_version": targetVersion})
}

type rollbackHelmReleaseRequest struct {
	Revision int `json:"revision" binding:"required,min=1"`
}

// RollbackHelmRelease restores a release to a previous revision. No chart
// download: Helm restores the manifest already stored for that revision.
// POST /api/v1/helm/:id/rollback
func (h *HelmHandler) RollbackHelmRelease(c *gin.Context) {
	id := c.Param("id")

	var req rollbackHelmReleaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

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

	restConfig, ok := h.cluster.GetRESTConfig(release.ClusterID.String())
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cluster is not currently connected"})
		return
	}

	cfg, err := helmops.NewActionConfig(restConfig, release.NamespaceName, h.logger)
	if err != nil {
		h.logger.Error("build helm action config", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to connect to cluster for helm"})
		return
	}

	rbErr := helmops.Rollback(cfg, release.Name, req.Revision)

	status := models.ActionLogStatusSuccess
	details := map[string]interface{}{
		"release_name":  release.Name,
		"namespace":     release.NamespaceName,
		"from_revision": release.Revision,
		"to_revision":   req.Revision,
	}
	if rbErr != nil {
		status = models.ActionLogStatusFailure
		details["error"] = rbErr.Error()
	}
	RecordAction(c, h.store, h.logger, models.ActionLogHelmRollback, "helm_release", id, status, details)

	if rbErr != nil {
		h.logger.Error("rollback helm release", zap.String("id", id), zap.Error(rbErr))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to rollback helm release: " + rbErr.Error()})
		return
	}

	h.cluster.TriggerSync(release.ClusterID.String())

	c.JSON(http.StatusOK, gin.H{"id": id, "revision": req.Revision})
}
