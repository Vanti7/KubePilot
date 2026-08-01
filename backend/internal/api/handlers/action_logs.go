package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/api/middleware"
	"github.com/kubepilot/backend/internal/models"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
	"gorm.io/datatypes"
)

// ActionLogHandler handles the audit-trail read endpoint.
type ActionLogHandler struct {
	store  *store.Store
	logger *zap.Logger
}

// NewActionLogHandler creates a new ActionLogHandler.
func NewActionLogHandler(s *store.Store, logger *zap.Logger) *ActionLogHandler {
	return &ActionLogHandler{store: s, logger: logger}
}

// ListActionLogs returns paginated audit-trail entries with optional filters.
// GET /api/v1/action-logs
func (h *ActionLogHandler) ListActionLogs(c *gin.Context) {
	filter := store.ActionLogFilter{
		UserID:     c.Query("user_id"),
		Action:     c.Query("action"),
		EntityType: c.Query("entity_type"),
		EntityID:   c.Query("entity_id"),
		Status:     c.Query("status"),
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

	logs, total, err := h.store.ListActionLogs(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("list action logs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list action logs"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":   logs,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

// RecordAction persists an audit-trail entry for a write action a handler
// just attempted. Call it once, after the mutation runs, passing
// models.ActionLogStatusSuccess or ActionLogStatusFailure — this is the
// reusable half of the "action_logs + RBAC guardrail" foundation that future
// write endpoints (deploy, Helm upgrade, scale/restart) build on; the role
// side is the existing middleware.RequireRole, applied at the router like
// every other mutation already in this codebase.
//
// User id, client IP and User-Agent are pulled from the gin.Context that
// JWTAuth already populated, so call sites don't repeat that boilerplate.
//
// It never aborts the request and never returns an error: a failure to write
// the audit row is only logged — losing an audit entry must not mask or
// override the underlying operation's own result, which the caller has
// already decided (and typically already responded) independently.
//
// details is marshaled to JSON as-is (nil is fine); keep it small and never
// put secrets in it (Helm values, kubeconfigs, tokens) — this is permanent.
func RecordAction(c *gin.Context, s *store.Store, logger *zap.Logger, action, entityType, entityID, status string, details map[string]interface{}) {
	entry := models.ActionLog{
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		Status:     status,
		IPAddress:  c.ClientIP(),
		UserAgent:  c.Request.UserAgent(),
	}

	if v, ok := c.Get(middleware.ContextKeyUserID); ok {
		if uidStr, ok := v.(string); ok && uidStr != "" {
			if uid, err := uuid.Parse(uidStr); err == nil {
				entry.UserID = &uid
			}
		}
	}

	if details != nil {
		if b, err := json.Marshal(details); err == nil {
			entry.Details = datatypes.JSON(b)
		}
	}

	if err := s.CreateActionLog(c.Request.Context(), &entry); err != nil {
		logger.Error("record action log", zap.String("action", action), zap.Error(err))
	}
}
