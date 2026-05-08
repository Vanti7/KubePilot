package config

import (
	"os"
	"strconv"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	DatabaseURL     string
	RedisURL        string
	JWTSecret       string
	Port            string
	LogLevel        string
	WorkerInterval  int
	HeadlampURL     string
	TLSInsecure     bool

	// Bootstrap — first-run admin account creation.
	// If set and no users exist yet, the backend will auto-create an admin account on startup.
	AdminEmail    string
	AdminPassword string
	AdminName     string

	// In-cluster registration.
	// When InCluster=true the backend auto-registers its own cluster using the pod's SA token.
	InCluster   bool
	ClusterName string // human name for the auto-registered local cluster (default: "local")
}

// Load reads configuration from environment variables, applying defaults where appropriate.
func Load() *Config {
	return &Config{
		DatabaseURL:    getEnv("DB_URL", ""),
		RedisURL:       getEnv("REDIS_URL", "redis://localhost:6379"),
		JWTSecret:      getEnv("JWT_SECRET", "change-me-in-production"),
		Port:           getEnv("PORT", "8080"),
		LogLevel:       getEnv("LOG_LEVEL", "info"),
		HeadlampURL:    getEnv("HEADLAMP_URL", ""),
		TLSInsecure:    getBoolEnv("TLS_INSECURE", false),
		WorkerInterval: getIntEnv("WORKER_INTERVAL_SECONDS", 300),
		AdminEmail:     getEnv("ADMIN_EMAIL", ""),
		AdminPassword:  getEnv("ADMIN_PASSWORD", ""),
		AdminName:      getEnv("ADMIN_NAME", "Administrator"),
		InCluster:      getBoolEnv("IN_CLUSTER", false),
		ClusterName:    getEnv("CLUSTER_NAME", "local"),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getIntEnv(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getBoolEnv(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}
