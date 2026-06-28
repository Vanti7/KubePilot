package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/backend/internal/config"
)

// SettingsHandler exposes non-secret runtime configuration for the Settings page.
type SettingsHandler struct {
	cfg     *config.Config
	version string
}

// NewSettingsHandler creates a new SettingsHandler.
func NewSettingsHandler(cfg *config.Config, version string) *SettingsHandler {
	return &SettingsHandler{cfg: cfg, version: version}
}

// settingsResponse is the non-secret subset of configuration surfaced to admins.
type settingsResponse struct {
	Version                   string `json:"version"`
	StorageDriver             string `json:"storage_driver"`
	CacheDriver               string `json:"cache_driver"`
	LocalMode                 bool   `json:"local_mode"`
	DemoMode                  bool   `json:"demo_mode"`
	InCluster                 bool   `json:"in_cluster"`
	WorkerIntervalSeconds     int    `json:"worker_interval_seconds"`
	NodeMetricsEnabled        bool   `json:"node_metrics_enabled"`
	NodeMetricsRetentionHours int    `json:"node_metrics_retention_hours"`
	MCPAllowWrites            bool   `json:"mcp_allow_writes"`
}

// GetSettings returns runtime configuration (admin only). No secrets are
// included (JWT secret, DB URL, SSH/registry credentials are never exposed).
// GET /api/v1/settings
func (h *SettingsHandler) GetSettings(c *gin.Context) {
	c.JSON(http.StatusOK, settingsResponse{
		Version:                   h.version,
		StorageDriver:             h.cfg.StorageDriver,
		CacheDriver:               h.cfg.CacheDriver,
		LocalMode:                 h.cfg.LocalMode,
		DemoMode:                  h.cfg.DemoMode,
		InCluster:                 h.cfg.InCluster,
		WorkerIntervalSeconds:     h.cfg.WorkerInterval,
		NodeMetricsEnabled:        h.cfg.NodeMetricsEnabled,
		NodeMetricsRetentionHours: h.cfg.NodeMetricsRetentionHours,
		MCPAllowWrites:            h.cfg.MCPAllowWrites,
	})
}
