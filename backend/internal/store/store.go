package store

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Store holds the database and cache clients used across the application.
// Cache is an abstraction over Redis (production) or an in-process map
// (local / single-binary mode).
type Store struct {
	DB     *gorm.DB
	Cache  Cache
	logger *zap.Logger
}

// NewStore creates a new Store with the given DB and cache.
func NewStore(db *gorm.DB, cache Cache, logger *zap.Logger) *Store {
	return &Store{
		DB:     db,
		Cache:  cache,
		logger: logger,
	}
}

// Health checks connectivity for both the database and the cache.
func (s *Store) Health(ctx context.Context) error {
	sqlDB, err := s.DB.DB()
	if err != nil {
		return fmt.Errorf("get sql.DB: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("database ping: %w", err)
	}
	if err := s.Cache.Ping(ctx); err != nil {
		return fmt.Errorf("cache ping: %w", err)
	}
	return nil
}
