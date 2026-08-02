package api

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/kubepilot/backend/internal/api/handlers"
	"github.com/kubepilot/backend/internal/api/middleware"
	"github.com/kubepilot/backend/internal/config"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
)

// NewRouter builds and returns the configured Gin engine.
func NewRouter(
	cfg *config.Config,
	s *store.Store,
	bus *handlers.EventBus,
	syncer handlers.ClusterOps,
	imageChecker handlers.ImageChecker,
	helmChecker handlers.HelmChecker,
	version string,
	logger *zap.Logger,
) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())

	// Structured request logging.
	router.Use(func(c *gin.Context) {
		c.Next()
		logger.Info("request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.String("client_ip", c.ClientIP()),
		)
	})

	// CORS.
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowAllOrigins = true
	corsConfig.AllowHeaders = append(corsConfig.AllowHeaders, "Authorization")
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	router.Use(cors.New(corsConfig))

	// Health endpoints — no auth required.
	healthH := handlers.NewHealthHandler(s)
	router.GET("/health", healthH.Liveness)
	router.GET("/health/ready", healthH.Readiness)
	router.GET("/health/connectors", healthH.Connectors)

	// All /api/v1 routes — auth endpoints are nested here so the frontend
	// base URL (/api/v1) resolves correctly.
	v1 := router.Group("/api/v1")

	// Auth endpoints — login and setup do not require JWT, the others do.
	authH := handlers.NewAuthHandler(s, cfg.JWTSecret, logger)
	authGroup := v1.Group("/auth")
	{
		authGroup.POST("/login", authH.Login)
		authGroup.GET("/setup", authH.SetupStatus)
		authGroup.POST("/setup", authH.Setup)

		authProtected := authGroup.Group("")
		authProtected.Use(middleware.JWTAuth(cfg.JWTSecret))
		{
			authProtected.POST("/refresh", authH.RefreshToken)
			authProtected.GET("/me", authH.Me)
			authProtected.GET("/users", middleware.RequireRole("admin"), authH.ListUsers)
			authProtected.POST("/users", middleware.RequireRole("admin"), authH.CreateUser)
			authProtected.PATCH("/users/:id", middleware.RequireRole("admin"), authH.UpdateUser)
			authProtected.DELETE("/users/:id", middleware.RequireRole("admin"), authH.DeleteUser)
		}
	}
	v1.Use(middleware.JWTAuth(cfg.JWTSecret))

	// Clusters.
	clusterH := handlers.NewClusterHandler(s, syncer, logger)
	clusters := v1.Group("/clusters")
	{
		clusters.GET("", clusterH.ListClusters)
		clusters.POST("", middleware.RequireRole("operator"), clusterH.CreateCluster)
		clusters.GET("/:id", clusterH.GetCluster)
		clusters.GET("/:id/resources", clusterH.GetClusterResources)
		clusters.PUT("/:id", middleware.RequireRole("operator"), clusterH.UpdateCluster)
		clusters.DELETE("/:id", middleware.RequireRole("admin"), clusterH.DeleteCluster)
		clusters.POST("/:id/sync", middleware.RequireRole("operator"), clusterH.SyncCluster)

		manifestH := handlers.NewManifestHandler(s, syncer, logger)
		clusters.POST("/:id/manifests/apply", middleware.RequireRole("operator"), manifestH.ApplyManifest)
	}

	// Findings.
	findingH := handlers.NewFindingHandler(s, syncer, imageChecker, helmChecker, logger)
	findings := v1.Group("/findings")
	{
		findings.GET("", findingH.ListFindings)
		findings.GET("/summary", findingH.GetFindingSummary)
		findings.GET("/export", findingH.ExportFindings)
		findings.GET("/:id", findingH.GetFinding)
		findings.PATCH("/:id/status", middleware.RequireRole("operator"), findingH.UpdateFindingStatus)
		findings.POST("/:id/remediate", middleware.RequireRole("operator"), findingH.RemediateFinding)
	}

	// Workloads.
	workloadH := handlers.NewWorkloadHandler(s, syncer, logger)
	workloads := v1.Group("/workloads")
	{
		workloads.GET("", workloadH.ListWorkloads)
		workloads.GET("/:id", workloadH.GetWorkload)
		workloads.PATCH("/:id/scale", middleware.RequireRole("operator"), workloadH.ScaleWorkload)
		workloads.POST("/:id/restart", middleware.RequireRole("operator"), workloadH.RestartWorkload)
	}

	// Helm releases.
	helmH := handlers.NewHelmHandler(s, syncer, logger)
	helmGroup := v1.Group("/helm")
	{
		helmGroup.GET("", helmH.ListHelmReleases)
		helmGroup.GET("/:id", helmH.GetHelmRelease)
		helmGroup.POST("/:id/upgrade", middleware.RequireRole("operator"), helmH.UpgradeHelmRelease)
		helmGroup.POST("/:id/rollback", middleware.RequireRole("operator"), helmH.RollbackHelmRelease)
	}

	// Nodes.
	nodeH := handlers.NewNodeHandler(s, logger)
	nodes := v1.Group("/nodes")
	{
		nodes.GET("", nodeH.ListNodes)
		nodes.GET("/:id", nodeH.GetNode)
		nodes.GET("/:id/metrics", nodeH.GetNodeMetrics)
	}

	// Namespaces.
	nsH := handlers.NewNamespaceHandler(s, logger)
	v1.GET("/namespaces", nsH.ListNamespaces)

	// Secrets.
	secretH := handlers.NewSecretHandler(s, logger)
	v1.GET("/secrets", secretH.ListSecrets)

	// Integrations.
	integrationH := handlers.NewIntegrationHandler(s, logger)
	integrations := v1.Group("/integrations")
	{
		integrations.GET("", integrationH.ListIntegrations)
		integrations.POST("", middleware.RequireRole("operator"), integrationH.CreateIntegration)
		integrations.POST("/:id/test", middleware.RequireRole("operator"), integrationH.TestIntegration)
		integrations.DELETE("/:id", middleware.RequireRole("admin"), integrationH.DeleteIntegration)
	}

	// Image registries.
	registryH := handlers.NewRegistryHandler(s, logger)
	registries := v1.Group("/registries")
	{
		registries.GET("", registryH.ListRegistries)
		registries.POST("", middleware.RequireRole("operator"), registryH.CreateRegistry)
		registries.PUT("/:id", middleware.RequireRole("operator"), registryH.UpdateRegistry)
		registries.DELETE("/:id", middleware.RequireRole("admin"), registryH.DeleteRegistry)
		registries.POST("/:id/test", middleware.RequireRole("operator"), registryH.TestRegistry)
	}

	// Helm chart repositories — used to resolve where an installed chart came from.
	helmRepoH := handlers.NewHelmRepositoryHandler(s, logger)
	helmRepos := v1.Group("/helm-repositories")
	{
		helmRepos.GET("", helmRepoH.ListHelmRepositories)
		helmRepos.POST("", middleware.RequireRole("operator"), helmRepoH.CreateHelmRepository)
		helmRepos.PUT("/:id", middleware.RequireRole("operator"), helmRepoH.UpdateHelmRepository)
		helmRepos.DELETE("/:id", middleware.RequireRole("admin"), helmRepoH.DeleteHelmRepository)
		helmRepos.POST("/:id/test", middleware.RequireRole("operator"), helmRepoH.TestHelmRepository)
	}

	// Settings — non-secret runtime config (admin only).
	settingsH := handlers.NewSettingsHandler(cfg, version)
	v1.GET("/settings", middleware.RequireRole("admin"), settingsH.GetSettings)

	// Action logs — audit trail (read-only endpoint; the write side is the
	// handlers.RecordAction helper other handlers call directly in-process,
	// not an HTTP endpoint).
	actionLogH := handlers.NewActionLogHandler(s, logger)
	v1.GET("/action-logs", actionLogH.ListActionLogs)

	// Overview.
	overviewH := handlers.NewOverviewHandler(s, logger)
	v1.GET("/overview", overviewH.GetOverview)

	// SSE events stream.
	eventsH := handlers.NewEventsHandler(bus, logger)
	v1.GET("/events", eventsH.Stream)

	return router
}
