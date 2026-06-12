package config

import (
	"os"
	"strconv"
)

// Storage / cache driver identifiers.
const (
	StorageDriverPostgres = "postgres"
	StorageDriverSQLite   = "sqlite"
	CacheDriverRedis      = "redis"
	CacheDriverMemory     = "memory"
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

	// Storage backend selection.
	// StorageDriver is "postgres" or "sqlite". CacheDriver is "redis" or "memory".
	// When unset, sensible defaults are derived from LocalMode / DB_URL (see Load).
	StorageDriver string
	SQLitePath    string
	CacheDriver   string

	// LocalMode enables the single-binary, laptop-friendly mode: SQLite storage,
	// in-memory cache, and auto-registration of the local cluster from the user's
	// kubeconfig. No PostgreSQL or Redis required.
	LocalMode      bool
	KubeconfigPath string // explicit kubeconfig path for LocalMode (empty = default loading rules)

	// SSH connection (LocalMode). When SSHHost is set, the local cluster is
	// registered in "ssh" mode: KubePilot SSHes to the node, reads its kubeconfig
	// and tunnels Kubernetes API traffic through the SSH connection — useful when
	// the API server is not reachable directly from this machine.
	SSHHost           string
	SSHPort           int
	SSHUser           string
	SSHPassword       string
	SSHKubeconfigPath string // remote path to the kubeconfig (empty = try common locations)
	SSHSudo           bool   // read the kubeconfig via `sudo -S cat` (password piped)

	// MCP — Model Context Protocol server (see `kubepilot mcp`).
	// MCPAllowWrites permits write tools (e.g. changing a finding status) over MCP.
	MCPAllowWrites bool

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
	local := getBoolEnv("LOCAL_MODE", false)
	dbURL := getEnv("DB_URL", "")

	// Derive storage driver: explicit STORAGE_DRIVER wins; otherwise SQLite in
	// local mode or when no DB_URL is provided, else PostgreSQL.
	storageDriver := getEnv("STORAGE_DRIVER", "")
	if storageDriver == "" {
		if local || dbURL == "" {
			storageDriver = StorageDriverSQLite
		} else {
			storageDriver = StorageDriverPostgres
		}
	}

	// Derive cache driver: in-memory whenever we are not on PostgreSQL+Redis.
	cacheDriver := getEnv("CACHE_DRIVER", "")
	if cacheDriver == "" {
		if local || storageDriver == StorageDriverSQLite {
			cacheDriver = CacheDriverMemory
		} else {
			cacheDriver = CacheDriverRedis
		}
	}

	return &Config{
		DatabaseURL:    dbURL,
		RedisURL:       getEnv("REDIS_URL", "redis://localhost:6379"),
		JWTSecret:      getEnv("JWT_SECRET", "change-me-in-production"),
		Port:           getEnv("PORT", "8080"),
		LogLevel:       getEnv("LOG_LEVEL", "info"),
		HeadlampURL:    getEnv("HEADLAMP_URL", ""),
		TLSInsecure:    getBoolEnv("TLS_INSECURE", false),
		WorkerInterval: getIntEnv("WORKER_INTERVAL_SECONDS", 300),
		StorageDriver:  storageDriver,
		SQLitePath:     getEnv("SQLITE_PATH", "kubepilot.db"),
		CacheDriver:    cacheDriver,
		LocalMode:      local,
		KubeconfigPath: getEnv("KUBECONFIG_PATH", ""),
		SSHHost:           getEnv("SSH_HOST", ""),
		SSHPort:           getIntEnv("SSH_PORT", 22),
		SSHUser:           getEnv("SSH_USER", ""),
		SSHPassword:       getEnv("SSH_PASSWORD", ""),
		SSHKubeconfigPath: getEnv("SSH_KUBECONFIG_PATH", ""),
		SSHSudo:           getBoolEnv("SSH_SUDO", false),
		MCPAllowWrites: getBoolEnv("MCP_ALLOW_WRITES", false),
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
