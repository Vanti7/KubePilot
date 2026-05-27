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
			authProtected.POST("/users", middleware.RequireRole("admin"), authH.CreateUser)
		}
	}
	v1.Use(middleware.JWTAuth(cfg.JWTSecret))

	// Clusters.
	clusterH := handlers.NewClusterHandler(s, logger)
	clusters := v1.Group("/clusters")
	{
		clusters.GET("", clusterH.ListClusters)
		clusters.POST("", middleware.RequireRole("operator"), clusterH.CreateCluster)
		clusters.GET("/:id", clusterH.GetCluster)
		clusters.PUT("/:id", middleware.RequireRole("operator"), clusterH.UpdateCluster)
		clusters.DELETE("/:id", middleware.RequireRole("admin"), clusterH.DeleteCluster)
		clusters.POST("/:id/sync", middleware.RequireRole("operator"), clusterH.SyncCluster)
	}

	// Findings.
	findingH := handlers.NewFindingHandler(s, logger)
	findings := v1.Group("/findings")
	{
		findings.GET("", findingH.ListFindings)
		findings.GET("/summary", findingH.GetFindingSummary)
		findings.GET("/:id", findingH.GetFinding)
		findings.PATCH("/:id/status", middleware.RequireRole("operator"), findingH.UpdateFindingStatus)
	}

	// Workloads.
	workloadH := handlers.NewWorkloadHandler(s, logger)
	workloads := v1.Group("/workloads")
	{
		workloads.GET("", workloadH.ListWorkloads)
		workloads.GET("/:id", workloadH.GetWorkload)
	}

	// Helm releases.
	helmH := handlers.NewHelmHandler(s, logger)
	helmGroup := v1.Group("/helm")
	{
		helmGroup.GET("", helmH.ListHelmReleases)
		helmGroup.GET("/:id", helmH.GetHelmRelease)
	}

	// Nodes.
	nodeH := handlers.NewNodeHandler(s, logger)
	nodes := v1.Group("/nodes")
	{
		nodes.GET("", nodeH.ListNodes)
		nodes.GET("/:id", nodeH.GetNode)
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

	// Overview.
	overviewH := handlers.NewOverviewHandler(s, logger)
	v1.GET("/overview", overviewH.GetOverview)

	// SSE events stream.
	eventsH := handlers.NewEventsHandler(bus, logger)
	v1.GET("/events", eventsH.Stream)

	return router
}
