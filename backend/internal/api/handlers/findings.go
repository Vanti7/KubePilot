package handlers

import (
	"net/http"
	"strconv"

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
