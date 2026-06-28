package handlers

import (
	"encoding/csv"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/backend/internal/api/middleware"
	"github.com/kubepilot/backend/internal/models"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// FindingHandler handles finding-related endpoints.
type FindingHandler struct {
	store  *store.Store
	logger *zap.Logger
}

// NewFindingHandler creates a new FindingHandler.
func NewFindingHandler(s *store.Store, logger *zap.Logger) *FindingHandler {
	return &FindingHandler{store: s, logger: logger}
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
