package handlers

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/backend/internal/api/middleware"
	"github.com/kubepilot/backend/internal/helmops"
	"github.com/kubepilot/backend/internal/k8sops"
	"github.com/kubepilot/backend/internal/models"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// FindingHandler handles finding-related endpoints.
type FindingHandler struct {
	store        *store.Store
	cluster      ClusterOps
	imageChecker ImageChecker
	helmChecker  HelmChecker
	logger       *zap.Logger
}

// NewFindingHandler creates a new FindingHandler. imageChecker/helmChecker may
// be nil (e.g. demo mode, where no watcher runs) — RemediateFinding then just
// skips the fast-resolve recheck and leaves the finding to the normal watcher
// cycle instead.
func NewFindingHandler(s *store.Store, cluster ClusterOps, imageChecker ImageChecker, helmChecker HelmChecker, logger *zap.Logger) *FindingHandler {
	return &FindingHandler{store: s, cluster: cluster, imageChecker: imageChecker, helmChecker: helmChecker, logger: logger}
}

// ListFindings returns paginated findings with optional filters.
// GET /api/v1/findings
func (h *FindingHandler) ListFindings(c *gin.Context) {
	filter := store.FindingFilter{
		ClusterID:   c.Query("cluster_id"),
		NamespaceID: c.Query("namespace_id"),
		Severity:    c.Query("severity"),
		Status:      c.Query("status"),
		Kind:        c.Query("kind"),
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

	findings, total, err := h.store.ListFindings(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("list findings", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list findings"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":   findings,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

// ExportFindings streams matching findings as a CSV download. It honors the
// same filters as ListFindings and pages internally up to EXPORT_MAX_ROWS
// (default 10000).
// GET /api/v1/findings/export
func (h *FindingHandler) ExportFindings(c *gin.Context) {
	filter := store.FindingFilter{
		ClusterID:   c.Query("cluster_id"),
		NamespaceID: c.Query("namespace_id"),
		Severity:    c.Query("severity"),
		Status:      c.Query("status"),
		Kind:        c.Query("kind"),
	}

	maxRows := 10000
	if v := os.Getenv("EXPORT_MAX_ROWS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxRows = n
		}
	}

	const page = 500
	var all []store.FindingWithScore
	for offset := 0; offset < maxRows; offset += page {
		filter.Limit = page
		filter.Offset = offset
		rows, total, err := h.store.ListFindings(c.Request.Context(), filter)
		if err != nil {
			h.logger.Error("export findings", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to export findings"})
			return
		}
		all = append(all, rows...)
		if len(rows) < page || int64(len(all)) >= total {
			break
		}
	}
	if len(all) > maxRows {
		all = all[:maxRows]
	}

	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", `attachment; filename="kubepilot-findings.csv"`)

	w := csv.NewWriter(c.Writer)
	defer w.Flush()
	_ = w.Write([]string{
		"cluster", "namespace", "kind", "resource", "current_version", "latest_version",
		"update_type", "severity", "score", "status", "first_detected_at", "title",
	})
	for _, f := range all {
		score := ""
		if f.Score != nil {
			score = strconv.FormatFloat(*f.Score, 'f', 0, 64)
		}
		_ = w.Write([]string{
			f.ClusterName,
			f.NamespaceName,
			f.Kind,
			f.WorkloadName,
			f.CurrentVersion,
			f.LatestVersion,
			f.UpdateType,
			f.Severity,
			score,
			f.Status,
			f.FirstDetectedAt.Format(time.RFC3339),
			f.Title,
		})
	}
}

// GetFinding returns a single finding by ID.
// GET /api/v1/findings/:id
func (h *FindingHandler) GetFinding(c *gin.Context) {
	id := c.Param("id")
	finding, err := h.store.GetFinding(c.Request.Context(), id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "finding not found"})
			return
		}
		h.logger.Error("get finding", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get finding"})
		return
	}

	rs, _ := h.store.GetRiskScore(c.Request.Context(), id)
	c.JSON(http.StatusOK, gin.H{"finding": finding, "risk_score": rs})
}

type updateStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

// UpdateFindingStatus transitions a finding's status.
// PATCH /api/v1/findings/:id/status
func (h *FindingHandler) UpdateFindingStatus(c *gin.Context) {
	id := c.Param("id")

	var req updateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	validStatuses := map[string]bool{
		models.FindingStatusOpen:     true,
		models.FindingStatusPlanned:  true,
		models.FindingStatusIgnored:  true,
		models.FindingStatusApproved: true,
		models.FindingStatusBlocked:  true,
		models.FindingStatusResolved: true,
	}
	if !validStatuses[req.Status] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status value"})
		return
	}

	userID, _ := c.Get(middleware.ContextKeyUserID)
	userIDStr, _ := userID.(string)

	if _, err := h.store.GetFinding(c.Request.Context(), id); err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "finding not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get finding"})
		return
	}

	if err := h.store.UpdateFindingStatus(c.Request.Context(), id, req.Status, userIDStr); err != nil {
		h.logger.Error("update finding status", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": id, "status": req.Status})
}

// RemediateFinding applies the real fix a finding represents — unlike
// UpdateFindingStatus (a label change only), this mutates the cluster: bumps
// a container's image tag to LatestVersion (image findings) or upgrades the
// Helm release to LatestVersion (helm findings).
// POST /api/v1/findings/:id/remediate
func (h *FindingHandler) RemediateFinding(c *gin.Context) {
	id := c.Param("id")

	finding, err := h.store.GetFinding(c.Request.Context(), id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "finding not found"})
			return
		}
		h.logger.Error("get finding", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get finding"})
		return
	}

	if !store.IsActiveFindingStatus(finding.Status) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "finding is not actionable in its current status"})
		return
	}

	switch finding.Kind {
	case models.FindingKindImage:
		h.remediateImage(c, finding)
	case models.FindingKindHelm:
		h.remediateHelm(c, finding)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "remediation is not supported for this finding kind"})
	}
}

