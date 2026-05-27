package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
)

// NamespaceHandler handles namespace endpoints.
type NamespaceHandler struct {
	store  *store.Store
	logger *zap.Logger
}

// NewNamespaceHandler creates a new NamespaceHandler.
func NewNamespaceHandler(s *store.Store, logger *zap.Logger) *NamespaceHandler {
	return &NamespaceHandler{store: s, logger: logger}
}

// ListNamespaces returns all namespaces, optionally filtered by cluster.
// GET /api/v1/namespaces
func (h *NamespaceHandler) ListNamespaces(c *gin.Context) {
	clusterID := c.Query("cluster_id")

	namespaces, err := h.store.ListNamespaces(c.Request.Context(), clusterID)
	if err != nil {
		h.logger.Error("list namespaces", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list namespaces"})
		return
	}

	c.JSON(http.StatusOK, namespaces)
}
