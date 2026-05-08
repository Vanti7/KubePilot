// Package bootstrap handles first-run setup: admin account creation and
// automatic registration of the local in-cluster Kubernetes cluster.
package bootstrap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/config"
	"github.com/kubepilot/backend/internal/models"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"k8s.io/client-go/rest"
)

// Run executes all bootstrap tasks.
// It is idempotent: tasks are skipped if already done.
func Run(ctx context.Context, db *gorm.DB, cfg *config.Config, logger *zap.Logger) error {
	if err := ensureEnvironments(ctx, db, logger); err != nil {
		return fmt.Errorf("ensure environments: %w", err)
	}
	if err := ensureAdminUser(ctx, db, cfg, logger); err != nil {
		return fmt.Errorf("ensure admin user: %w", err)
	}
	if cfg.InCluster {
		if err := ensureLocalCluster(ctx, db, cfg, logger); err != nil {
			// Non-fatal: log and continue.
			logger.Warn("could not auto-register local cluster", zap.Error(err))
		}
	}
	return nil
}

// ensureEnvironments seeds the default environments if the table is empty.
func ensureEnvironments(ctx context.Context, db *gorm.DB, logger *zap.Logger) error {
	var count int64
	db.WithContext(ctx).Model(&models.Environment{}).Count(&count)
	if count > 0 {
		return nil
	}

	defaults := []models.Environment{
		{ID: uuid.New(), Name: "Production", Slug: "prod", CriticalityWeight: 1.0, Color: "#ef4444"},
		{ID: uuid.New(), Name: "Pre-production", Slug: "preprod", CriticalityWeight: 0.7, Color: "#f97316"},
		{ID: uuid.New(), Name: "Staging", Slug: "staging", CriticalityWeight: 0.5, Color: "#eab308"},
		{ID: uuid.New(), Name: "Development", Slug: "dev", CriticalityWeight: 0.3, Color: "#3b82f6"},
	}

	if err := db.WithContext(ctx).Create(&defaults).Error; err != nil {
		return err
	}
	logger.Info("seeded default environments")
	return nil
}

// ensureAdminUser creates the first admin account if no users exist.
// Credentials come from ADMIN_EMAIL / ADMIN_PASSWORD env vars, or are
// auto-generated and printed to stdout for the operator to retrieve.
func ensureAdminUser(ctx context.Context, db *gorm.DB, cfg *config.Config, logger *zap.Logger) error {
	var count int64
	db.WithContext(ctx).Model(&models.User{}).Count(&count)
	if count > 0 {
		return nil
	}

	email := cfg.AdminEmail
	password := cfg.AdminPassword
	name := cfg.AdminName

	if email == "" {
		email = "admin@kubepilot.local"
	}
	if password == "" {
		generated, err := randomPassword(16)
		if err != nil {
			return fmt.Errorf("generate admin password: %w", err)
		}
		password = generated
		// Print to stdout so the operator can retrieve it from pod logs.
		fmt.Fprintf(os.Stdout, "\n"+
			"╔══════════════════════════════════════════════════╗\n"+
			"║         KubePilot — First-Run Setup              ║\n"+
			"╠══════════════════════════════════════════════════╣\n"+
			"║  Admin email    : %-31s║\n"+
			"║  Admin password : %-31s║\n"+
			"║  Change the password after first login.          ║\n"+
			"╚══════════════════════════════════════════════════╝\n\n",
			email, password)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}

	admin := models.User{
		ID:           uuid.New(),
		Email:        email,
		Name:         name,
		PasswordHash: string(hash),
		Role:         models.RoleAdmin,
		IsActive:     true,
	}

	if err := db.WithContext(ctx).Create(&admin).Error; err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}

	logger.Info("admin user created", zap.String("email", email))
	return nil
}

// ensureLocalCluster auto-registers the cluster where KubePilot itself is running.
// It uses the in-cluster service account token to discover the API server URL.
func ensureLocalCluster(ctx context.Context, db *gorm.DB, cfg *config.Config, logger *zap.Logger) error {
	var count int64
	db.WithContext(ctx).Model(&models.Cluster{}).
		Where("name = ?", cfg.ClusterName).
		Count(&count)
	if count > 0 {
		logger.Info("local cluster already registered", zap.String("name", cfg.ClusterName))
		return nil
	}

	// Resolve the in-cluster REST config to get the API server URL.
	restCfg, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("in-cluster config: %w", err)
	}

	// Find the "prod" environment or the first available one.
	var env models.Environment
	if err := db.WithContext(ctx).Where("slug = ?", "prod").First(&env).Error; err != nil {
		if err := db.WithContext(ctx).First(&env).Error; err != nil {
			return fmt.Errorf("find environment: %w", err)
		}
	}

	envID := env.ID
	cluster := models.Cluster{
		ID:            uuid.New(),
		EnvironmentID: &envID,
		Name:          cfg.ClusterName,
		Slug:          cfg.ClusterName,
		APIEndpoint:   restCfg.Host,
		TLSInsecure:   cfg.TLSInsecure,
		Status:        models.ClusterStatusUnknown,
		// KubeconfigRef is empty: the collector will use in-cluster config.
	}

	if err := db.WithContext(ctx).Create(&cluster).Error; err != nil {
		return fmt.Errorf("create local cluster record: %w", err)
	}

	logger.Info("local cluster auto-registered",
		zap.String("name", cfg.ClusterName),
		zap.String("endpoint", restCfg.Host),
	)
	return nil
}

func randomPassword(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
