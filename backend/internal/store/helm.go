package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
	"gorm.io/gorm/clause"
)

// HelmFilter holds optional filters for listing Helm releases.
type HelmFilter struct {
	ClusterID     string
	NamespaceName string
	Status        string
	Limit         int
	Offset        int
}

// ListHelmReleases returns Helm releases matching the filter.
func (s *Store) ListHelmReleases(ctx context.Context, filter HelmFilter) ([]models.HelmRelease, int64, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	q := s.DB.WithContext(ctx).Model(&models.HelmRelease{}).
		Preload("Cluster")

	if filter.ClusterID != "" {
		q = q.Where("cluster_id = ?", filter.ClusterID)
	}
	if filter.NamespaceName != "" {
		q = q.Where("namespace_name = ?", filter.NamespaceName)
	}
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var releases []models.HelmRelease
	err := q.Order("name ASC").
		Limit(limit).
		Offset(filter.Offset).
		Find(&releases).Error
	return releases, total, err
}

// GetHelmRelease retrieves a single Helm release by ID.
func (s *Store) GetHelmRelease(ctx context.Context, id string) (*models.HelmRelease, error) {
	var release models.HelmRelease
	result := s.DB.WithContext(ctx).
		Preload("Cluster").
		Where("id = ?", id).
		First(&release)
	if result.Error != nil {
		return nil, result.Error
	}
	return &release, nil
}

// UpsertHelmRelease inserts or updates a Helm release by cluster+namespace+name.
func (s *Store) UpsertHelmRelease(ctx context.Context, release *models.HelmRelease) error {
	if release.ID == uuid.Nil {
		release.ID = uuid.New()
	}
	release.LastSeenAt = time.Now()

	return s.DB.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "cluster_id"},
				{Name: "namespace_name"},
				{Name: "name"},
			},
			// repo_url is deliberately absent: it is resolved by the Helm watcher,
			// not by the collector (a Helm release secret does not record its
			// origin), so re-collecting must not wipe it.
			DoUpdates: clause.AssignmentColumns([]string{
				"chart_name",
				"chart_version",
				"app_version",
				"status",
				"revision",
				"values",
				"last_deployed_at",
				"last_seen_at",
				"updated_at",
			}),
		}).
		Create(release).Error
}

// DeleteHelmReleasesNotSeenSince removes stale Helm releases for a cluster.
func (s *Store) DeleteHelmReleasesNotSeenSince(ctx context.Context, clusterID string, since time.Time) error {
	return s.DB.WithContext(ctx).
		Where("cluster_id = ? AND last_seen_at < ?", clusterID, since).
		Delete(&models.HelmRelease{}).Error
}

// ListAllHelmReleases returns every Helm release (used by the helm watcher,
// which resolves the repository itself when repo_url is still empty).
func (s *Store) ListAllHelmReleases(ctx context.Context) ([]models.HelmRelease, error) {
	var releases []models.HelmRelease
	result := s.DB.WithContext(ctx).Find(&releases)
	return releases, result.Error
}

// GetHelmReleaseByClusterAndName retrieves a Helm release by cluster+namespace+name.
func (s *Store) GetHelmReleaseByClusterAndName(ctx context.Context, clusterID, namespace, name string) (*models.HelmRelease, error) {
	var release models.HelmRelease
	result := s.DB.WithContext(ctx).
		Where("cluster_id = ? AND namespace_name = ? AND name = ?", clusterID, namespace, name).
		First(&release)
	if result.Error != nil {
		return nil, result.Error
	}
	return &release, nil
}
