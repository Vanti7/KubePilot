package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
)

// SecretHandler handles secret endpoints.
type SecretHandler struct {
	store  *store.Store
	logger *zap.Logger
}

// NewSecretHandler creates a new SecretHandler.
func NewSecretHandler(s *store.Store, logger *zap.Logger) *SecretHandler {
	return &SecretHandler{store: s, logger: logger}
}

// ListSecrets returns paginated secrets with optional filters.
// GET /api/v1/secrets
func (h *SecretHandler) ListSecrets(c *gin.Context) {
	filter := store.SecretFilter{
		ClusterID:     c.Query("cluster_id"),
		NamespaceName: c.Query("namespace"),
		Type:          c.Query("type"),
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

	secrets, total, err := h.store.ListSecrets(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("list secrets", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list secrets"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":   secrets,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}
