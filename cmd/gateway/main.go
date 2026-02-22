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

 // Create router
 router := gin.Default()

 // WebSocket endpoint
 router.GET("/ws", func(c *gin.Context) {
  serveWebSocket(hub, c)
 })

 // Health check
 router.GET("/health", func(c *gin.Context) {
   c.JSON(http.StatusOK, gin.H{
    "status": "ok",
    "clients": hub.GetClientCount(),
   })
 })

 // Create server
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

func serveWebSocket(hub *gateway.Hub, c *gin.Context) {
 // TODO: Implement WebSocket upgrade and client registration
 c.JSON(http.StatusOK, gin.H{"status": "websocket endpoint"})
}
