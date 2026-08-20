package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/atlur/devplatform/internal/platform"
	"github.com/atlur/devplatform/internal/telemetry"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	version   = "dev"
	buildTime = "unknown"
	gitCommit = "unknown"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "control-plane",
		Short: "DevPlatform Control Plane API",
		Run:   runServer,
	}

	rootCmd.Flags().String("config", "", "Config file path")
	rootCmd.Flags().String("addr", ":8080", "HTTP listen address")
	rootCmd.Flags().String("log-level", "info", "Log level (debug, info, warn, error)")

	viper.BindPFlag("config", rootCmd.Flags().Lookup("config"))
	viper.BindPFlag("addr", rootCmd.Flags().Lookup("addr"))
	viper.BindPFlag("log-level", rootCmd.Flags().Lookup("log-level"))

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("/etc/devplatform")

	viper.SetEnvPrefix("DEVPLATFORM")
	viper.AutomaticEnv()

	if err := rootCmd.Execute(); err != nil {
		log.Fatal().Err(err).Msg("Failed to execute command")
	}
}

func runServer(cmd *cobra.Command, args []string) {
	setupLogging(viper.GetString("log-level"))

	log.Info().
		Str("version", version).
		Str("build_time", buildTime).
		Str("git_commit", gitCommit).
		Msg("Starting DevPlatform Control Plane")

	// Initialize telemetry
	shutdown, err := telemetry.Init(telemetry.Config{
		ServiceName:    "devplatform-control-plane",
		ServiceVersion: version,
		Environment:    viper.GetString("environment"),
		OTLPEndpoint:   viper.GetString("otel.endpoint"),
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize telemetry")
	}
	defer shutdown(context.Background())

	// Load configuration
	cfg := platform.LoadConfig()

	// Initialize platform
	plt, err := platform.New(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize platform")
	}
	defer plt.Close()

	// Setup HTTP server
	router := setupRouter(plt)

	addr := viper.GetString("addr")
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server
	go func() {
		log.Info().Str("addr", addr).Msg("Starting HTTP server")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("HTTP server error")
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("Server forced to shutdown")
	}

	log.Info().Msg("Server exited")
}

func setupLogging(level string) {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
}

func setupRouter(plt *platform.Platform) *gin.Engine {
	if viper.GetString("environment") == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(telemetry.GinMiddleware("control-plane"))

	// Health endpoints
	router.GET("/health", plt.HealthHandler)
	router.GET("/ready", plt.ReadyHandler)
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// API v1
	v1 := router.Group("/api/v1")
	{
		v1.Use(plt.AuthMiddleware())

		// Projects
		v1.POST("/projects", plt.CreateProject)
		v1.GET("/projects", plt.ListProjects)
		v1.GET("/projects/:id", plt.GetProject)
		v1.PUT("/projects/:id", plt.UpdateProject)
		v1.DELETE("/projects/:id", plt.DeleteProject)

		// Environments
		v1.POST("/projects/:id/environments", plt.CreateEnvironment)
		v1.GET("/projects/:id/environments", plt.ListEnvironments)
		v1.GET("/projects/:id/environments/:envId", plt.GetEnvironment)
		v1.DELETE("/projects/:id/environments/:envId", plt.DeleteEnvironment)
		v1.POST("/projects/:id/environments/:envId/deploy", plt.DeployEnvironment)

		// Deployments
		v1.GET("/projects/:id/deployments", plt.ListDeployments)
		v1.GET("/projects/:id/deployments/:deploymentId", plt.GetDeployment)
		v1.GET("/projects/:id/deployments/:deploymentId/logs", plt.GetDeploymentLogs)

		// AI endpoints
		v1.POST("/ai/review", plt.AIReviewPR)
		v1.POST("/ai/docs", plt.AIGenerateDocs)
		v1.POST("/ai/tests", plt.AIGenerateTests)
		v1.POST("/ai/migrate", plt.AIMigrateCode)

		// WebSocket for real-time updates
		v1.GET("/ws", plt.WebSocketHandler)
	}

	return router
}