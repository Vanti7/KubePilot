package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/backend/internal/models"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// IntegrationHandler handles integration account endpoints.
type IntegrationHandler struct {
	store  *store.Store
	logger *zap.Logger
}

// NewIntegrationHandler creates a new IntegrationHandler.
func NewIntegrationHandler(s *store.Store, logger *zap.Logger) *IntegrationHandler {
	return &IntegrationHandler{store: s, logger: logger}
}

// integrationDTO is the response shape expected by the frontend.
type integrationDTO struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	IntegrationType string     `json:"integration_type"`
	Enabled         bool       `json:"enabled"`
	LastTestedAt    *time.Time `json:"last_tested_at,omitempty"`
	LastTestStatus  string     `json:"last_test_status"`
	URL             string     `json:"url,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// integrationToDTO maps a model to the frontend DTO, renaming fields as needed.
func integrationToDTO(a models.IntegrationAccount) integrationDTO {
	var url string
	if len(a.Config) > 0 {
		var cfg map[string]string
		if err := json.Unmarshal(a.Config, &cfg); err == nil {
			url = cfg["url"]
		}
	}
	status := "not_tested"
	if a.LastSyncAt != nil {
		status = "ok"
	}
	return integrationDTO{
		ID:              a.ID.String(),
		Name:            a.Name,
		IntegrationType: a.Type,
		Enabled:         a.IsEnabled,
		LastTestedAt:    a.LastSyncAt,
		LastTestStatus:  status,
		URL:             url,
		CreatedAt:       a.CreatedAt,
		UpdatedAt:       a.UpdatedAt,
	}
}

// ListIntegrations returns all configured integrations.
// GET /api/v1/integrations
func (h *IntegrationHandler) ListIntegrations(c *gin.Context) {
	integrations, err := h.store.ListIntegrations(c.Request.Context())
	if err != nil {
		h.logger.Error("list integrations", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list integrations"})
		return
	}

	out := make([]integrationDTO, len(integrations))
	for i, a := range integrations {
		out[i] = integrationToDTO(a)
	}
	c.JSON(http.StatusOK, out)
}

// CreateIntegration adds a new integration account.
// POST /api/v1/integrations
func (h *IntegrationHandler) CreateIntegration(c *gin.Context) {
	var req struct {
		Name            string `json:"name"             binding:"required"`
		IntegrationType string `json:"integration_type" binding:"required"`
		URL             string `json:"url"`
		CredentialsRef  string `json:"credentials_ref"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cfg := map[string]string{}
	if req.URL != "" {
		cfg["url"] = req.URL
	}
	cfgJSON, _ := json.Marshal(cfg)

	integration := &models.IntegrationAccount{
		Name:      req.Name,
		Type:      req.IntegrationType,
		Config:    datatypes.JSON(cfgJSON),
		SecretRef: req.CredentialsRef,
		IsEnabled: true,
	}

	if err := h.store.CreateIntegration(c.Request.Context(), integration); err != nil {
		h.logger.Error("create integration", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create integration"})
		return
	}

	c.JSON(http.StatusCreated, integrationToDTO(*integration))
}

// TestIntegration performs a connectivity test for the integration.
// POST /api/v1/integrations/:id/test
// For MVP, this simply records the test time. Real dispatch logic is V2.
func (h *IntegrationHandler) TestIntegration(c *gin.Context) {
	id := c.Param("id")

	if err := h.store.UpdateIntegrationSyncTime(c.Request.Context(), id); err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "integration not found"})
			return
		}
		h.logger.Error("test integration", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to test integration"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Test recorded — notification dispatch coming in V2"})
}

// DeleteIntegration removes an integration account.
// DELETE /api/v1/integrations/:id
func (h *IntegrationHandler) DeleteIntegration(c *gin.Context) {
	id := c.Param("id")

	if err := h.store.DeleteIntegration(c.Request.Context(), id); err != nil {
		h.logger.Error("delete integration", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete integration"})
		return
	}

	c.Status(http.StatusNoContent)
}
