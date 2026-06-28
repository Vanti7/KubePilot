package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
)

// ListRegistries returns all configured image registries ordered by name.
func (s *Store) ListRegistries(ctx context.Context) ([]models.ImageRegistry, error) {
	var registries []models.ImageRegistry
	return registries, s.DB.WithContext(ctx).Order("name ASC").Find(&registries).Error
}

// GetRegistry returns a single image registry by ID.
func (s *Store) GetRegistry(ctx context.Context, id string) (*models.ImageRegistry, error) {
	var registry models.ImageRegistry
	if err := s.DB.WithContext(ctx).Where("id = ?", id).First(&registry).Error; err != nil {
		return nil, err
	}
	return &registry, nil
}

// CreateRegistry inserts a new image registry and links any already-collected
// images on the same host to it.
func (s *Store) CreateRegistry(ctx context.Context, registry *models.ImageRegistry) error {
	if registry.ID == uuid.Nil {
		registry.ID = uuid.New()
	}
	if err := s.DB.WithContext(ctx).Create(registry).Error; err != nil {
		return err
	}
	return s.LinkImagesToRegistry(ctx, registry.Host, registry.ID)
}

// UpdateRegistry persists changes to an existing registry and re-links images
// (the host may have changed).
func (s *Store) UpdateRegistry(ctx context.Context, registry *models.ImageRegistry) error {
	if err := s.DB.WithContext(ctx).Save(registry).Error; err != nil {
		return err
	}
	return s.LinkImagesToRegistry(ctx, registry.Host, registry.ID)
}

// DeleteRegistry removes a registry and unlinks its images.
func (s *Store) DeleteRegistry(ctx context.Context, id string) error {
	if err := s.DB.WithContext(ctx).
		Model(&models.ContainerImage{}).
		Where("registry_id = ?", id).
		Update("registry_id", nil).Error; err != nil {
		return err
	}
	return s.DB.WithContext(ctx).Where("id = ?", id).Delete(&models.ImageRegistry{}).Error
}

// LinkImagesToRegistry points every container image on the given host at the
// registry, so the watcher picks up its credentials and TLS settings.
func (s *Store) LinkImagesToRegistry(ctx context.Context, host string, registryID uuid.UUID) error {
	return s.DB.WithContext(ctx).
		Model(&models.ContainerImage{}).
		Where("registry = ?", host).
		Update("registry_id", registryID).Error
}
