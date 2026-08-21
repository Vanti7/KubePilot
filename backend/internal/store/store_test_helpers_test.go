package store

import (
	"path/filepath"
	"testing"

	"github.com/kubepilot/backend/internal/models"
	"go.uber.org/zap"
)

// newFullTestStore opens a fresh SQLite-backed Store with the entire schema
// migrated (models.AllModels(), the same set used at real startup). Tests
// that touch more than a table or two use this instead of hand-picking a
// migration list per file.
func newFullTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := OpenSQLite(dbPath, false, zap.NewNop())
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
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
