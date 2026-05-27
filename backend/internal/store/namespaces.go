package store

import (
	"context"

	"github.com/kubepilot/backend/internal/models"
)

// ListNamespaces returns all namespaces, optionally filtered by cluster ID.
func (s *Store) ListNamespaces(ctx context.Context, clusterID string) ([]models.Namespace, error) {
	var namespaces []models.Namespace
	q := s.DB.WithContext(ctx).Model(&models.Namespace{}).Order("name ASC")
	if clusterID != "" {
		q = q.Where("cluster_id = ?", clusterID)
	}
	return namespaces, q.Find(&namespaces).Error
}
