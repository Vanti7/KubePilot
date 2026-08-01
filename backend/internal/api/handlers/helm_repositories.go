package handlers

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/backend/internal/models"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gopkg.in/yaml.v3"
)

// HelmRepositoryHandler handles chart repository configuration endpoints.
type HelmRepositoryHandler struct {
	store  *store.Store
	logger *zap.Logger
}

// NewHelmRepositoryHandler creates a new HelmRepositoryHandler.
func NewHelmRepositoryHandler(s *store.Store, logger *zap.Logger) *HelmRepositoryHandler {
	return &HelmRepositoryHandler{store: s, logger: logger}
}

// helmRepositoryDTO is the API shape — credentials are never returned, only
// whether they are configured and the username.
type helmRepositoryDTO struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	URL            string    `json:"url"`
	Username       string    `json:"username,omitempty"`
	HasCredentials bool      `json:"has_credentials"`
	TLSInsecure    bool      `json:"tls_insecure"`
	BuiltIn        bool      `json:"built_in"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func helmRepoAuthConfig(r models.HelmRepository) (username, password string) {
	if len(r.AuthConfig) == 0 {
		return "", ""
	}
	var cfg map[string]string
	if err := json.Unmarshal(r.AuthConfig, &cfg); err != nil {
		return "", ""
	}
	return cfg["username"], cfg["password"]
}

func helmRepositoryToDTO(r models.HelmRepository) helmRepositoryDTO {
	user, pass := helmRepoAuthConfig(r)
	return helmRepositoryDTO{
		ID:             r.ID.String(),
		Name:           r.Name,
		URL:            r.URL,
		Username:       user,
		HasCredentials: user != "" || pass != "",
		TLSInsecure:    r.TLSInsecure,
		BuiltIn:        r.BuiltIn,
		CreatedAt:      r.CreatedAt,
		UpdatedAt:      r.UpdatedAt,
	}
}

// helmRepositoryRequest is the create/update payload.
type helmRepositoryRequest struct {
	Name        string `json:"name"         binding:"required"`
	URL         string `json:"url"          binding:"required"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	TLSInsecure bool   `json:"tls_insecure"`
}

// ListHelmRepositories returns all configured chart repositories.
// GET /api/v1/helm-repositories
func (h *HelmRepositoryHandler) ListHelmRepositories(c *gin.Context) {
	repos, err := h.store.ListHelmRepositories(c.Request.Context())
	if err != nil {
		h.logger.Error("list helm repositories", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list helm repositories"})
		return
	}
	out := make([]helmRepositoryDTO, len(repos))
	for i, r := range repos {
		out[i] = helmRepositoryToDTO(r)
	}
	c.JSON(http.StatusOK, out)
}

// CreateHelmRepository adds a new chart repository.
// POST /api/v1/helm-repositories
func (h *HelmRepositoryHandler) CreateHelmRepository(c *gin.Context) {
	var req helmRepositoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	repo := &models.HelmRepository{
		Name:        req.Name,
		URL:         strings.TrimRight(req.URL, "/"),
		AuthConfig:  buildAuthConfig(req.Username, req.Password),
		TLSInsecure: req.TLSInsecure,
	}
	if err := h.store.CreateHelmRepository(c.Request.Context(), repo); err != nil {
		h.logger.Error("create helm repository", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create helm repository"})
		return
	}
	c.JSON(http.StatusCreated, helmRepositoryToDTO(*repo))
}

// UpdateHelmRepository edits an existing chart repository. An empty password
// preserves the stored credentials.
// PUT /api/v1/helm-repositories/:id
func (h *HelmRepositoryHandler) UpdateHelmRepository(c *gin.Context) {
	id := c.Param("id")
	var req helmRepositoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	repo, err := h.store.GetHelmRepository(c.Request.Context(), id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "helm repository not found"})
			return
		}
		h.logger.Error("get helm repository", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load helm repository"})
		return
	}

	repo.Name = req.Name
	repo.URL = strings.TrimRight(req.URL, "/")
	repo.TLSInsecure = req.TLSInsecure
	// Keep existing password when none is supplied.
	if req.Password != "" {
		repo.AuthConfig = buildAuthConfig(req.Username, req.Password)
	} else {
		_, existingPass := helmRepoAuthConfig(*repo)
		repo.AuthConfig = buildAuthConfig(req.Username, existingPass)
	}

	if err := h.store.UpdateHelmRepository(c.Request.Context(), repo); err != nil {
		h.logger.Error("update helm repository", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update helm repository"})
		return
	}
	c.JSON(http.StatusOK, helmRepositoryToDTO(*repo))
}

// DeleteHelmRepository removes a chart repository.
// DELETE /api/v1/helm-repositories/:id
func (h *HelmRepositoryHandler) DeleteHelmRepository(c *gin.Context) {
	id := c.Param("id")
	if err := h.store.DeleteHelmRepository(c.Request.Context(), id); err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "helm repository not found"})
			return
		}
		h.logger.Error("delete helm repository", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete helm repository"})
		return
	}
	c.Status(http.StatusNoContent)
}

// TestHelmRepository fetches the repository index and reports how many charts it
// publishes.
// POST /api/v1/helm-repositories/:id/test
func (h *HelmRepositoryHandler) TestHelmRepository(c *gin.Context) {
	id := c.Param("id")
	repo, err := h.store.GetHelmRepository(c.Request.Context(), id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "helm repository not found"})
			return
		}
		h.logger.Error("get helm repository", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load helm repository"})
		return
	}

	charts, msg, ok := probeHelmRepository(c.Request.Context(), *repo)
	c.JSON(http.StatusOK, gin.H{
		"ok":      ok,
		"charts":  charts,
		"message": msg,
	})
}

// probeHelmRepository downloads index.yaml and counts the charts it lists.
func probeHelmRepository(ctx context.Context, r models.HelmRepository) (int, string, bool) {
	client := &http.Client{Timeout: 20 * time.Second}
	if r.TLSInsecure {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	}

	indexURL := strings.TrimRight(r.URL, "/") + "/index.yaml"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return 0, err.Error(), false
	}
	if user, pass := helmRepoAuthConfig(r); user != "" {
		req.SetBasicAuth(user, pass)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, err.Error(), false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, "Repository returned HTTP " + http.StatusText(resp.StatusCode), false
	}

	var idx struct {
		Entries map[string][]struct{} `yaml:"entries"`
	}
	if err := yaml.NewDecoder(resp.Body).Decode(&idx); err != nil {
		return 0, "index.yaml is not a valid Helm repository index", false
	}

	return len(idx.Entries), "Repository reachable", true
}