func (h *FindingHandler) remediateImage(c *gin.Context, finding *models.UpdateFinding) {
	ctx := c.Request.Context()
	if finding.Workload == nil || finding.ContainerImage == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "this finding is missing workload or image data — try re-syncing the cluster"})
		return
	}

	clientset, ok := h.cluster.GetClientset(finding.ClusterID.String())
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cluster is not currently connected"})
		return
	}

	// Same normalization collector.parseImage always applies (registry/repository:tag),
	// so this stays a valid reference even for implicit Docker Hub images.
	newImage := fmt.Sprintf("%s/%s:%s", finding.ContainerImage.Registry, finding.ContainerImage.Repository, finding.LatestVersion)
	fixErr := k8sops.SetContainerImage(ctx, clientset, finding.Workload.Kind, finding.NamespaceName, finding.Workload.Name, finding.ContainerImage.ContainerName, newImage)

	status := models.ActionLogStatusSuccess
	details := map[string]interface{}{
		"finding_id":     finding.ID,
		"container_name": finding.ContainerImage.ContainerName,
		"from_image":     finding.ContainerImage.Image,
		"to_image":       newImage,
	}
	if fixErr != nil {
		status = models.ActionLogStatusFailure
		details["error"] = fixErr.Error()
	}
	RecordAction(c, h.store, h.logger, models.ActionLogUpdateImage, "workload", finding.WorkloadID.String(), status, details)

	if fixErr != nil {
		h.logger.Error("remediate image finding", zap.String("finding_id", finding.ID.String()), zap.Error(fixErr))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update image: " + fixErr.Error()})
		return
	}

	// Same TriggerSync correctif as scale/restart/helm/deploy — collectWorkloads
	// has no live Watch, only a 60s poll.
	h.cluster.TriggerSync(finding.ClusterID.String())
	h.scheduleImageRecheck(finding.ID.String(), finding.ContainerImage.ID.String())

	c.JSON(http.StatusOK, gin.H{"id": finding.ID, "kind": finding.Kind, "detail": gin.H{"image": newImage}})
}

