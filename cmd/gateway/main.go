package main

import (
	"context"
	"encoding/json"
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

	// Create agent client
	agentClient := gateway.NewAgentClient("http://localhost:18790")

	// Create server
	server := gateway.NewServer(hub)

	// Set up message handler to forward to agent
	server.SetMessageHandler(func(client *gateway.Client, msg *gateway.Message) error {
		log.Info().
			Str("type", string(msg.Type)).
			Str("user_id", msg.UserID).
			Str("session_id", msg.SessionID).
			Msg("received message")

		// Handle chat messages
		if msg.Type == gateway.MessageTypeChat {
			// Extract content
			var content string
			if c, ok := msg.Data.(string); ok {
				content = c
			} else if data, ok := msg.Data.(map[string]interface{}); ok {
				if c, exists := data["content"]; exists {
					if str, ok := c.(string); ok {
						content = str
					}
				}
			}

			if content == "" {
				return fmt.Errorf("invalid message content")
			}

			// Send typing indicator
			typingMsg := &gateway.Message{
				Type:      "typing",
				UserID:    client.UserID,
				SessionID: client.SessionID,
				Data: map[string]interface{}{
					"typing": true,
				},
				Timestamp: time.Now().Unix(),
			}
			typingData, _ := json.Marshal(typingMsg)
			client.Send <- typingData

			// Send to agent (async)
			go func() {
				response, err := agentClient.SendMessage(context.Background(), client.UserID, client.SessionID, content)
				if err != nil {
					log.Error().Err(err).Msg("failed to get agent response")

					// Send error back to client
					errorMsg := &gateway.Message{
						Type:      gateway.MessageTypeError,
						UserID:    client.UserID,
						SessionID: client.SessionID,
						Data: map[string]interface{}{
							"error": err.Error(),
						},
						Timestamp: time.Now().Unix(),
					}
					errorData, _ := json.Marshal(errorMsg)
					client.Send <- errorData
					return
				}

				// Send response back to client
				respMsg := &gateway.Message{
					Type:      gateway.MessageTypeChat,
					UserID:    client.UserID,
					SessionID: client.SessionID,
					Data: map[string]interface{}{
						"content": response,
					},
					Timestamp: time.Now().Unix(),
				}
				respData, _ := json.Marshal(respMsg)
				client.Send <- respData
			}()
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
