package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
	"gorm.io/gorm"
)

// ListClusters returns all clusters with their environments preloaded.
func (s *Store) ListClusters(ctx context.Context) ([]models.Cluster, error) {
	var clusters []models.Cluster
	result := s.DB.WithContext(ctx).
		Preload("Environment").
		Order("name ASC").
		Find(&clusters)
	return clusters, result.Error
}

// GetCluster retrieves a single cluster by ID.
func (s *Store) GetCluster(ctx context.Context, id string) (*models.Cluster, error) {
	var cluster models.Cluster
	result := s.DB.WithContext(ctx).
		Preload("Environment").
		Where("id = ?", id).
		First(&cluster)
	if result.Error != nil {
		return nil, result.Error
	}
	return &cluster, nil
}

// CreateCluster inserts a new cluster record.
func (s *Store) CreateCluster(ctx context.Context, cluster *models.Cluster) error {
	if cluster.ID == uuid.Nil {
		cluster.ID = uuid.New()
	}
	return s.DB.WithContext(ctx).Create(cluster).Error
}

// UpdateCluster saves all fields of the cluster to the database.
func (s *Store) UpdateCluster(ctx context.Context, cluster *models.Cluster) error {
	return s.DB.WithContext(ctx).Save(cluster).Error
}

// DeleteCluster removes a cluster by ID.
func (s *Store) DeleteCluster(ctx context.Context, id string) error {
	return s.DB.WithContext(ctx).
		Where("id = ?", id).
		Delete(&models.Cluster{}).Error
}

// UpdateClusterStatus updates status and last_seen_at for a cluster.
func (s *Store) UpdateClusterStatus(ctx context.Context, id, status string, lastSeenAt time.Time) error {
	return s.DB.WithContext(ctx).
		Model(&models.Cluster{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":       status,
			"last_seen_at": lastSeenAt,
			"updated_at":   time.Now(),
		}).Error
}

// GetClusterBySlug retrieves a cluster by its slug.
func (s *Store) GetClusterBySlug(ctx context.Context, slug string) (*models.Cluster, error) {
	var cluster models.Cluster
	result := s.DB.WithContext(ctx).
		Preload("Environment").
		Where("slug = ?", slug).
		First(&cluster)
	if result.Error != nil {
		return nil, result.Error
	}
	return &cluster, nil
}

// ListClustersForEnv returns all clusters belonging to the given environment.
func (s *Store) ListClustersForEnv(ctx context.Context, envID string) ([]models.Cluster, error) {
	var clusters []models.Cluster
	result := s.DB.WithContext(ctx).
		Preload("Environment").
		Where("environment_id = ?", envID).
		Order("name ASC").
		Find(&clusters)
	return clusters, result.Error
}

// UpsertNamespace inserts or updates a namespace record.
func (s *Store) UpsertNamespace(ctx context.Context, ns *models.Namespace) error {
	return s.DB.WithContext(ctx).
		Where(models.Namespace{ClusterID: ns.ClusterID, Name: ns.Name}).
		Assign(models.Namespace{
			Status:    ns.Status,
			Labels:    ns.Labels,
			UpdatedAt: time.Now(),
		}).
		FirstOrCreate(ns).Error
}

// UpsertNode inserts or updates a node record identified by cluster+name.
func (s *Store) UpsertNode(ctx context.Context, node *models.Node) error {
	if node.ID == uuid.Nil {
		node.ID = uuid.New()
	}
	return s.DB.WithContext(ctx).
		Where(models.Node{ClusterID: node.ClusterID, Name: node.Name}).
		Assign(node).
		FirstOrCreate(node).Error
}

// ListNodes returns nodes for a given cluster.
func (s *Store) ListNodes(ctx context.Context, clusterID string) ([]models.Node, error) {
	var nodes []models.Node
	q := s.DB.WithContext(ctx).Order("name ASC")
	if clusterID != "" {
		q = q.Where("cluster_id = ?", clusterID)
	}
	return nodes, q.Find(&nodes).Error
}

// GetNode returns a node by ID.
func (s *Store) GetNode(ctx context.Context, id string) (*models.Node, error) {
	var node models.Node
	result := s.DB.WithContext(ctx).
		Preload("Cluster").
		Where("id = ?", id).
		First(&node)
	if result.Error != nil {
		return nil, result.Error
	}
	return &node, nil
}

// DeleteNodesNotSeenSince removes stale node records for a cluster.
func (s *Store) DeleteNodesNotSeenSince(ctx context.Context, clusterID string, since time.Time) error {
	return s.DB.WithContext(ctx).
		Where("cluster_id = ? AND updated_at < ?", clusterID, since).
		Delete(&models.Node{}).Error
}

// GetOrCreateEnvironment retrieves or creates an environment by name.
func (s *Store) GetOrCreateEnvironment(ctx context.Context, name, slug string) (*models.Environment, error) {
	var env models.Environment
	result := s.DB.WithContext(ctx).
		Where(models.Environment{Slug: slug}).
		Attrs(models.Environment{
			ID:                uuid.New(),
			Name:              name,
			Slug:              slug,
			CriticalityWeight: 1.0,
			Color:             "#6B7280",
		}).
		FirstOrCreate(&env)
	if result.Error != nil {
		return nil, result.Error
	}
	return &env, nil
}

// ListEnvironments returns all environments.
func (s *Store) ListEnvironments(ctx context.Context) ([]models.Environment, error) {
	var envs []models.Environment
	result := s.DB.WithContext(ctx).Order("name ASC").Find(&envs)
	return envs, result.Error
}

// clusterExists is a helper to check if a cluster ID exists.
func (s *Store) clusterExists(ctx context.Context, id string) (bool, error) {
	var count int64
	err := s.DB.WithContext(ctx).
		Model(&models.Cluster{}).
		Where("id = ?", id).
		Count(&count).Error
	return count > 0, err
}

// UpdateK8sVersion updates just the k8s_version field for a cluster.
func (s *Store) UpdateK8sVersion(ctx context.Context, clusterID, version string) error {
	return s.DB.WithContext(ctx).
		Model(&models.Cluster{}).
		Where("id = ?", clusterID).
		Updates(map[string]interface{}{
			"k8s_version": version,
			"updated_at":  time.Now(),
		}).Error
}

// GetNamespaceByName retrieves a namespace by cluster and name.
func (s *Store) GetNamespaceByName(ctx context.Context, clusterID, name string) (*models.Namespace, error) {
	var ns models.Namespace
	result := s.DB.WithContext(ctx).
		Where("cluster_id = ? AND name = ?", clusterID, name).
		First(&ns)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, result.Error
	}
	return &ns, nil
}