func (h *FindingHandler) remediateHelm(c *gin.Context, finding *models.UpdateFinding) {
	ctx := c.Request.Context()
	if finding.HelmRelease == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "this finding is missing Helm release data — try re-syncing the cluster"})
		return
	}
	release := finding.HelmRelease

	if release.RepoURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chart repository not resolved yet for this release"})
		return
	}

	// Same sequence as HelmHandler.UpgradeHelmRelease, deliberately duplicated
	// rather than factored into a shared helper — mirrors the existing
	// watcher/helmops chart-resolution duplication in this codebase, and
	// keeps this new endpoint from risking a behavior change in the
	// already-shipped Helm-page upgrade flow.
	values := map[string]interface{}{}
	if len(release.Values) > 0 {
		_ = json.Unmarshal(release.Values, &values)
	}

	repo, repoErr := h.store.GetHelmRepositoryByURL(ctx, release.RepoURL)
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
		h.logger.Error("build helm action config", zap.String("finding_id", finding.ID.String()), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to connect to cluster for helm"})
		return
	}

	rel, upErr := helmops.UpgradeChart(ctx, cfg, release.RepoURL, repo, release.Name, release.ChartName, finding.LatestVersion, values)

	status := models.ActionLogStatusSuccess
	details := map[string]interface{}{
		"finding_id":   finding.ID,
		"chart_name":   release.ChartName,
		"from_version": release.ChartVersion,
		"to_version":   finding.LatestVersion,
		"namespace":    release.NamespaceName,
		"release_name": release.Name,
	}
	if upErr != nil {
		status = models.ActionLogStatusFailure
		details["error"] = upErr.Error()
	}
	RecordAction(c, h.store, h.logger, models.ActionLogHelmUpgrade, "helm_release", finding.HelmReleaseID.String(), status, details)

	if upErr != nil {
		h.logger.Error("remediate helm finding", zap.String("finding_id", finding.ID.String()), zap.Error(upErr))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upgrade helm release: " + upErr.Error()})
		return
	}

	h.cluster.TriggerSync(release.ClusterID.String())
	h.scheduleHelmRecheck(finding.ID.String(), release.ID.String())

	c.JSON(http.StatusOK, gin.H{"id": finding.ID, "kind": finding.Kind, "detail": gin.H{"revision": rel.Version, "chart_version": finding.LatestVersion}})
}

// postRemediateRecheckDelays paces the fast-resolve recheck after a real fix:
// the collector needs its own pass to pick up the freshly-patched live state
// (new image tag / new Helm revision) before CheckImage/CheckRelease can see
// it, so a single immediate check would usually just see stale data.
var postRemediateRecheckDelays = []time.Duration{3 * time.Second, 6 * time.Second, 12 * time.Second}

// scheduleImageRecheck re-evaluates a single image against its registry
// shortly after a real fix, instead of waiting for the image watcher's next
// periodic pass (WORKER_INTERVAL_SECONDS, default 300s) — CheckImage already
// resolves the finding itself once it sees the installed tag has caught up
// (store.ResolveActiveFindingForImage). Deliberately detached from the
// request (context.Background(), its own bounded timeout per attempt): the
// HTTP response is already written by the time any of this runs. Never
// surfaces to the caller — failures are logged and swallowed, the same
// fire-and-forget philosophy as RecordAction.
func (h *FindingHandler) scheduleImageRecheck(findingID, imageID string) {
	if h.imageChecker == nil {
		return
	}
	go func() {
		for _, delay := range postRemediateRecheckDelays {
			time.Sleep(delay)

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			image, err := h.store.GetContainerImage(ctx, imageID)
			if err != nil {
				h.logger.Warn("post-remediate image recheck: load image", zap.String("image_id", imageID), zap.Error(err))
				cancel()
				continue
			}
			if err := h.imageChecker.CheckImage(ctx, image); err != nil {
				h.logger.Warn("post-remediate image recheck", zap.String("image_id", imageID), zap.Error(err))
			}
			finding, ferr := h.store.GetFinding(ctx, findingID)
			cancel()
			if ferr == nil && finding.Status == models.FindingStatusResolved {
				return
			}
		}
	}()
}

// scheduleHelmRecheck is scheduleImageRecheck's Helm-release equivalent.
func (h *FindingHandler) scheduleHelmRecheck(findingID, releaseID string) {
	if h.helmChecker == nil {
		return
	}
	go func() {
		for _, delay := range postRemediateRecheckDelays {
			time.Sleep(delay)

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			release, err := h.store.GetHelmRelease(ctx, releaseID)
			if err != nil {
				h.logger.Warn("post-remediate helm recheck: load release", zap.String("release_id", releaseID), zap.Error(err))
				cancel()
				continue
			}
			if err := h.helmChecker.CheckRelease(ctx, release); err != nil {
				h.logger.Warn("post-remediate helm recheck", zap.String("release_id", releaseID), zap.Error(err))
			}
			finding, ferr := h.store.GetFinding(ctx, findingID)
			cancel()
			if ferr == nil && finding.Status == models.FindingStatusResolved {
				return
			}
		}
	}()
}

// GetFindingSummary returns aggregated counts of findings by severity.
// GET /api/v1/findings/summary
func (h *FindingHandler) GetFindingSummary(c *gin.Context) {
	var clusterIDs []string
	if ids := c.QueryArray("cluster_id"); len(ids) > 0 {
		clusterIDs = ids
	}

	summary, err := h.store.GetFindingSummary(c.Request.Context(), clusterIDs)
	if err != nil {
		h.logger.Error("get finding summary", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get summary"})
		return
	}

	c.JSON(http.StatusOK, summary)
}
