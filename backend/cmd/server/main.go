package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/backend/internal/api"
	"github.com/kubepilot/backend/internal/api/handlers"
	"github.com/kubepilot/backend/internal/bootstrap"
	"github.com/kubepilot/backend/internal/collector"
	"github.com/kubepilot/backend/internal/config"
	"github.com/kubepilot/backend/internal/models"
	"github.com/kubepilot/backend/internal/scoring"
	"github.com/kubepilot/backend/internal/store"
	"github.com/kubepilot/backend/internal/watcher"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func main() {
	cfg := config.Load()

	logger, err := buildLogger(cfg.LogLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync() //nolint:errcheck

	// -----------------------------------------------------------------------
	// Connect to PostgreSQL
	// -----------------------------------------------------------------------
	db, err := connectPostgres(cfg, logger)
	if err != nil {
		logger.Fatal("connect to postgres", zap.Error(err))
	}
	logger.Info("connected to postgres")

	// Run AutoMigrate for all models.
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		logger.Fatal("auto migrate", zap.Error(err))
	}
	logger.Info("database schema up to date")

	// -----------------------------------------------------------------------
	// Bootstrap (first-run: seed environments, create admin, register local cluster)
	// -----------------------------------------------------------------------
	if err := bootstrap.Run(context.Background(), db, cfg, logger); err != nil {
		logger.Fatal("bootstrap failed", zap.Error(err))
	}

	// -----------------------------------------------------------------------
	// Connect to Redis
	// -----------------------------------------------------------------------
	rdb, err := connectRedis(cfg, logger)
	if err != nil {
		logger.Fatal("connect to redis", zap.Error(err))
	}
	logger.Info("connected to redis")

	// -----------------------------------------------------------------------
	// Build shared store
	// -----------------------------------------------------------------------
	s := store.NewStore(db, rdb, logger)

	// -----------------------------------------------------------------------
	// SSE event bus
	// -----------------------------------------------------------------------
	bus := handlers.NewEventBus()
	defer bus.Stop()

	// -----------------------------------------------------------------------
	// Build HTTP server
	// -----------------------------------------------------------------------
	if cfg.LogLevel != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := api.NewRouter(cfg, s, bus, logger)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// -----------------------------------------------------------------------
	// Background workers
	// -----------------------------------------------------------------------
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Collector manager — starts/stops K8s collectors per cluster.
	colMgr := collector.NewCollectorManager(s, logger)
	go colMgr.RunCron(ctx)

	// Image watcher.
	imgWatcher := watcher.NewImageWatcher(s, logger)
	workerInterval := time.Duration(cfg.WorkerInterval) * time.Second
	go imgWatcher.Run(ctx, workerInterval)

	// Helm watcher.
	helmWatcher := watcher.NewHelmWatcher(s, logger)
	go helmWatcher.Run(ctx, workerInterval)

	// Scoring engine — rescores all open findings periodically.
	scoreEngine := scoring.NewScoringEngine(s, logger)
	go func() {
		// Initial scoring pass after a brief delay to allow collectors to populate data.
		time.Sleep(30 * time.Second)
		if err := scoreEngine.ScoreAll(ctx); err != nil {
			logger.Error("initial score all", zap.Error(err))
		}

		ticker := time.NewTicker(workerInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := scoreEngine.ScoreAll(ctx); err != nil {
					logger.Error("score all", zap.Error(err))
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// -----------------------------------------------------------------------
	// Start HTTP server
	// -----------------------------------------------------------------------
	go func() {
		logger.Info("HTTP server listening", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("http server error", zap.Error(err))
		}
	}()

	// -----------------------------------------------------------------------
	// Graceful shutdown on SIGTERM / SIGINT
	// -----------------------------------------------------------------------
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit

	logger.Info("shutting down …")

	// Cancel background workers.
	cancel()
	colMgr.Stop()
	imgWatcher.Stop()
	helmWatcher.Stop()

	// Drain HTTP connections.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http server shutdown", zap.Error(err))
	}

	// Close DB pool.
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.Close()
	}

	logger.Info("shutdown complete")
}

// connectPostgres opens a GORM connection to PostgreSQL.
func connectPostgres(cfg *config.Config, logger *zap.Logger) (*gorm.DB, error) {
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DB_URL is required")
	}

	gormCfg := &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	}
	if cfg.LogLevel == "debug" {
		gormCfg.Logger = gormlogger.Default.LogMode(gormlogger.Info)
	}

	var db *gorm.DB
	var err error
	for attempt := 1; attempt <= 10; attempt++ {
		db, err = gorm.Open(postgres.Open(cfg.DatabaseURL), gormCfg)
		if err == nil {
			sqlDB, sqlErr := db.DB()
			if sqlErr == nil {
				sqlDB.SetMaxOpenConns(25)
				sqlDB.SetMaxIdleConns(5)
				sqlDB.SetConnMaxLifetime(5 * time.Minute)
				if pingErr := sqlDB.Ping(); pingErr == nil {
					return db, nil
				}
			}
		}
		logger.Warn("waiting for postgres",
			zap.Int("attempt", attempt),
			zap.Error(err),
		)
		time.Sleep(3 * time.Second)
	}
	return nil, fmt.Errorf("could not connect to postgres after 10 attempts: %w", err)
}

// connectRedis opens a Redis client and verifies connectivity.
func connectRedis(cfg *config.Config, logger *zap.Logger) (*redis.Client, error) {
	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis URL: %w", err)
	}

	rdb := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return rdb, nil
}

// buildLogger constructs a zap.Logger with the given level.
func buildLogger(level string) (*zap.Logger, error) {
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		zapLevel = zapcore.InfoLevel
	}

	cfg := zap.Config{
		Level:            zap.NewAtomicLevelAt(zapLevel),
		Development:      zapLevel == zapcore.DebugLevel,
		Encoding:         "json",
		EncoderConfig:    zap.NewProductionEncoderConfig(),
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}
	cfg.EncoderConfig.TimeKey = "ts"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	return cfg.Build()
}
