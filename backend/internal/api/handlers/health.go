package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/backend/internal/store"
)

// HealthHandler holds dependencies for health endpoints.
type HealthHandler struct {
	store *store.Store
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(s *store.Store) *HealthHandler {
	return &HealthHandler{store: s}
}

// Liveness responds 200 OK — used by Kubernetes liveness probe.
// GET /health
func (h *HealthHandler) Liveness(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Readiness checks downstream connectivity.
// GET /health/ready
func (h *HealthHandler) Readiness(c *gin.Context) {
	if err := h.store.Health(c.Request.Context()); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unhealthy",
			"error":  err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}

// Connectors returns detailed connectivity status for each dependency.
// GET /health/connectors
func (h *HealthHandler) Connectors(c *gin.Context) {
	ctx := c.Request.Context()

	dbOK := true
	dbErr := ""
	sqlDB, err := h.store.DB.DB()
	if err != nil {
		dbOK = false
		dbErr = err.Error()
	} else if err = sqlDB.PingContext(ctx); err != nil {
		dbOK = false
		dbErr = err.Error()
	}

	cacheOK := true
	cacheErr := ""
	if err := h.store.Cache.Ping(ctx); err != nil {
		cacheOK = false
		cacheErr = err.Error()
	}

	overall := "ok"
	statusCode := http.StatusOK
	if !dbOK || !cacheOK {
		overall = "degraded"
		statusCode = http.StatusServiceUnavailable
	}

	c.JSON(statusCode, gin.H{
		"status": overall,
		"connectors": gin.H{
			"database": gin.H{"ok": dbOK, "error": dbErr},
			"cache":    gin.H{"ok": cacheOK, "error": cacheErr},
		},
	})
}
