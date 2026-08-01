package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
	"go.uber.org/zap"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := OpenSQLite(dbPath, false, zap.NewNop())
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.ActionLog{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	t.Cleanup(func() {
		// Windows can't remove the TempDir's db file on cleanup while the
		// pool still holds it open.
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return NewStore(db, nil, zap.NewNop())
}

func TestCreateAndListActionLogs(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userID := uuid.New()
	entries := []models.ActionLog{
		{UserID: &userID, Action: "scale", EntityType: "workload", EntityID: "w1", Status: models.ActionLogStatusSuccess},
		{UserID: &userID, Action: "scale", EntityType: "workload", EntityID: "w2", Status: models.ActionLogStatusFailure},
		{Action: "sync", EntityType: "cluster", EntityID: "c1", Status: models.ActionLogStatusSuccess},
	}
	for i := range entries {
		if err := s.CreateActionLog(ctx, &entries[i]); err != nil {
			t.Fatalf("create action log %d: %v", i, err)
		}
		// Ensure distinct, increasing CreatedAt for a deterministic DESC order
		// (SQLite's default timestamp precision can otherwise collide within
		// a fast test run).
		time.Sleep(2 * time.Millisecond)
	}

	t.Run("no filter returns all, newest first", func(t *testing.T) {
		rows, total, err := s.ListActionLogs(ctx, ActionLogFilter{})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 3 {
			t.Fatalf("total = %d, want 3", total)
		}
		if len(rows) != 3 {
			t.Fatalf("len(rows) = %d, want 3", len(rows))
		}
		if rows[0].EntityID != "c1" || rows[2].EntityID != "w1" {
			t.Fatalf("unexpected order: %v", []string{rows[0].EntityID, rows[1].EntityID, rows[2].EntityID})
		}
	})

	t.Run("filter by status", func(t *testing.T) {
		rows, total, err := s.ListActionLogs(ctx, ActionLogFilter{Status: models.ActionLogStatusFailure})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 1 || len(rows) != 1 || rows[0].EntityID != "w2" {
			t.Fatalf("filter by status failed: total=%d rows=%v", total, rows)
		}
	})

	t.Run("filter by entity_type", func(t *testing.T) {
		rows, total, err := s.ListActionLogs(ctx, ActionLogFilter{EntityType: "workload"})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 2 || len(rows) != 2 {
			t.Fatalf("filter by entity_type failed: total=%d rows=%v", total, rows)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		rows, total, err := s.ListActionLogs(ctx, ActionLogFilter{Limit: 1, Offset: 1})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 3 {
			t.Fatalf("total = %d, want 3", total)
		}
		if len(rows) != 1 {
			t.Fatalf("len(rows) = %d, want 1", len(rows))
		}
	})
}
