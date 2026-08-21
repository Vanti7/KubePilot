package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
	"gorm.io/gorm/clause"
)

// SecretFilter holds optional filters for listing secrets.
type SecretFilter struct {
	ClusterID     string
	NamespaceName string
	Type          string
	Limit         int
	Offset        int
}

// ListSecrets returns secrets matching the provided filter.
func (s *Store) ListSecrets(ctx context.Context, filter SecretFilter) ([]models.Secret, int64, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	q := s.DB.WithContext(ctx).Model(&models.Secret{}).Preload("Cluster")

	if filter.ClusterID != "" {
		q = q.Where("cluster_id = ?", filter.ClusterID)
	}
	if filter.NamespaceName != "" {
		q = q.Where("namespace_name = ?", filter.NamespaceName)
	}
	if filter.Type != "" {
		q = q.Where("type = ?", filter.Type)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var secrets []models.Secret
	err := q.Order("namespace_name ASC, name ASC").
		Limit(limit).
		Offset(filter.Offset).
		Find(&secrets).Error
	return secrets, total, err
}

// UpsertSecret inserts or updates a secret by cluster+namespace+name.
//
// Reuses the existing row's id first — same fix, same reason as
// UpsertWorkload/UpsertNode: ON CONFLICT DO UPDATE alone does not report the
// surviving row's id back to Go, so a freshly generated UUID would leave a
// caller that trusts secret.ID afterward holding a phantom id.
func (s *Store) UpsertSecret(ctx context.Context, secret *models.Secret) error {
	var existing []uuid.UUID
	if err := s.DB.WithContext(ctx).Model(&models.Secret{}).
		Where("cluster_id = ? AND namespace_name = ? AND name = ?", secret.ClusterID, secret.NamespaceName, secret.Name).
		Limit(1).Pluck("id", &existing).Error; err != nil {
		return err
	}
	switch {
	case len(existing) > 0:
		secret.ID = existing[0]
	case secret.ID == uuid.Nil:
		secret.ID = uuid.New()
	}
	secret.LastSeenAt = time.Now()

	return s.DB.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "cluster_id"},
				{Name: "namespace_name"},
				{Name: "name"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"type",
				"keys",
				"k8s_created_at",
				"k8s_updated_at",
				"last_seen_at",
				"updated_at",
			}),
		}).
		Create(secret).Error
}

// DeleteSecretsNotSeenSince removes stale secrets for a cluster.
func (s *Store) DeleteSecretsNotSeenSince(ctx context.Context, clusterID string, since time.Time) error {
	return s.DB.WithContext(ctx).
		Where("cluster_id = ? AND last_seen_at < ?", clusterID, since).
		Delete(&models.Secret{}).Error
}
