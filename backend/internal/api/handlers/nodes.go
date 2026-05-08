package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// NodeHandler handles Kubernetes node endpoints.
type NodeHandler struct {
	store  *store.Store
	logger *zap.Logger
}

// NewNodeHandler creates a new NodeHandler.
func NewNodeHandler(s *store.Store, logger *zap.Logger) *NodeHandler {
	return &NodeHandler{store: s, logger: logger}
}

// ListNodes returns all nodes, optionally filtered by cluster.
// GET /api/v1/nodes
func (h *NodeHandler) ListNodes(c *gin.Context) {
	clusterID := c.Query("cluster_id")

	nodes, err := h.store.ListNodes(c.Request.Context(), clusterID)
	if err != nil {
		h.logger.Error("list nodes", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list nodes"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": nodes, "total": len(nodes)})
}

// GetNode returns a single node by ID.
// GET /api/v1/nodes/:id
func (h *NodeHandler) GetNode(c *gin.Context) {
	id := c.Param("id")

	node, err := h.store.GetNode(c.Request.Context(), id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
			return
		}
		h.logger.Error("get node", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get node"})
		return
	}

	c.JSON(http.StatusOK, node)
}
