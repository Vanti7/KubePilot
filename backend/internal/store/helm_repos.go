package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
)

// ListHelmRepositories returns all configured chart repositories ordered by name.
func (s *Store) ListHelmRepositories(ctx context.Context) ([]models.HelmRepository, error) {
	var repos []models.HelmRepository
	return repos, s.DB.WithContext(ctx).Order("name ASC").Find(&repos).Error
}

// GetHelmRepository returns a single chart repository by ID.
func (s *Store) GetHelmRepository(ctx context.Context, id string) (*models.HelmRepository, error) {
	var repo models.HelmRepository
	if err := s.DB.WithContext(ctx).Where("id = ?", id).First(&repo).Error; err != nil {
		return nil, err
	}
	return &repo, nil
}

// GetHelmRepositoryByURL returns the configured repository for a URL, or nil
// when the URL was autodiscovered rather than configured.
func (s *Store) GetHelmRepositoryByURL(ctx context.Context, repoURL string) (*models.HelmRepository, error) {
	var repo models.HelmRepository
	if err := s.DB.WithContext(ctx).Where("url = ?", repoURL).First(&repo).Error; err != nil {
		return nil, err
	}
	return &repo, nil
}

// CreateHelmRepository inserts a new chart repository.
func (s *Store) CreateHelmRepository(ctx context.Context, repo *models.HelmRepository) error {
	if repo.ID == uuid.Nil {
		repo.ID = uuid.New()
	}
	return s.DB.WithContext(ctx).Create(repo).Error
}

// UpdateHelmRepository persists changes to an existing chart repository.
func (s *Store) UpdateHelmRepository(ctx context.Context, repo *models.HelmRepository) error {
	return s.DB.WithContext(ctx).Save(repo).Error
}

// DeleteHelmRepository removes a chart repository and clears the resolved repo
// URL from the releases that pointed at it, so they are resolved again on the
// next watcher pass.
func (s *Store) DeleteHelmRepository(ctx context.Context, id string) error {
	repo, err := s.GetHelmRepository(ctx, id)
	if err != nil {
		return err
	}
	if err := s.DB.WithContext(ctx).
		Model(&models.HelmRelease{}).
		Where("repo_url = ?", repo.URL).
		Update("repo_url", "").Error; err != nil {
		return err
	}
	return s.DB.WithContext(ctx).Where("id = ?", id).Delete(&models.HelmRepository{}).Error
}

// SetHelmReleaseRepoURL records the repository a release was resolved to.
func (s *Store) SetHelmReleaseRepoURL(ctx context.Context, id uuid.UUID, repoURL string) error {
	return s.DB.WithContext(ctx).
		Model(&models.HelmRelease{}).
		Where("id = ?", id).
		Update("repo_url", repoURL).Error
}
