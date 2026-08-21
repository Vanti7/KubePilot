package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/api/middleware"
	"github.com/kubepilot/backend/internal/models"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ExceptionRuleHandler handles exception rule endpoints. Exception rules
// change how the scoring engine treats matching findings (suppress / reduce
// severity / accept risk) — see internal/scoring.ScoreFinding and
// docs/scoring.md §9.
type ExceptionRuleHandler struct {
	store  *store.Store
	logger *zap.Logger
}

// NewExceptionRuleHandler creates a new ExceptionRuleHandler.
func NewExceptionRuleHandler(s *store.Store, logger *zap.Logger) *ExceptionRuleHandler {
	return &ExceptionRuleHandler{store: s, logger: logger}
}

// ListExceptionRules returns every exception rule (active, inactive and
// expired) for the management page.
// GET /api/v1/exception-rules
func (h *ExceptionRuleHandler) ListExceptionRules(c *gin.Context) {
	rules, err := h.store.ListExceptionRules(c.Request.Context())
	if err != nil {
		h.logger.Error("list exception rules", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list exception rules"})
		return
	}
	c.JSON(http.StatusOK, rules)
}

// exceptionRuleRequest is the create/update payload. Scope is whichever of
// ClusterID/WorkloadID/ImagePattern is set (NamespaceName narrows a
// cluster-scoped rule further) — see models.ExceptionRule's doc comment for
// the precedence order the scoring engine applies when several match.
type exceptionRuleRequest struct {
	Name          string     `json:"name"      binding:"required"`
	RuleType      string     `json:"rule_type" binding:"required"`
	ClusterID     *string    `json:"cluster_id"`
	NamespaceName string     `json:"namespace_name"`
	WorkloadID    *string    `json:"workload_id"`
	FindingKind   string     `json:"finding_kind"`
	ImagePattern  string     `json:"image_pattern"`
	Reason        string     `json:"reason"    binding:"required"`
	ExpiresAt     *time.Time `json:"expires_at"`
	IsActive      *bool      `json:"is_active"`
}

func validRuleType(t string) bool {
	switch t {
	case models.ExceptionRuleTypeSuppress, models.ExceptionRuleTypeReduceSeverity, models.ExceptionRuleTypeAcceptRisk:
		return true
	}
	return false
}

// parseOptionalUUID parses *s into a *uuid.UUID, or returns nil for an unset/empty field.
func parseOptionalUUID(s *string) (*uuid.UUID, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	id, err := uuid.Parse(*s)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// CreateExceptionRule adds a new exception rule.
// POST /api/v1/exception-rules
func (h *ExceptionRuleHandler) CreateExceptionRule(c *gin.Context) {
	var req exceptionRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !validRuleType(req.RuleType) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rule_type"})
		return
	}
	clusterID, err := parseOptionalUUID(req.ClusterID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cluster_id"})
		return
	}
	workloadID, err := parseOptionalUUID(req.WorkloadID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid workload_id"})
		return
	}

	var createdBy *uuid.UUID
	if raw, ok := c.Get(middleware.ContextKeyUserID); ok {
		if s, ok := raw.(string); ok {
			if uid, err := uuid.Parse(s); err == nil {
				createdBy = &uid
			}
		}
	}

	rule := &models.ExceptionRule{
		Name:          req.Name,
		RuleType:      req.RuleType,
		ClusterID:     clusterID,
		NamespaceName: req.NamespaceName,
		WorkloadID:    workloadID,
		FindingKind:   req.FindingKind,
		ImagePattern:  req.ImagePattern,
		Reason:        req.Reason,
		ExpiresAt:     req.ExpiresAt,
		CreatedByID:   createdBy,
		IsActive:      true,
	}
	if req.IsActive != nil {
		rule.IsActive = *req.IsActive
	}

	if err := h.store.CreateExceptionRule(c.Request.Context(), rule); err != nil {
		h.logger.Error("create exception rule", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create exception rule"})
		return
	}
	c.JSON(http.StatusCreated, rule)
}

// UpdateExceptionRule replaces an existing exception rule's editable fields
// (including a plain is_active toggle — the frontend reuses this endpoint
// for pause/resume rather than a separate route).
// PUT /api/v1/exception-rules/:id
func (h *ExceptionRuleHandler) UpdateExceptionRule(c *gin.Context) {
	id := c.Param("id")

	var req exceptionRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !validRuleType(req.RuleType) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rule_type"})
		return
	}
	clusterID, err := parseOptionalUUID(req.ClusterID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cluster_id"})
		return
	}
	workloadID, err := parseOptionalUUID(req.WorkloadID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid workload_id"})
		return
	}

	rule, err := h.store.GetExceptionRule(c.Request.Context(), id)
	if err != nil {
		h.logger.Error("get exception rule", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load exception rule"})
		return
	}
	if rule == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "exception rule not found"})
		return
	}

	rule.Name = req.Name
	rule.RuleType = req.RuleType
	rule.ClusterID = clusterID
	rule.NamespaceName = req.NamespaceName
	rule.WorkloadID = workloadID
	rule.FindingKind = req.FindingKind
	rule.ImagePattern = req.ImagePattern
	rule.Reason = req.Reason
	rule.ExpiresAt = req.ExpiresAt
	if req.IsActive != nil {
		rule.IsActive = *req.IsActive
	}
	// Preloaded by GetExceptionRule — clear before Save so GORM doesn't also
	// try to upsert these as nested associations.
	rule.Cluster = nil
	rule.Workload = nil

	if err := h.store.UpdateExceptionRule(c.Request.Context(), rule); err != nil {
		h.logger.Error("update exception rule", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update exception rule"})
		return
	}
	c.JSON(http.StatusOK, rule)
}

// DeleteExceptionRule removes an exception rule.
// DELETE /api/v1/exception-rules/:id
func (h *ExceptionRuleHandler) DeleteExceptionRule(c *gin.Context) {
	id := c.Param("id")
	if err := h.store.DeleteExceptionRule(c.Request.Context(), id); err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "exception rule not found"})
			return
		}
		h.logger.Error("delete exception rule", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete exception rule"})
		return
	}
	c.Status(http.StatusNoContent)
}
