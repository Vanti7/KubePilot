package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
	"gorm.io/gorm"
)

// CreateExceptionRule adds a new exception rule.
func (s *Store) CreateExceptionRule(ctx context.Context, r *models.ExceptionRule) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return s.DB.WithContext(ctx).Create(r).Error
}

// ListExceptionRules returns every exception rule (active, inactive and
// expired alike) for the management page — unlike ListActiveExceptionRules,
// which the scoring engine uses and which only returns rules that currently
// apply. Cluster/Workload are preloaded so the UI can show names, not just IDs.
func (s *Store) ListExceptionRules(ctx context.Context) ([]models.ExceptionRule, error) {
	var rules []models.ExceptionRule
	err := s.DB.WithContext(ctx).
		Preload("Cluster").
		Preload("Workload").
		Order("created_at DESC").
		Find(&rules).Error
	return rules, err
}

// GetExceptionRule retrieves a single exception rule by ID.
func (s *Store) GetExceptionRule(ctx context.Context, id string) (*models.ExceptionRule, error) {
	var rule models.ExceptionRule
	err := s.DB.WithContext(ctx).Preload("Cluster").Preload("Workload").
		Where("id = ?", id).First(&rule).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &rule, nil
}

// UpdateExceptionRule replaces an exception rule's editable fields.
func (s *Store) UpdateExceptionRule(ctx context.Context, r *models.ExceptionRule) error {
	return s.DB.WithContext(ctx).Save(r).Error
}

// DeleteExceptionRule removes an exception rule.
func (s *Store) DeleteExceptionRule(ctx context.Context, id string) error {
	result := s.DB.WithContext(ctx).Delete(&models.ExceptionRule{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
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
