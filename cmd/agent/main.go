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
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/agent"
	"github.com/rocky/marstaff/internal/api"
	"github.com/rocky/marstaff/internal/config"
	"github.com/rocky/marstaff/internal/device"
	"github.com/rocky/marstaff/internal/model"
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

	// Connect to database
	var db *gorm.DB
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.Database.Username,
		cfg.Database.Password,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.Database,
	)

	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Warn().Err(err).Msg("failed to connect to database, running without persistence")
		db = nil
	} else {
		log.Info().Msg("connected to database")
		// Auto migrate tables
		if err := db.AutoMigrate(&model.User{}, &model.Session{}, &model.Message{}, &model.Skill{}, &model.Memory{}); err != nil {
			log.Warn().Err(err).Msg("failed to auto migrate tables")
		}
	}

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
		DB:         db,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create engine")
	}

	// Create and register tool executor
	executor := agent.NewExecutor(engine)
	executor.RegisterBuiltInTools()

	// Create and register device control tools
	deviceToolExecutor := device.NewToolExecutor(engine)
	deviceToolExecutor.RegisterBuiltInTools()

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

	// Log available tools
	tools := registry.GetTools()
	log.Info().Int("count", len(tools)).Msg("available tools")
	for _, t := range tools {
		log.Info().
			Str("name", t.Name).
			Str("description", t.Description).
			Msg("tool available")
	}

	// Create session API if database is connected
	var sessionAPI *api.SessionAPI
	if db != nil {
		sessionAPI = api.NewSessionAPI(db)
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
			Tools       bool     `json:"tools"`
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

		// Add current time to context
		ctx := context.WithValue(c.Request.Context(), "current_time", time.Now().Format("2006-01-02 15:04:05"))

		var resp *agent.ChatResponse
		var err error

		// Process chat with or without tools
		if req.Tools {
			resp, err = executor.ExecuteWithTools(ctx, chatReq)
		} else {
			resp, err = engine.Chat(ctx, chatReq)
		}

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Process streaming
		if req.Stream {
			// For now, return non-streaming with tools
			// TODO: Implement streaming with tool calls
		}

		c.JSON(http.StatusOK, gin.H{
			"content":       resp.Content,
			"tool_calls":    resp.ToolCalls,
			"usage":         resp.Usage,
			"finish_reason": resp.FinishReason,
		})
	})

	// Health check
	router.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":   "ok",
			"provider": prov.Name(),
			"database": db != nil,
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

	// Session management endpoints (if database is connected)
	if sessionAPI != nil {
		sessions := router.Group("/api/sessions")
		{
			sessions.POST("", sessionAPI.CreateSession)
			sessions.GET("/:id", sessionAPI.GetSession)
			sessions.GET("", sessionAPI.ListSessions)
			sessions.DELETE("/:id", sessionAPI.DeleteSession)
			sessions.POST("/:id/messages", sessionAPI.AddMessage)
			sessions.GET("/:id/messages", sessionAPI.GetMessages)
		}

		// Memory endpoints
		memory := router.Group("/api/memory")
		{
			memory.POST("/:user_id", sessionAPI.SetMemory)
			memory.GET("/:user_id", sessionAPI.GetMemory)
		}
	}

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

	// Close database connection
	if db != nil {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
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
