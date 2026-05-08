package store

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Store holds the database and cache clients used across the application.
type Store struct {
	DB     *gorm.DB
	Redis  *redis.Client
	logger *zap.Logger
}

// NewStore creates a new Store with the given DB and Redis clients.
func NewStore(db *gorm.DB, rdb *redis.Client, logger *zap.Logger) *Store {
	return &Store{
		DB:     db,
		Redis:  rdb,
		logger: logger,
	}
}

// Health checks connectivity for both PostgreSQL and Redis.
func (s *Store) Health(ctx context.Context) error {
	sqlDB, err := s.DB.DB()
	if err != nil {
		return fmt.Errorf("get sql.DB: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("postgres ping: %w", err)
	}
	if err := s.Redis.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}
	return nil
}
