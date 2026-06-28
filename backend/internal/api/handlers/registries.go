package handlers

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/backend/internal/models"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// RegistryHandler handles image registry configuration endpoints.
type RegistryHandler struct {
	store  *store.Store
	logger *zap.Logger
}

// NewRegistryHandler creates a new RegistryHandler.
func NewRegistryHandler(s *store.Store, logger *zap.Logger) *RegistryHandler {
	return &RegistryHandler{store: s, logger: logger}
}

// registryDTO is the API shape — credentials are never returned, only whether
// they are configured and the username.
type registryDTO struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Host           string    `json:"host"`
	Type           string    `json:"type"`
	Username       string    `json:"username,omitempty"`
	HasCredentials bool      `json:"has_credentials"`
	TLSInsecure    bool      `json:"tls_insecure"`
	RateLimitRPM   int       `json:"rate_limit_rpm"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func authConfig(r models.ImageRegistry) (username, password string) {
	if len(r.AuthConfig) == 0 {
		return "", ""
	}
	var cfg map[string]string
	if err := json.Unmarshal(r.AuthConfig, &cfg); err != nil {
		return "", ""
	}
	return cfg["username"], cfg["password"]
}

func registryToDTO(r models.ImageRegistry) registryDTO {
	user, pass := authConfig(r)
	return registryDTO{
		ID:             r.ID.String(),
		Name:           r.Name,
		Host:           r.Host,
		Type:           r.Type,
		Username:       user,
		HasCredentials: user != "" || pass != "",
		TLSInsecure:    r.TLSInsecure,
		RateLimitRPM:   r.RateLimitRPM,
		CreatedAt:      r.CreatedAt,
		UpdatedAt:      r.UpdatedAt,
	}
}

// registryRequest is the create/update payload.
type registryRequest struct {
	Name         string `json:"name"         binding:"required"`
	Host         string `json:"host"         binding:"required"`
	Type         string `json:"type"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	TLSInsecure  bool   `json:"tls_insecure"`
	RateLimitRPM int    `json:"rate_limit_rpm"`
}

func buildAuthConfig(username, password string) datatypes.JSON {
	cfg := map[string]string{}
	if username != "" {
		cfg["username"] = username
	}
	if password != "" {
		cfg["password"] = password
	}
	b, _ := json.Marshal(cfg)
	return datatypes.JSON(b)
}

// ListRegistries returns all configured registries.
// GET /api/v1/registries
func (h *RegistryHandler) ListRegistries(c *gin.Context) {
	registries, err := h.store.ListRegistries(c.Request.Context())
	if err != nil {
		h.logger.Error("list registries", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list registries"})
		return
	}
	out := make([]registryDTO, len(registries))
	for i, r := range registries {
		out[i] = registryToDTO(r)
	}
	c.JSON(http.StatusOK, out)
}

// CreateRegistry adds a new registry.
// POST /api/v1/registries
func (h *RegistryHandler) CreateRegistry(c *gin.Context) {
	var req registryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	registry := &models.ImageRegistry{
		Name:         req.Name,
		Host:         req.Host,
		Type:         defaultStr(req.Type, "generic"),
		AuthConfig:   buildAuthConfig(req.Username, req.Password),
		TLSInsecure:  req.TLSInsecure,
		RateLimitRPM: defaultInt(req.RateLimitRPM, 60),
	}
	if err := h.store.CreateRegistry(c.Request.Context(), registry); err != nil {
		h.logger.Error("create registry", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create registry"})
		return
	}
	c.JSON(http.StatusCreated, registryToDTO(*registry))
}

// UpdateRegistry edits an existing registry. An empty password preserves the
// stored credentials.
// PUT /api/v1/registries/:id
func (h *RegistryHandler) UpdateRegistry(c *gin.Context) {
	id := c.Param("id")
	var req registryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	registry, err := h.store.GetRegistry(c.Request.Context(), id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "registry not found"})
			return
		}
		h.logger.Error("get registry", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load registry"})
		return
	}

	registry.Name = req.Name
	registry.Host = req.Host
	registry.Type = defaultStr(req.Type, "generic")
	registry.TLSInsecure = req.TLSInsecure
	registry.RateLimitRPM = defaultInt(req.RateLimitRPM, 60)
	// Keep existing password when none is supplied.
	if req.Password != "" {
		registry.AuthConfig = buildAuthConfig(req.Username, req.Password)
	} else {
		_, existingPass := authConfig(*registry)
		registry.AuthConfig = buildAuthConfig(req.Username, existingPass)
	}

	if err := h.store.UpdateRegistry(c.Request.Context(), registry); err != nil {
		h.logger.Error("update registry", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update registry"})
		return
	}
	c.JSON(http.StatusOK, registryToDTO(*registry))
}

// DeleteRegistry removes a registry.
// DELETE /api/v1/registries/:id
func (h *RegistryHandler) DeleteRegistry(c *gin.Context) {
	id := c.Param("id")
	if err := h.store.DeleteRegistry(c.Request.Context(), id); err != nil {
		h.logger.Error("delete registry", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete registry"})
		return
	}
	c.Status(http.StatusNoContent)
}

// TestRegistry probes the registry's /v2/ endpoint with the stored credentials.
// POST /api/v1/registries/:id/test
func (h *RegistryHandler) TestRegistry(c *gin.Context) {
	id := c.Param("id")
	registry, err := h.store.GetRegistry(c.Request.Context(), id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "registry not found"})
			return
		}
		h.logger.Error("get registry", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load registry"})
		return
	}

	status, reachable, msg := probeRegistry(c.Request.Context(), *registry)
	c.JSON(http.StatusOK, gin.H{
		"ok":          reachable,
		"status_code": status,
		"message":     msg,
	})
}

// probeRegistry performs a GET on https://<host>/v2/. A 200 or 401 both mean the
// registry is reachable and speaks the OCI distribution API.
func probeRegistry(ctx context.Context, r models.ImageRegistry) (int, bool, string) {
	client := &http.Client{Timeout: 15 * time.Second}
	if r.TLSInsecure {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	}

	url := fmt.Sprintf("https://%s/v2/", r.Host)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, false, err.Error()
	}
	if user, pass := authConfig(r); user != "" {
		req.SetBasicAuth(user, pass)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, false, err.Error()
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return resp.StatusCode, true, "Registry reachable and authenticated"
	case http.StatusUnauthorized:
		return resp.StatusCode, true, "Registry reachable (anonymous access denied — credentials required for listing)"
	default:
		return resp.StatusCode, false, fmt.Sprintf("Registry returned HTTP %d", resp.StatusCode)
	}
}

func defaultStr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func defaultInt(v, fallback int) int {
	if v == 0 {
		return fallback
	}
	return v
}
