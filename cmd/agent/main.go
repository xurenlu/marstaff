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

	"github.com/rocky/marstaff/internal/agent"
	"github.com/rocky/marstaff/internal/config"
	"github.com/rocky/marstaff/internal/provider"
)

var (
	configFile string
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "agent",
		Short: "Marstaff AI Agent",
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

	// Create provider
	prov, err := provider.CreateProvider(cfg.Provider.Default, getProviderConfig(cfg, cfg.Provider.Default))
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create provider")
	}

	// Health check
	if err := prov.HealthCheck(context.Background()); err != nil {
		log.Warn().Err(err).Msg("provider health check failed")
	} else {
		log.Info().Str("provider", prov.Name()).Msg("provider healthy")
	}

	// Log supported models
	for _, model := range prov.SupportedModels() {
		log.Debug().Str("model", model).Msg("supported model")
	}

	// Create agent engine
	engine, err := agent.NewEngine(&agent.Config{
		Provider:   prov,
		SkillsPath: cfg.Skills.Path,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create engine")
	}

	// Log loaded skills
	registry := engine.GetSkillRegistry()
	skills := registry.List()
	log.Info().Int("count", len(skills)).Msg("loaded skills")
	for _, s := range skills {
		meta := s.Metadata()
		log.Info().
			Str("id", meta.ID).
			Str("name", meta.Name).
			Str("category", meta.Category).
			Msg("skill loaded")
	}

	// Create HTTP server for API
	router := gin.Default()

	// Chat endpoint
	router.POST("/api/chat", func(c *gin.Context) {
		var req struct {
			SessionID   string   `json:"session_id"`
			UserID      string   `json:"user_id"`
			Messages    []Message `json:"messages"`
			Model       string   `json:"model"`
			Temperature float64  `json:"temperature"`
			Stream      bool     `json:"stream"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Convert messages
		provMessages := make([]provider.Message, len(req.Messages))
		for i, msg := range req.Messages {
			provMessages[i] = provider.Message{
				Role:    provider.MessageRole(msg.Role),
				Content: msg.Content,
			}
		}

		// Create chat request
		chatReq := &agent.ChatRequest{
			SessionID:   req.SessionID,
			UserID:      req.UserID,
			Messages:    provMessages,
			Model:       req.Model,
			Temperature: req.Temperature,
		}

		// Process chat
		if req.Stream {
			// Streaming response
			ch, err := engine.ChatStream(c.Request.Context(), chatReq)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			// Set SSE headers
			c.Header("Content-Type", "text/event-stream")
			c.Header("Cache-Control", "no-cache")
			c.Header("Connection", "keep-alive")

			// Stream chunks
			for chunk := range ch {
				fmt.Fprintf(c.Writer, "data: %s\n\n", chunk)
				c.Writer.Flush()
			}
			fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
			c.Writer.Flush()
		} else {
			// Non-streaming response
			resp, err := engine.Chat(c.Request.Context(), chatReq)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"content":       resp.Content,
				"tool_calls":    resp.ToolCalls,
				"usage":         resp.Usage,
				"finish_reason": resp.FinishReason,
			})
		}
	})

	// Health check
	router.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":   "ok",
			"provider": prov.Name(),
		})
	})

	// Skills list
	router.GET("/api/skills", func(c *gin.Context) {
		skills := registry.ListEnabled()
		result := make([]gin.H, len(skills))
		for i, s := range skills {
			meta := s.Metadata()
			result[i] = gin.H{
				"id":          meta.ID,
				"name":        meta.Name,
				"description": meta.Description,
				"category":    meta.Category,
				"version":     meta.Version,
			}
		}
		c.JSON(http.StatusOK, gin.H{"skills": result})
	})

	// Start server
	addr := "0.0.0.0:18790"
	srv := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	go func() {
		log.Info().Str("addr", addr).Msg("agent API server starting")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("failed to start server")
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("shutting down agent...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("server forced to shutdown")
	}

	log.Info().Msg("agent exited")
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

func getProviderConfig(cfg *config.Config, name string) map[string]interface{} {
	switch name {
	case "zai":
		return cfg.Provider.ZAI
	case "qwen":
		return cfg.Provider.Qwen
	case "openai":
		return cfg.Provider.OpenAI
	default:
		return nil
	}
}

// Message represents a chat message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
