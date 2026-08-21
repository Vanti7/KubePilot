package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
	"gorm.io/gorm"
)

// CreateExceptionRule adds a new exception rule. isActive is a separate
// parameter rather than read off r.IsActive, on purpose:
//
// IsActive has a gorm "default:true" tag, and GORM's struct-based Create
// substitutes the tag's parsed default for ANY field whose Go value is its
// zero value at insert time — for a plain bool that means false is
// indistinguishable from "field never set", so there is no way to recover
// the caller's real intent from r.IsActive alone after the fact (Select/Omit
// does not change this — it's driven purely by the zero-value check, not by
// column selection; an earlier version of this function tried inferring
// intent from r.IsActive before calling Create and got it backwards for
// every caller that simply left the field unset expecting the default,
// which every test in internal/scoring/engine_test.go does — that version
// is why isActive is now a required, unambiguous argument instead).
func (s *Store) CreateExceptionRule(ctx context.Context, r *models.ExceptionRule, isActive bool) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	if err := s.DB.WithContext(ctx).Create(r).Error; err != nil {
		return err
	}
	if !isActive {
		r.IsActive = false
		return s.DB.WithContext(ctx).Model(r).Update("is_active", false).Error
	}
	r.IsActive = true
	return nil
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
