package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
)

// ListIntegrations returns all integration accounts ordered by name.
func (s *Store) ListIntegrations(ctx context.Context) ([]models.IntegrationAccount, error) {
	var integrations []models.IntegrationAccount
	return integrations, s.DB.WithContext(ctx).Order("name ASC").Find(&integrations).Error
}

// GetIntegration returns a single integration account by ID.
func (s *Store) GetIntegration(ctx context.Context, id string) (*models.IntegrationAccount, error) {
	var integration models.IntegrationAccount
	result := s.DB.WithContext(ctx).Where("id = ?", id).First(&integration)
	return &integration, result.Error
}

// CreateIntegration inserts a new integration account.
func (s *Store) CreateIntegration(ctx context.Context, integration *models.IntegrationAccount) error {
	if integration.ID == uuid.Nil {
		integration.ID = uuid.New()
	}
	return s.DB.WithContext(ctx).Create(integration).Error
}

// DeleteIntegration removes an integration account by ID.
func (s *Store) DeleteIntegration(ctx context.Context, id string) error {
	return s.DB.WithContext(ctx).
		Where("id = ?", id).
		Delete(&models.IntegrationAccount{}).Error
}

// UpdateIntegrationSyncTime sets last_sync_at to now for the given integration.
func (s *Store) UpdateIntegrationSyncTime(ctx context.Context, id string) error {
	now := time.Now()
	return s.DB.WithContext(ctx).
		Model(&models.IntegrationAccount{}).
		Where("id = ?", id).
		Update("last_sync_at", now).Error
}
