package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
)

// CreateExceptionRule adds a new exception rule.
func (s *Store) CreateExceptionRule(ctx context.Context, r *models.ExceptionRule) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return s.DB.WithContext(ctx).Create(r).Error
}

// ListActiveExceptionRules returns exception rules that are active and not
// expired. Matching a specific finding against a rule's scope (workload,
// cluster/namespace, image pattern glob) happens in the caller
// (internal/scoring) — a glob match against the image reference can't be
// expressed in SQL, so all candidates are fetched and filtered in Go. Rule
// counts are expected to stay small at the scale this tool targets.
func (s *Store) ListActiveExceptionRules(ctx context.Context) ([]models.ExceptionRule, error) {
	var rules []models.ExceptionRule
	err := s.DB.WithContext(ctx).
		Where("is_active = ?", true).
		Where("expires_at IS NULL OR expires_at > ?", time.Now()).
		Find(&rules).Error
	return rules, err
}
