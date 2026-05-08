package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
	"gorm.io/gorm/clause"
)

// WorkloadFilter holds optional filters for listing workloads.
type WorkloadFilter struct {
	ClusterID     string
	NamespaceName string
	Kind          string
	HealthStatus  string
	Limit         int
	Offset        int
}

// ListWorkloads returns workloads matching the provided filter.
func (s *Store) ListWorkloads(ctx context.Context, filter WorkloadFilter) ([]models.Workload, int64, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	q := s.DB.WithContext(ctx).Model(&models.Workload{}).
		Preload("Cluster").
		Preload("Namespace")

	if filter.ClusterID != "" {
		q = q.Where("cluster_id = ?", filter.ClusterID)
	}
	if filter.NamespaceName != "" {
		q = q.Where("namespace_name = ?", filter.NamespaceName)
	}
	if filter.Kind != "" {
		q = q.Where("kind = ?", filter.Kind)
	}
	if filter.HealthStatus != "" {
		q = q.Where("health_status = ?", filter.HealthStatus)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var workloads []models.Workload
	err := q.Order("name ASC").
		Limit(limit).
		Offset(filter.Offset).
		Find(&workloads).Error
	return workloads, total, err
}

// GetWorkload retrieves a single workload by ID with container images.
func (s *Store) GetWorkload(ctx context.Context, id string) (*models.Workload, error) {
	var workload models.Workload
	result := s.DB.WithContext(ctx).
		Preload("Cluster").
		Preload("Namespace").
		Where("id = ?", id).
		First(&workload)
	if result.Error != nil {
		return nil, result.Error
	}
	return &workload, nil
}

// UpsertWorkload inserts or updates a workload by cluster+namespace+name+kind.
func (s *Store) UpsertWorkload(ctx context.Context, workload *models.Workload) error {
	if workload.ID == uuid.Nil {
		workload.ID = uuid.New()
	}
	workload.LastSeenAt = time.Now()

	return s.DB.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "cluster_id"},
				{Name: "namespace_name"},
				{Name: "name"},
				{Name: "kind"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"replicas_desired",
				"replicas_ready",
				"health_status",
				"labels",
				"annotations",
				"last_seen_at",
				"updated_at",
			}),
		}).
		Create(workload).Error
}

// DeleteWorkloadsNotSeenSince removes stale workloads for a cluster.
func (s *Store) DeleteWorkloadsNotSeenSince(ctx context.Context, clusterID string, since time.Time) error {
	return s.DB.WithContext(ctx).
		Where("cluster_id = ? AND last_seen_at < ?", clusterID, since).
		Delete(&models.Workload{}).Error
}

// ListContainerImages returns all container images for a workload.
func (s *Store) ListContainerImages(ctx context.Context, workloadID string) ([]models.ContainerImage, error) {
	var images []models.ContainerImage
	result := s.DB.WithContext(ctx).
		Where("workload_id = ?", workloadID).
		Find(&images)
	return images, result.Error
}

// UpsertContainerImage inserts or updates a container image by workload+container_name.
func (s *Store) UpsertContainerImage(ctx context.Context, image *models.ContainerImage) error {
	if image.ID == uuid.Nil {
		image.ID = uuid.New()
	}

	return s.DB.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "workload_id"},
				{Name: "container_name"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"image",
				"registry",
				"repository",
				"tag",
				"digest",
				"is_init_container",
				"registry_id",
				"updated_at",
			}),
		}).
		Create(image).Error
}

// GetContainerImage retrieves a container image by ID.
func (s *Store) GetContainerImage(ctx context.Context, id string) (*models.ContainerImage, error) {
	var image models.ContainerImage
	result := s.DB.WithContext(ctx).
		Preload("Workload").
		Preload("ImageRegistry").
		Where("id = ?", id).
		First(&image)
	if result.Error != nil {
		return nil, result.Error
	}
	return &image, nil
}

// ListAllContainerImages returns all container images (used by image watcher).
func (s *Store) ListAllContainerImages(ctx context.Context) ([]models.ContainerImage, error) {
	var images []models.ContainerImage
	result := s.DB.WithContext(ctx).
		Preload("ImageRegistry").
		Find(&images)
	return images, result.Error
}

// GetImageRegistry retrieves an image registry by host.
func (s *Store) GetImageRegistry(ctx context.Context, host string) (*models.ImageRegistry, error) {
	var registry models.ImageRegistry
	result := s.DB.WithContext(ctx).
		Where("host = ?", host).
		First(&registry)
	if result.Error != nil {
		return nil, result.Error
	}
	return &registry, nil
}

// UpsertImageTagObservation records a tag observation for an image.
func (s *Store) UpsertImageTagObservation(ctx context.Context, obs *models.ImageTagObservation) error {
	if obs.ID == uuid.Nil {
		obs.ID = uuid.New()
	}
	if obs.ObservedAt.IsZero() {
		obs.ObservedAt = time.Now()
	}

	return s.DB.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "image_id"},
				{Name: "tag"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"digest",
				"observed_at",
				"is_latest",
			}),
		}).
		Create(obs).Error
}

// ListWorkloadsByCluster returns all workloads for a cluster (used by collectors).
func (s *Store) ListWorkloadsByCluster(ctx context.Context, clusterID string) ([]models.Workload, error) {
	var workloads []models.Workload
	result := s.DB.WithContext(ctx).
		Where("cluster_id = ?", clusterID).
		Find(&workloads)
	return workloads, result.Error
}
