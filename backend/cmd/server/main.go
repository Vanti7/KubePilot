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
	"github.com/kubepilot/backend/internal/mcp"
	"github.com/kubepilot/backend/internal/models"
	"github.com/kubepilot/backend/internal/scoring"
	"github.com/kubepilot/backend/internal/store"
	"github.com/kubepilot/backend/internal/watcher"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/gorm"
)

// version is the server build version, surfaced over MCP serverInfo.
const version = "0.2.0-alpha.1"

func main() {
	// Subcommand dispatch. With no argument the HTTP server runs (the default
	// for Docker/Helm). `kubepilot mcp` runs the MCP stdio server instead.
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		runMCP()
		return
	}
	runServer()
}

// runServer boots the full HTTP API server with background workers.
func runServer() {
	cfg := config.Load()

	logger, err := buildLogger(cfg.LogLevel, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync() //nolint:errcheck

	db, err := openDatabase(cfg, logger)
	if err != nil {
		logger.Fatal("open database", zap.Error(err))
	}
	logger.Info("database connected", zap.String("driver", cfg.StorageDriver))

	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		logger.Fatal("auto migrate", zap.Error(err))
	}
	logger.Info("database schema up to date")

	if err := bootstrap.Run(context.Background(), db, cfg, logger); err != nil {
		logger.Fatal("bootstrap failed", zap.Error(err))
	}

	cache, err := openCache(cfg, logger)
	if err != nil {
		logger.Fatal("open cache", zap.Error(err))
	}
	defer cache.Close() //nolint:errcheck
	logger.Info("cache ready", zap.String("driver", cfg.CacheDriver))

	s := store.NewStore(db, cache, logger)

	bus := handlers.NewEventBus()
	defer bus.Stop()

	colMgr := collector.NewCollectorManager(s, logger, collector.MetricsConfig{
		Enabled:   cfg.NodeMetricsEnabled,
		Retention: time.Duration(cfg.NodeMetricsRetentionHours) * time.Hour,
	})

	if cfg.LogLevel != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := api.NewRouter(cfg, s, bus, colMgr, logger)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// In demo mode the dataset is seeded and static: skip the live collectors,
	// registry/helm watchers and scoring engine so they don't overwrite or purge it.
	var imgWatcher *watcher.ImageWatcher
	var helmWatcher *watcher.HelmWatcher
	if cfg.DemoMode {
		logger.Info("demo mode: live collectors, watchers and scoring are disabled")
	} else {
		go colMgr.RunCron(ctx)

		imgWatcher = watcher.NewImageWatcher(s, logger)
		workerInterval := time.Duration(cfg.WorkerInterval) * time.Second
		go imgWatcher.Run(ctx, workerInterval)

		helmWatcher = watcher.NewHelmWatcher(s, logger)
		go helmWatcher.Run(ctx, workerInterval)

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
	}

	go func() {
		logger.Info("HTTP server listening", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("http server error", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit

	logger.Info("shutting down …")

	cancel()
	colMgr.Stop()
	if imgWatcher != nil {
		imgWatcher.Stop()
	}
	if helmWatcher != nil {
		helmWatcher.Stop()
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http server shutdown", zap.Error(err))
	}

	if sqlDB, err := db.DB(); err == nil {
		sqlDB.Close()
	}

	logger.Info("shutdown complete")
}

// runMCP boots the Model Context Protocol server over stdio. stdout is reserved
// for the JSON-RPC channel, so all logging is sent to stderr.
func runMCP() {
	cfg := config.Load()

	logger, err := buildLogger(cfg.LogLevel, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync() //nolint:errcheck

	db, err := openDatabase(cfg, logger)
	if err != nil {
		logger.Fatal("open database", zap.Error(err))
	}
	// Ensure the schema exists (no-op if the server already created it).
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		logger.Fatal("auto migrate", zap.Error(err))
	}

	cache, err := openCache(cfg, logger)
	if err != nil {
		logger.Fatal("open cache", zap.Error(err))
	}
	defer cache.Close() //nolint:errcheck

	s := store.NewStore(db, cache, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
		<-quit
		cancel()
	}()

	logger.Info("mcp stdio server starting", zap.Bool("allow_writes", cfg.MCPAllowWrites))
	srv := mcp.NewServer(s, version, cfg.MCPAllowWrites, logger)
	if err := srv.ServeStdio(ctx, os.Stdin, os.Stdout); err != nil && err != context.Canceled {
		logger.Error("mcp server exited", zap.Error(err))
	}
}

// openDatabase opens the configured storage backend.
func openDatabase(cfg *config.Config, logger *zap.Logger) (*gorm.DB, error) {
	debug := cfg.LogLevel == "debug"
	switch cfg.StorageDriver {
	case config.StorageDriverSQLite:
		return store.OpenSQLite(cfg.SQLitePath, debug, logger)
	default:
		return store.OpenPostgres(cfg.DatabaseURL, debug, logger)
	}
}

// openCache opens the configured cache backend.
func openCache(cfg *config.Config, logger *zap.Logger) (store.Cache, error) {
	switch cfg.CacheDriver {
	case config.CacheDriverMemory:
		return store.NewMemoryCache(), nil
	default:
		return store.NewRedisCache(cfg.RedisURL)
	}
}

// buildLogger constructs a zap.Logger writing to the given destination. MCP mode
// logs to stderr to keep stdout clean for the JSON-RPC channel.
func buildLogger(level string, dest *os.File) (*zap.Logger, error) {
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		zapLevel = zapcore.InfoLevel
	}

	outputPath := "stdout"
	if dest == os.Stderr {
		outputPath = "stderr"
	}

	cfg := zap.Config{
		Level:            zap.NewAtomicLevelAt(zapLevel),
		Development:      zapLevel == zapcore.DebugLevel,
		Encoding:         "json",
		EncoderConfig:    zap.NewProductionEncoderConfig(),
		OutputPaths:      []string{outputPath},
		ErrorOutputPaths: []string{"stderr"},
	}
	cfg.EncoderConfig.TimeKey = "ts"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	return cfg.Build()
}
