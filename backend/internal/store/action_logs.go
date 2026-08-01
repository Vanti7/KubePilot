package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
)

// ActionLogFilter holds optional filters for listing action-log entries.
type ActionLogFilter struct {
	UserID     string
	Action     string
	EntityType string
	EntityID   string
	Status     string
	Limit      int
	Offset     int
}

// ListActionLogs returns audit-trail entries matching the filter, newest
// first, with total count.
func (s *Store) ListActionLogs(ctx context.Context, filter ActionLogFilter) ([]models.ActionLog, int64, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	q := s.DB.WithContext(ctx).Model(&models.ActionLog{})
	if filter.UserID != "" {
		q = q.Where("user_id = ?", filter.UserID)
	}
	if filter.Action != "" {
		q = q.Where("action = ?", filter.Action)
	}
	if filter.EntityType != "" {
		q = q.Where("entity_type = ?", filter.EntityType)
	}
	if filter.EntityID != "" {
		q = q.Where("entity_id = ?", filter.EntityID)
	}
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []models.ActionLog
	err := q.Preload("User").
		Order("created_at DESC").
		Limit(limit).
		Offset(filter.Offset).
		Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// CreateActionLog inserts a single audit-trail row. Framework-agnostic — the
// gin-aware caller (handlers.RecordAction) fills in request-derived fields
// (IP, User-Agent, user id) before calling this.
func (s *Store) CreateActionLog(ctx context.Context, entry *models.ActionLog) error {
	if entry.ID == uuid.Nil {
		entry.ID = uuid.New()
	}
	return s.DB.WithContext(ctx).Create(entry).Error
}
