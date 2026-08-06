package config

import (
	"crypto/rand"
	"encoding/hex"
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
	DatabaseURL string
	RedisURL    string
	JWTSecret   string
	// JWTSecretGenerated is true when JWT_SECRET was not set and a random
	// secret was generated for this process only (see Load) — the caller
	// should warn loudly once a logger is available.
	JWTSecretGenerated bool
	Port               string
	LogLevel           string
	WorkerInterval     int
	HeadlampURL        string
	TLSInsecure        bool

	// Node metrics — collected from the kubelet Summary API on each collection pass.
	// NodeMetricsEnabled toggles collection; NodeMetricsRetentionHours bounds how
	// long time-series samples are kept before being purged.
	NodeMetricsEnabled        bool
	NodeMetricsRetentionHours int

	// HelmAutodiscover lets the Helm watcher fall back to Artifact Hub to find
	// which repository an installed chart came from, when none of the configured
	// repositories carries it. Disable it to keep chart resolution fully offline.
	HelmAutodiscover bool

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

	// DemoMode seeds a realistic multi-cluster dataset (clusters, nodes, metrics,
	// workloads, helm releases, findings) and disables the live collectors/watchers
	// so the seeded data is not overwritten. For demos and screenshots.
	DemoMode bool
}

// Load reads configuration from environment variables, applying defaults where appropriate.
func Load() *Config {
	local := getBoolEnv("LOCAL_MODE", false)
	demo := getBoolEnv("DEMO_MODE", false)
	dbURL := getEnv("DB_URL", "")

	// Derive storage driver: explicit STORAGE_DRIVER wins; otherwise SQLite in
	// local/demo mode or when no DB_URL is provided, else PostgreSQL.
	storageDriver := getEnv("STORAGE_DRIVER", "")
	if storageDriver == "" {
		if local || demo || dbURL == "" {
			storageDriver = StorageDriverSQLite
		} else {
			storageDriver = StorageDriverPostgres
		}
	}

	// In demo mode, default to a known admin password so the operator can log in
	// without scraping generated credentials from the logs.
	adminPassword := getEnv("ADMIN_PASSWORD", "")
	if demo && adminPassword == "" {
		adminPassword = "demo"
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

	// A fixed default secret would let anyone forge an admin token against any
	// KubePilot instance that didn't override it — generate a random one
	// instead when JWT_SECRET is unset. It is process-local, not persisted:
	// existing sessions become invalid on every restart until the operator
	// sets JWT_SECRET explicitly (expected for production / persistent local
	// use, see the startup warning in cmd/server/main.go).
	jwtSecret := getEnv("JWT_SECRET", "")
	jwtSecretGenerated := jwtSecret == ""
	if jwtSecretGenerated {
		jwtSecret = generateRandomSecret()
	}

	return &Config{
		DatabaseURL:               dbURL,
		RedisURL:                  getEnv("REDIS_URL", "redis://localhost:6379"),
		JWTSecret:                 jwtSecret,
		JWTSecretGenerated:        jwtSecretGenerated,
		Port:                      getEnv("PORT", "8080"),
		LogLevel:                  getEnv("LOG_LEVEL", "info"),
		HeadlampURL:               getEnv("HEADLAMP_URL", ""),
		TLSInsecure:               getBoolEnv("TLS_INSECURE", false),
		WorkerInterval:            getIntEnv("WORKER_INTERVAL_SECONDS", 300),
		HelmAutodiscover:          getBoolEnv("HELM_AUTODISCOVER", true),
		NodeMetricsEnabled:        getBoolEnv("NODE_METRICS_ENABLED", true),
		NodeMetricsRetentionHours: getIntEnv("NODE_METRICS_RETENTION_HOURS", 168),
		StorageDriver:             storageDriver,
		SQLitePath:                getEnv("SQLITE_PATH", "kubepilot.db"),
		CacheDriver:               cacheDriver,
		LocalMode:                 local,
		KubeconfigPath:            getEnv("KUBECONFIG_PATH", ""),
		SSHHost:                   getEnv("SSH_HOST", ""),
		SSHPort:                   getIntEnv("SSH_PORT", 22),
		SSHUser:                   getEnv("SSH_USER", ""),
		SSHPassword:               getEnv("SSH_PASSWORD", ""),
		SSHKubeconfigPath:         getEnv("SSH_KUBECONFIG_PATH", ""),
		SSHSudo:                   getBoolEnv("SSH_SUDO", false),
		MCPAllowWrites:            getBoolEnv("MCP_ALLOW_WRITES", false),
		AdminEmail:                getEnv("ADMIN_EMAIL", ""),
		AdminPassword:             adminPassword,
		AdminName:                 getEnv("ADMIN_NAME", "Administrator"),
		InCluster:                 getBoolEnv("IN_CLUSTER", false),
		ClusterName:               getEnv("CLUSTER_NAME", "local"),
		DemoMode:                  demo,
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

// generateRandomSecret returns a random 32-byte hex-encoded string for
// signing JWTs when JWT_SECRET is not configured.
func generateRandomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failing means the entropy source is broken — there is no
		// safe fallback, so fail loudly instead of signing tokens with a weak
		// secret.
		panic("config: failed to generate random JWT secret: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func getBoolEnv(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}
