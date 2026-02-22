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
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/rocky/marstaff/internal/config"
	"github.com/rocky/marstaff/internal/gateway"
)

var (
 configFile string
)

func main() {
 var rootCmd = &cobra.Command{
  Use:   "gateway",
  Short: "Marstaff WebSocket Gateway",
  Run:   run,
 }

 rootCmd.Flags().StringVarP(&configFile, "config", "c", "configs/config.yaml", "config file path")

 if err := rootCmd.Execute(); err != nil {
  fmt.Fprintln(os.Stderr, err)
  os.Exit(1)
 }
}

func run(cmd *cobra.Command, args []string) {
	// Load config
	cfg, err := config.Load(configFile)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	// Setup logger
	setupLogger(cfg)

	// Create hub
	hub := gateway.NewHub()
	go hub.Run()

	// Create server
	server := gateway.NewServer(hub)

	// Set up message handler (for demo, echo back)
	server.SetMessageHandler(func(client *gateway.Client, msg *gateway.Message) error {
		log.Info().
			Str("type", string(msg.Type)).
			Str("user_id", msg.UserID).
			Msg("received message")

		// Echo the message back for testing
		if msg.Type == gateway.MessageTypeChat {
			response := &gateway.Message{
				Type:      gateway.MessageTypeChat,
				SessionID: msg.SessionID,
				UserID:    msg.UserID,
				Data: map[string]interface{}{
					"content": fmt.Sprintf("Echo: %v", msg.Data),
				},
				Timestamp: time.Now().Unix(),
			}
			hub.Broadcast(response)
		}
		return nil
	})

	// Create router
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.Default()

	// CORS middleware
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	})

	// Serve static files
	router.Static("/static", "./web/static")
	router.LoadHTMLGlob("web/templates/*")

	// Index page
	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "chat.html", nil)
	})

	// WebSocket endpoint
	router.GET("/ws", server.ServeWebSocket)

	// API routes
	api := router.Group("/api")
	{
		api.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"status":  "ok",
				"clients": server.GetClientCount(),
			})
		})
	}

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	// Start server in background
	go func() {
		log.Info().Str("addr", addr).Msg("gateway server starting")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("failed to start server")
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("server forced to shutdown")
	}

	log.Info().Msg("server exited")
}

func setupLogger(cfg *config.Config) {
 zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
 zerolog.SetGlobalLevel(zerolog.InfoLevel)

 if cfg.Log.Level == "debug" {
  zerolog.SetGlobalLevel(zerolog.DebugLevel)
 } else if cfg.Log.Level == "warn" {
  zerolog.SetGlobalLevel(zerolog.WarnLevel)
 }

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
}
