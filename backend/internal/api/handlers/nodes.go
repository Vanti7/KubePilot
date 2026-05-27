package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/backend/internal/models"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
	"gorm.io/datatypes"
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

// nodeDTO is the response shape expected by the frontend.
// It maps individual capacity/allocatable string fields into objects.
type nodeDTO struct {
	ID               string            `json:"id"`
	ClusterID        string            `json:"cluster_id"`
	Name             string            `json:"name"`
	Role             string            `json:"role"`
	Status           string            `json:"status"`
	K8sVersion       string            `json:"k8s_version"`
	KubeletVersion   string            `json:"kubelet_version"` // alias for K8sVersion
	OSImage          string            `json:"os_image"`
	KernelVersion    string            `json:"kernel_version"`
	ContainerRuntime string            `json:"container_runtime"`
	Arch             string            `json:"arch"`
	Capacity         map[string]string `json:"capacity"`
	Allocatable      map[string]string `json:"allocatable"`
	Labels           datatypes.JSON    `json:"labels,omitempty"`
	Taints           datatypes.JSON    `json:"taints,omitempty"`
	Conditions       datatypes.JSON    `json:"conditions,omitempty"`
	LastSeenAt       time.Time         `json:"last_seen_at"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

func nodeToDTO(n models.Node) nodeDTO {
	return nodeDTO{
		ID:               n.ID.String(),
		ClusterID:        n.ClusterID.String(),
		Name:             n.Name,
		Role:             n.Role,
		Status:           n.Status,
		K8sVersion:       n.K8sVersion,
		KubeletVersion:   n.K8sVersion,
		OSImage:          n.OSImage,
		KernelVersion:    n.KernelVersion,
		ContainerRuntime: n.ContainerRuntime,
		Arch:             n.Arch,
		Capacity: map[string]string{
			"cpu":    n.CapacityCPU,
			"memory": n.CapacityMemory,
		},
		Allocatable: map[string]string{
			"cpu":    n.AllocatableCPU,
			"memory": n.AllocatableMemory,
		},
		Labels:     n.Labels,
		Taints:     n.Taints,
		Conditions: n.Conditions,
		LastSeenAt: n.UpdatedAt,
		CreatedAt:  n.CreatedAt,
		UpdatedAt:  n.UpdatedAt,
	}
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

	out := make([]nodeDTO, len(nodes))
	for i, n := range nodes {
		out[i] = nodeToDTO(n)
	}
	c.JSON(http.StatusOK, gin.H{"data": out, "total": len(out)})
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

	c.JSON(http.StatusOK, nodeToDTO(*node))
}
