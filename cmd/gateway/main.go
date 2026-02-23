package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/agent"
	"github.com/rocky/marstaff/internal/api"
	"github.com/rocky/marstaff/internal/config"
	"github.com/rocky/marstaff/internal/device"
	"github.com/rocky/marstaff/internal/gateway"
	"github.com/rocky/marstaff/internal/media"
	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/provider"
	"github.com/rocky/marstaff/internal/repository"
	"github.com/rocky/marstaff/internal/tools"
)

var (
	configFile string
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "gateway",
		Short: "Marstaff - AI Agent Web Server",
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
		if err := db.AutoMigrate(&model.User{}, &model.Session{}, &model.Message{}, &model.Skill{}, &model.Memory{}, &model.TodoItem{}); err != nil {
			log.Warn().Err(err).Msg("failed to auto migrate tables")
		}
	}

	// Create provider
	prov, err := provider.CreateProvider(cfg.Provider.Default, getProviderConfig(cfg, cfg.Provider.Default))
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create provider")
	}

	if err := prov.HealthCheck(context.Background()); err != nil {
		log.Warn().Err(err).Msg("provider health check failed")
	} else {
		log.Info().Str("provider", prov.Name()).Msg("provider healthy")
	}

	// Create todo repository
	var todoRepo *repository.TodoRepository
	if db != nil {
		todoRepo = repository.NewTodoRepository(db)
	}

	// Create agent engine
	engine, err := agent.NewEngine(&agent.Config{
		Provider:   prov,
		SkillsPath: cfg.Skills.Path,
		DB:         db,
		TodoRepo:   todoRepo,
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

	// Create and register file/command tools with security
	securityConfigPath := "configs/security.yaml"
	toolsExecutor, err := tools.NewExecutor(engine, securityConfigPath)
	if err != nil {
		log.Warn().Err(err).Msg("failed to create tools executor, file/command tools not available")
	} else {
		toolsExecutor.RegisterBuiltInTools()
		log.Info().Str("security_config", securityConfigPath).Msg("file and command tools registered")

		// Create and register media generation provider
		mediaConfig := getMediaProviderConfig(cfg)
		if len(mediaConfig) > 0 {
			mediaProv, err := media.CreateProvider(cfg.Media.Default, mediaConfig)
			if err != nil {
				log.Warn().Err(err).Msg("failed to create media provider, image/video tools not available")
			} else {
				toolsExecutor.SetMediaProvider(mediaProv)
				toolsExecutor.RegisterMediaTools()
				log.Info().Str("provider", mediaProv.Name()).Msg("media generation tools registered")
			}
		}
	}

	// Register todo tools
	if todoRepo != nil {
		todoExecutor := tools.NewTodoExecutor(engine, todoRepo)
		todoExecutor.RegisterBuiltInTools()
		log.Info().Msg("todo tools registered")
	}

	// Register browser tools (always available, no security needed)
	if toolsExecutor != nil {
		toolsExecutor.RegisterBrowserTools()
		log.Info().Msg("browser tools registered")
	}


	// Register cron tools
	cronExecutor := tools.NewCronExecutor(engine)
	cronExecutor.RegisterBuiltInTools()
	log.Info().Msg("cron tools registered")

	registry := engine.GetSkillRegistry()
	log.Info().Int("skills", len(registry.List())).Int("tools", len(registry.GetTools())).Msg("agent initialized")

	// Create session API
	var sessionAPI *api.SessionAPI
	if db != nil {
		sessionAPI = api.NewSessionAPI(db)
	}

	// Create hub and WebSocket server
	hub := gateway.NewHub()
	go hub.Run()
	server := gateway.NewServer(hub)

	activeSessions := make(map[string]string)

	// Create OSS uploader if configured
	var ossUploader *gateway.OSSUploader
	if cfg.OSS.AccessKeyID != "" && cfg.OSS.AccessKeySecret != "" {
		var err error
		ossUploader, err = gateway.NewOSSUploaderWithConfig(gateway.OSSConfig{
			AccessKeyID:     cfg.OSS.AccessKeyID,
			AccessKeySecret: cfg.OSS.AccessKeySecret,
			Bucket:          cfg.OSS.Bucket,
			Endpoint:        cfg.OSS.Endpoint,
			Domain:          cfg.OSS.Domain,
			PathPrefix:      cfg.OSS.PathPrefix,
		})
		if err != nil {
			log.Warn().Err(err).Msg("failed to create OSS uploader, file upload not available")
		} else {
			log.Info().Msg("OSS uploader initialized")
		}
	}

	// Set up message handler - call agent engine directly (in-process)
	server.SetMessageHandler(func(client *gateway.Client, msg *gateway.Message) error {
		log.Info().
			Str("type", string(msg.Type)).
			Str("user_id", msg.UserID).
			Str("session_id", msg.SessionID).
			Msg("received message")

		if msg.Type != gateway.MessageTypeChat {
			return nil
		}

		// Parse content and content_parts
		var content string
		var contentParts []gateway.ContentPart
		if c, ok := msg.Data.(string); ok {
			content = c
		} else if data, ok := msg.Data.(map[string]interface{}); ok {
			if c, exists := data["content"]; exists {
				if str, ok := c.(string); ok {
					content = str
				}
			}
			if parts, exists := data["content_parts"]; exists {
				if arr, ok := parts.([]interface{}); ok {
					for _, p := range arr {
						if m, ok := p.(map[string]interface{}); ok {
							var part gateway.ContentPart
							if t, ok := m["type"].(string); ok {
								part.Type = t
							}
							if t, ok := m["text"].(string); ok {
								part.Text = t
							}
							if iu, ok := m["image_url"].(map[string]interface{}); ok {
								if u, ok := iu["url"].(string); ok {
									part.ImageURL = &struct {
										URL string `json:"url"`
									}{URL: u}
								}
							}
							if part.Type != "" {
								contentParts = append(contentParts, part)
							}
						}
					}
				}
			}
		}

		if content == "" && len(contentParts) == 0 {
			return fmt.Errorf("invalid message content")
		}
		if content == "" && len(contentParts) > 0 {
			for _, p := range contentParts {
				if p.Type == "text" && p.Text != "" {
					content = p.Text
					break
				}
			}
			if content == "" {
				content = "[Screenshot]"
			}
		}

		// Create session if needed
		sessionID := client.SessionID
		if sessionID == "" && sessionAPI != nil {
			sessionID = uuid.New().String()
			client.SessionID = sessionID
			activeSessions[client.ID] = sessionID
			hub.AddClientToSession(client, sessionID)

			ctx := context.Background()
			_, err := sessionAPI.CreateSessionDirect(ctx, &api.CreateSessionRequest{
				SessionID: sessionID,
				UserID:    client.UserID,
				Platform:  "web",
				Title:     content[:min(50, len(content))],
				Model:     "default",
			})
			if err != nil {
				log.Error().Err(err).Msg("failed to create session")
			} else {
				log.Info().Str("session_id", sessionID).Msg("created new session")
			}
		}

		// Save user message
		if sessionID != "" && sessionAPI != nil {
			ctx := context.Background()
			req := &api.AddMessageRequest{Role: "user", Content: content}
			if len(contentParts) > 0 {
				req.ContentParts = make([]api.ContentPartForStorage, len(contentParts))
				for i, p := range contentParts {
					req.ContentParts[i] = api.ContentPartForStorage{
						Type: p.Type,
						Text: p.Text,
					}
					if p.ImageURL != nil {
						req.ContentParts[i].ImageURL = &api.ImageURLPart{URL: p.ImageURL.URL}
					}
				}
			}
			_ = sessionAPI.AddMessageToSession(ctx, sessionID, req)
		}

		// Send typing indicator
		hub.SendToUser(client.UserID, &gateway.Message{
			Type:      "typing",
			UserID:    client.UserID,
			SessionID: client.SessionID,
			Data:      map[string]interface{}{"typing": true},
			Timestamp: time.Now().Unix(),
		})

		// Call agent in-process (async)
		go func() {
			sendTypingDone := func() {
				hub.SendToUser(client.UserID, &gateway.Message{
					Type:      "typing",
					UserID:    client.UserID,
					SessionID: sessionID,
					Data:      map[string]interface{}{"typing": false},
					Timestamp: time.Now().Unix(),
				})
			}

			// Convert gateway content parts to provider format
			provParts := make([]provider.ContentPart, len(contentParts))
			for i, p := range contentParts {
				provParts[i] = provider.ContentPart{
					Type: p.Type,
					Text: p.Text,
				}
				if p.ImageURL != nil {
					provParts[i].ImageURL = &struct {
						URL string `json:"url"`
					}{URL: p.ImageURL.URL}
				}
			}

			chatReq := &agent.ChatRequest{
				SessionID:   sessionID,
				UserID:      client.UserID,
				Messages:    []provider.Message{{Role: provider.RoleUser, Content: content, ContentParts: provParts}},
				PlanMode:    false,
			}

			ctx := context.WithValue(context.Background(), "current_time", time.Now().Format("2006-01-02 15:04:05"))

			resp, err := executor.ExecuteWithTools(ctx, chatReq)
			if err != nil {
				log.Error().Err(err).Msg("failed to get agent response")
				sendTypingDone()
				hub.SendToUser(client.UserID, &gateway.Message{
					Type:      gateway.MessageTypeError,
					UserID:    client.UserID,
					SessionID: sessionID,
					Data:     map[string]interface{}{"error": err.Error()},
					Timestamp: time.Now().Unix(),
				})
				return
			}

			response := resp.Content
			if response == "" {
				response = "（暂无回复）"
				log.Warn().Str("session_id", sessionID).Msg("agent returned empty response")
			}

			// Check if response contains special SEARCH_OPEN marker
			const searchMarker = "SEARCH_OPEN:"
			if strings.HasPrefix(response, searchMarker) {
				// Extract the URL and the actual message
				parts := strings.SplitN(response[len(searchMarker):], "\n\n", 2)
				searchURL := strings.TrimSpace(parts[0])
				userMessage := ""
				if len(parts) > 1 {
					userMessage = strings.TrimSpace(parts[1])
				}

				log.Info().Str("url", searchURL).Msg("agent requested to open search")

				// Send open_search message first
				hub.SendToUser(client.UserID, &gateway.Message{
					Type:      gateway.MessageTypeOpenSearch,
					UserID:    client.UserID,
					SessionID: sessionID,
					Data:     map[string]interface{}{"url": searchURL},
					Timestamp: time.Now().Unix(),
				})

				// Save assistant message (without the marker)
				if sessionID != "" && sessionAPI != nil {
					ctx := context.Background()
					_ = sessionAPI.AddMessageToSession(ctx, sessionID, &api.AddMessageRequest{
						Role:    "assistant",
						Content: userMessage,
					})
				}

				sendTypingDone()

				// Send the user-friendly message
				if userMessage != "" {
					hub.SendToUser(client.UserID, &gateway.Message{
						Type:      gateway.MessageTypeChat,
						UserID:    client.UserID,
						SessionID: sessionID,
						Data:     map[string]interface{}{"content": userMessage},
						Timestamp: time.Now().Unix(),
					})
				}
			} else {
				// Normal chat response
				// Save assistant message
				if sessionID != "" && sessionAPI != nil {
					ctx := context.Background()
					_ = sessionAPI.AddMessageToSession(ctx, sessionID, &api.AddMessageRequest{
						Role:    "assistant",
						Content: response,
					})
				}

				sendTypingDone()

				hub.SendToUser(client.UserID, &gateway.Message{
					Type:      gateway.MessageTypeChat,
					UserID:    client.UserID,
					SessionID: sessionID,
					Data:     map[string]interface{}{"content": response},
					Timestamp: time.Now().Unix(),
				})
			}
		}()

		return nil
	})

	// Create router
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.Default()

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

	// Web UI
	router.Static("/static", "./web/static")
	router.GET("/", func(c *gin.Context) {
		c.File("./web/templates/chat.html")
	})
	router.GET("/ws", server.ServeWebSocket)

	// API routes
	apiGroup := router.Group("/api")
	{
		apiGroup.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"status":   "ok",
				"provider": prov.Name(),
				"clients":  server.GetClientCount(),
				"database": db != nil,
			})
		})

		// Chat API (for programmatic access)
		apiGroup.POST("/chat", func(c *gin.Context) {
			var req struct {
				SessionID   string                   `json:"session_id"`
				UserID      string                   `json:"user_id"`
				Messages    []chatMessage            `json:"messages"`
				Model       string                   `json:"model"`
				Temperature float64                   `json:"temperature"`
				Stream      bool                     `json:"stream"`
				Tools       bool                     `json:"tools"`
				PlanMode    bool                     `json:"plan_mode"`
				Thinking    *provider.ThinkingParams `json:"thinking,omitempty"`
			}

			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			provMessages := make([]provider.Message, len(req.Messages))
			hasImages := false
			for i, msg := range req.Messages {
				provMessages[i] = provider.Message{
					Role:         provider.MessageRole(msg.Role),
					Content:      msg.Content,
					ContentParts: msg.ContentParts,
				}
				for _, p := range msg.ContentParts {
					if p.Type == "image_url" && p.ImageURL != nil {
						hasImages = true
						break
					}
				}
			}

			modelName := req.Model
			if hasImages && modelName == "" {
				if provCfg := getProviderConfig(cfg, cfg.Provider.Default); provCfg != nil {
					if vm, ok := provCfg["vision_model"].(string); ok && vm != "" {
						modelName = vm
					}
				}
			}

			chatReq := &agent.ChatRequest{
				SessionID:   req.SessionID,
				UserID:      req.UserID,
				Messages:    provMessages,
				Model:       modelName,
				Temperature: req.Temperature,
				PlanMode:    req.PlanMode,
				Thinking:    req.Thinking,
			}

			ctx := context.WithValue(c.Request.Context(), "current_time", time.Now().Format("2006-01-02 15:04:05"))

			var resp *agent.ChatResponse
			var chatErr error
			if req.Tools {
				resp, chatErr = executor.ExecuteWithTools(ctx, chatReq)
			} else {
				resp, chatErr = engine.Chat(ctx, chatReq)
			}

			if chatErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": chatErr.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"content":       resp.Content,
				"thinking":      resp.Thinking,
				"tool_calls":    resp.ToolCalls,
				"usage":         resp.Usage,
				"finish_reason": resp.FinishReason,
			})
		})

		apiGroup.GET("/skills", func(c *gin.Context) {
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

		sessions := apiGroup.Group("/sessions")
		if sessionAPI != nil {
			sessions.POST("", sessionAPI.CreateSession)
			sessions.GET("/:id", sessionAPI.GetSession)
			sessions.GET("", sessionAPI.ListSessions)
			sessions.PATCH("/:id", sessionAPI.UpdateSession)
			sessions.DELETE("/:id", sessionAPI.DeleteSession)
			sessions.POST("/:id/messages", sessionAPI.AddMessage)
			sessions.GET("/:id/messages", sessionAPI.GetMessages)
		} else {
			sessions.GET("", func(c *gin.Context) {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not connected", "sessions": []interface{}{}})
			})
		}

		if sessionAPI != nil {
			memory := apiGroup.Group("/memory")
			{
				memory.POST("/:user_id", sessionAPI.SetMemory)
				memory.GET("/:user_id", sessionAPI.GetMemory)
			}
		}

		if ossUploader != nil {
			apiGroup.POST("/upload", func(c *gin.Context) {
				file, err := c.FormFile("file")
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "no file uploaded"})
					return
				}
				result, err := ossUploader.UploadFile(file)
				if err != nil {
					log.Error().Err(err).Msg("failed to upload file to OSS")
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upload file"})
					return
				}
				c.JSON(http.StatusOK, result)
			})
		}
	}

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	go func() {
		log.Info().Str("addr", addr).Msg("Marstaff server starting")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("failed to start server")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("server forced to shutdown")
	}

	if db != nil {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	}

	log.Info().Msg("server exited")
}

type chatMessage struct {
	Role         string                 `json:"role"`
	Content      string                 `json:"content,omitempty"`
	ContentParts []provider.ContentPart `json:"content_parts,omitempty"`
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
	case "zhipu":
		return cfg.Provider.Zhipu
	default:
		return nil
	}
}

func getMediaProviderConfig(cfg *config.Config) map[string]interface{} {
	switch cfg.Media.Default {
	case "qwen_wanxiang":
		return cfg.Media.QWenWanxiang
	default:
		return nil
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
