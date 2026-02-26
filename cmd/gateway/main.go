package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

	"github.com/rocky/marstaff/internal/adapter"
	"github.com/rocky/marstaff/internal/agent"
	"github.com/rocky/marstaff/internal/api"
	afkpkg "github.com/rocky/marstaff/internal/afk"
	"github.com/rocky/marstaff/internal/config"
	"github.com/rocky/marstaff/internal/device"
	"github.com/rocky/marstaff/internal/gateway"
	"github.com/rocky/marstaff/internal/media"
	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/pipeline"
	"github.com/rocky/marstaff/internal/provider"
	"github.com/rocky/marstaff/internal/repository"
	"github.com/rocky/marstaff/internal/skill"
	"github.com/rocky/marstaff/internal/tools"
)

var (
	configFile    string
	enableTelegram bool
	Version       = "1.11.0-rc1"
)

// truncateRunes truncates s to at most max runes (avoids cutting mid-UTF8-char)
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

func main() {
	var rootCmd = &cobra.Command{
		Use:   "gateway",
		Short: "Marstaff - AI Agent Web Server",
		Run:   run,
	}

	rootCmd.Flags().StringVarP(&configFile, "config", "c", "configs/config.yaml", "config file path")
	rootCmd.Flags().BoolVarP(&enableTelegram, "enable-telegram", "t", false, "enable Telegram bot (disabled by default in China)")

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
		if err := db.AutoMigrate(&model.User{}, &model.Session{}, &model.Message{}, &model.Skill{}, &model.Memory{}, &model.TodoItem{}, &model.Project{}, &model.Rule{}, &model.MCPServer{}, &model.MCPTool{}, &model.AFKTask{}, &model.AFKTaskExecution{}, &model.Pipeline{}); err != nil {
			log.Warn().Err(err).Msg("failed to auto migrate tables")
		}
	}

	// Create provider
	prov, err := provider.CreateProvider(cfg.Provider.Default, getProviderConfig(cfg, cfg.Provider.Default))
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create provider")
	}

	// Build vision providers map (qwen, zai with vision_model) for image recognition
	visionProviders := make(map[string]provider.Provider)
	for _, name := range []string{"qwen", "zai"} {
		if provCfg := getProviderConfig(cfg, name); provCfg != nil {
			if vm, ok := provCfg["vision_model"].(string); ok && vm != "" {
				vp, err := provider.CreateProvider(name, provCfg)
				if err != nil {
					log.Warn().Err(err).Str("provider", name).Msg("failed to create vision provider")
				} else {
					visionProviders[name] = vp
					log.Info().Str("provider", name).Str("model", vm).Msg("vision provider registered")
				}
			}
		}
	}

	// Build chat providers map for user-selectable chat engines
	chatProviders := make(map[string]provider.Provider)
	for _, name := range []string{"zai", "qwen", "gemini", "deepseek", "minimax", "minimax_intl", "ollama", "vllm"} {
		if provCfg := getProviderConfig(cfg, name); provCfg != nil {
			cp, err := provider.CreateProvider(name, provCfg)
			if err != nil {
				log.Warn().Err(err).Str("provider", name).Msg("failed to create chat provider")
			} else {
				chatProviders[name] = cp
				log.Info().Str("provider", name).Msg("chat provider registered")
			}
		}
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

	// Create rule repository
	var ruleRepo *repository.RuleRepository
	if db != nil {
		ruleRepo = repository.NewRuleRepository(db)
	}

	// Create token usage repository
	var tokenUsageRepo *repository.TokenUsageRepository
	if db != nil {
		tokenUsageRepo = repository.NewTokenUsageRepository(db)
	}

	// Create session repository (needed for AFK async tasks)
	var sessionRepo *repository.SessionRepository
	if db != nil {
		sessionRepo = repository.NewSessionRepository(db)
	}

	// Create AFK task repository (needed for async video tasks)
	var afkTaskRepo *repository.AFKTaskRepository
	if db != nil {
		afkTaskRepo = repository.NewAFKTaskRepository(db)
	}

	// Create agent engine
	engine, err := agent.NewEngine(&agent.Config{
		Provider:      prov,
		SkillsPath:    cfg.Skills.Path,
		DB:            db,
		TodoRepo:      todoRepo,
		RuleRepo:      ruleRepo,
		TokenUsageRepo: tokenUsageRepo,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create engine")
	}

	// Create and register tool executor
	executor := agent.NewExecutor(engine)
	if cfg.Security.Sandbox.Mode == "non_main" {
		executor.SetSandboxMode(cfg.Security.Sandbox.Mode)
	}
	executor.RegisterBuiltInTools()

	// Create OSS uploader early for device screenshot tools (screenshots must upload to OSS, not base64)
	var ossUploaderForDevice *gateway.OSSUploader
	if cfg.OSS.AccessKeyID != "" && cfg.OSS.AccessKeySecret != "" {
		ossUploaderForDevice, err = gateway.NewOSSUploaderWithConfig(gateway.OSSConfig{
			AccessKeyID:     cfg.OSS.AccessKeyID,
			AccessKeySecret: cfg.OSS.AccessKeySecret,
			Bucket:          cfg.OSS.Bucket,
			Endpoint:        cfg.OSS.Endpoint,
			Domain:          cfg.OSS.Domain,
			PathPrefix:      cfg.OSS.PathPrefix,
		})
		if err != nil {
			log.Warn().Err(err).Msg("failed to create OSS uploader for device screenshots")
		} else {
			log.Info().Msg("OSS uploader ready for device screenshots")
		}
	}

	// Create and register device control tools
	deviceToolExecutor := device.NewToolExecutor(engine)
	if ossUploaderForDevice != nil {
		deviceToolExecutor.SetImageUploader(newOSSImageUploader(ossUploaderForDevice))
	}
	// Wire vision provider for screen analysis (device_screen_analyze)
	if vp, ok := visionProviders["qwen"]; ok {
		if provCfg := getProviderConfig(cfg, "qwen"); provCfg != nil {
			if vm, ok := provCfg["vision_model"].(string); ok && vm != "" {
				deviceToolExecutor.SetVisionProvider(vp, vm)
				log.Info().Str("model", vm).Msg("vision provider wired for screen analysis")
			}
		}
	} else if vp, ok := visionProviders["zai"]; ok {
		if provCfg := getProviderConfig(cfg, "zai"); provCfg != nil {
			if vm, ok := provCfg["vision_model"].(string); ok && vm != "" {
				deviceToolExecutor.SetVisionProvider(vp, vm)
				log.Info().Str("model", vm).Msg("vision provider wired for screen analysis")
			}
		}
	}
	deviceToolExecutor.RegisterBuiltInTools()

	// Create and register file/command tools with security
	securityConfigPath := "configs/security.yaml"
	toolsExecutor, err := tools.NewExecutor(engine, securityConfigPath)
	if err != nil {
		log.Warn().Err(err).Msg("failed to create tools executor, file/command tools not available")
	} else {
		toolsExecutor.RegisterBuiltInTools()
		if cfg.Security.Sandbox.Mode == "non_main" {
			toolsExecutor.SetSandbox(cfg.Security.Sandbox.Mode, cfg.Security.Sandbox.Image)
			log.Info().Str("sandbox_mode", cfg.Security.Sandbox.Mode).Str("image", cfg.Security.Sandbox.Image).Msg("sandbox enabled for non-main sessions")
		}
		// Wire session repo for sandbox (and media async tasks)
		if sessionRepo != nil && afkTaskRepo != nil {
			toolsExecutor.SetRepositories(afkTaskRepo, sessionRepo)
		}
		log.Info().Str("security_config", securityConfigPath).Msg("file and command tools registered")

		// Create and register media generation provider
		mediaConfig := getMediaProviderConfig(cfg)
		log.Debug().Int("config_size", len(mediaConfig)).Interface("config", mediaConfig).Msg("media config loaded")
		if len(mediaConfig) > 0 {
			mediaProv, err := media.CreateProvider(cfg.Media.Default, mediaConfig)
			if err != nil {
				log.Warn().Err(err).Msg("failed to create media provider, image/video tools not available")
			} else {
				toolsExecutor.SetMediaProvider(mediaProv)

				// Reuse OSS uploader for media (videos, images) if already created for device screenshots
				if ossUploaderForDevice != nil {
					toolsExecutor.SetMediaUploader(newOSSVideoUploader(ossUploaderForDevice))
					toolsExecutor.SetImageUploader(newOSSMediaImageUploader(ossUploaderForDevice))
					log.Info().Msg("OSS uploader configured for media storage")
				} else if cfg.OSS.AccessKeyID != "" && cfg.OSS.AccessKeySecret != "" {
					var ossUploader *gateway.OSSUploader
					ossUploader, err = gateway.NewOSSUploaderWithConfig(gateway.OSSConfig{
						AccessKeyID:     cfg.OSS.AccessKeyID,
						AccessKeySecret: cfg.OSS.AccessKeySecret,
						Bucket:          cfg.OSS.Bucket,
						Endpoint:        cfg.OSS.Endpoint,
						Domain:          cfg.OSS.Domain,
						PathPrefix:      cfg.OSS.PathPrefix,
					})
					if err != nil {
						log.Warn().Err(err).Msg("failed to create OSS uploader, videos will use direct URLs")
					} else {
						ossUploaderForDevice = ossUploader
						toolsExecutor.SetMediaUploader(newOSSVideoUploader(ossUploader))
						toolsExecutor.SetImageUploader(newOSSMediaImageUploader(ossUploader))
						log.Info().Msg("OSS uploader configured for media storage")
					}
				}

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

	// Register git workflow tools
	if toolsExecutor != nil {
		toolsExecutor.RegisterGitTools()
		log.Info().Msg("git workflow tools registered")
	}

	// Register file operations tools (download_file, etc.)
	if toolsExecutor != nil {
		toolsExecutor.RegisterFileOperationsTools()
		log.Info().Msg("file operations tools registered")
	}

	// Register install tools (skills, rules, MCP)
	if toolsExecutor != nil {
		installExecutor := tools.NewInstallExecutor(engine, toolsExecutor)
		// Set API base to local server
		installExecutor.SetAPIBase(fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port))
		// Always set a registry: builtin (default) + optional remote
		builtinReg, err := skill.NewBuiltinRegistry()
		if err != nil {
			log.Warn().Err(err).Msg("failed to load builtin skill registry")
		}
		var reg skill.SkillRegistryClient
		if builtinReg != nil && cfg.Skills.RegistryURL != "" {
			reg = skill.NewCompositeRegistry(builtinReg, skill.NewRemoteRegistry(cfg.Skills.RegistryURL))
			log.Info().Str("registry", cfg.Skills.RegistryURL).Msg("skill registry configured (builtin + remote)")
		} else if builtinReg != nil {
			reg = builtinReg
			log.Info().Msg("skill registry configured (builtin only, set skills.registry_url for more)")
		} else if cfg.Skills.RegistryURL != "" {
			reg = skill.NewRemoteRegistry(cfg.Skills.RegistryURL)
			log.Info().Str("registry", cfg.Skills.RegistryURL).Msg("skill registry configured (remote only)")
		}
		if reg != nil {
			installExecutor.SetRegistry(reg)
		}
		installExecutor.RegisterBuiltInTools()
		log.Info().Msg("install tools registered (skills, rules, MCP)")
	}

	// Register cron tools
	cronExecutor := tools.NewCronExecutor(engine)
	cronExecutor.RegisterBuiltInTools()
	log.Info().Msg("cron tools registered")

	// Create AFK task scheduler and services
	var afkScheduler *afkpkg.Scheduler
	if db != nil && afkTaskRepo != nil && sessionRepo != nil {
		afkTaskAPI := api.NewAFKTaskAPI(db)

		// Create AFK services
		taskExecutor := afkpkg.NewTaskExecutor(engine)
		notificationService := afkpkg.NewNotificationService(afkTaskRepo)

		// Configure notification service from config/environment
		if telegramToken := os.Getenv("TELEGRAM_BOT_TOKEN"); telegramToken != "" {
			notificationService.SetTelegramBotToken(telegramToken)
		}
		if resendAPIKey := os.Getenv("RESEND_API_KEY"); resendAPIKey != "" {
			fromEmail := os.Getenv("RESEND_FROM_EMAIL")
			notificationService.SetResendConfig(resendAPIKey, fromEmail)
		}

		// Create and start scheduler with sessionRepo for async task handling
		afkScheduler = afkpkg.NewScheduler(afkTaskRepo, sessionRepo, taskExecutor, notificationService)
		afkScheduler.Start(60 * time.Second) // Check every minute
		log.Info().Msg("AFK task scheduler started")

		// Register AFK tools
		afkExecutor := tools.NewAFKExecutor(engine, afkTaskAPI, afkTaskRepo)
		afkExecutor.RegisterBuiltInTools()
		log.Info().Msg("AFK task tools registered")
	}

	// Create Pipeline system for complex multi-step workflows
	var pipelineEngine *pipeline.Engine
	var pipelineAPI *api.PipelineAPI
	if db != nil && sessionRepo != nil {
		pipelineRepo := repository.NewPipelineRepository(db)
		videoTaskExecutor := pipeline.NewVideoTaskExecutor(sessionRepo)
		pipelineEngine = pipeline.NewEngine(pipelineRepo, videoTaskExecutor, nil)
		pipelineAPI = api.NewPipelineAPI(db, pipelineEngine)

		// Register pipeline tools with agent
		pipelineExecutor := tools.NewPipelineExecutor(pipelineEngine, pipelineRepo)
		pipelineExecutor.RegisterBuiltInTools(engine)
		log.Info().Msg("Pipeline system initialized with tools registered")

		// Start background pipeline executor (resumes pending/running pipelines)
		go func() {
			ctx := context.Background()
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()

			for range ticker.C {
				pending, err := pipelineRepo.GetPending(ctx)
				if err != nil {
					continue
				}

				for _, p := range pending {
					if p.Status == model.PipelineStatusPending {
						log.Info().Uint("pipeline_id", p.ID).Msg("auto-starting pending pipeline")
						pipelineEngine.Execute(ctx, p.ID)
					}
					// Running pipelines are already being executed
				}
			}
		}()
		log.Info().Msg("Pipeline background executor started")
	}

	registry := engine.GetSkillRegistry()
	log.Info().Int("skills", len(registry.List())).Int("tools", engine.GetToolCount()).Msg("agent initialized")

	// Create session API
	var sessionAPI *api.SessionAPI
	if db != nil {
		sessionAPI = api.NewSessionAPI(db)
	}

	// Create summary and memory services
	var summaryService *agent.SummaryService
	var memoryService *agent.MemoryService
	if db != nil {
		// Ensure default user exists for single-user mode
		userRepo := repository.NewUserRepository(db)
		const defaultUserID = "default"
		const platformWeb = "web"
		defaultUser, err := userRepo.GetByPlatformID(context.Background(), platformWeb, defaultUserID)
		if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
			defaultUser = &model.User{
				Platform:       platformWeb,
				PlatformUserID: defaultUserID,
				Username:       defaultUserID,
				Email:          "default@marstaff.local",
			}
			if err := userRepo.Create(context.Background(), defaultUser); err != nil {
				log.Warn().Err(err).Msg("failed to create default user")
			} else {
				log.Info().Str("user_id", defaultUser.ID).Msg("created default user")
			}
		}

		messageRepo := repository.NewMessageRepository(db)
		memoryRepo := repository.NewMemoryRepository(db)

		// Create summary service
		summaryService = agent.NewSummaryService(engine, prov, messageRepo, sessionRepo, agent.DefaultConversationSummaryConfig())
		log.Info().Msg("Summary service initialized")

		// Create memory service
		memoryService = agent.NewMemoryService(engine, prov, memoryRepo, messageRepo, agent.DefaultMemoryConfig())
		log.Info().Msg("Memory service initialized")
	}

	// Create project API
	var projectAPI *api.ProjectAPI
	if db != nil {
		projectAPI = api.NewProjectAPI(db)
	}

	// Create skills API
	var skillsAPI *api.SkillsAPI
	if db != nil {
		skillsAPI = api.NewSkillsAPI(db, cfg.Skills.Path, registry)
		log.Info().Msg("Skills API initialized")
	}

	// Create token usage API
	var tokenUsageAPI *api.TokenUsageAPI
	if db != nil {
		tokenUsageAPI = api.NewTokenUsageAPI(db)
		log.Info().Msg("Token usage API initialized")
	}

	// Create hub and WebSocket server
	hub := gateway.NewHub()
	go hub.Run()
	server := gateway.NewServer(hub)

	// Register sessions collaboration tools (sessions_list, sessions_history, sessions_send, sessions_spawn)
	if db != nil && sessionRepo != nil {
		messageRepo := repository.NewMessageRepository(db)
		sessionsExecutor := tools.NewSessionsExecutor(engine, executor, sessionRepo, messageRepo, hub)
		sessionsExecutor.RegisterBuiltInTools()
		log.Info().Msg("sessions tools registered")
	}

	// Create async task notifier for WebSocket notifications
	asyncTaskNotifier := gateway.NewAsyncTaskNotifier(hub)
	log.Info().Msg("Async task notifier initialized")

	// Set OSS uploader for background video uploads (DashScope videos will be uploaded to our OSS)
	if ossUploaderForDevice != nil {
		asyncTaskNotifier.SetOSSUploader(ossUploaderForDevice)
		log.Info().Msg("OSS uploader configured for async task notifier (background video upload)")
	}

	// Set session API for message persistence
	if sessionAPI != nil {
		asyncTaskNotifier.SetSessionAPI(sessionAPI)
		log.Info().Msg("Session API configured for async task notifier (message persistence)")
	}

	// Wire the notifier to the AFK scheduler for async task notifications
	if afkScheduler != nil {
		afkScheduler.SetAsyncNotifier(asyncTaskNotifier)
		log.Info().Msg("Async task notifier wired to AFK scheduler")
	}

	// Set up user repository for WebSocket server (to resolve platform_user_id to real user ID)
	if db != nil {
		userRepo := repository.NewUserRepository(db)
		server.SetUserRepository(&userRepoAdapter{repo: userRepo})
	}

	// Start IM adapters (telegram, matrix, discord, slack)
	var startedAdapters []adapter.Adapter
	for _, ac := range cfg.Adapters {
		// Skip telegram unless explicitly enabled via command line flag
		if ac.Type == "telegram" && !enableTelegram {
			log.Info().Msg("telegram adapter disabled (use --enable-telegram or -t flag to enable)")
			continue
		}
		if !ac.Enabled || ac.Type == "websocket" {
			continue
		}
		a, err := createAdapter(ac, executor, sessionAPI, chatProviders, cfg, engine, summaryService, memoryService)
		if err != nil {
			log.Warn().Err(err).Str("type", ac.Type).Msg("failed to create adapter, skipping")
			continue
		}
		if err := a.Start(context.Background()); err != nil {
			log.Warn().Err(err).Str("type", ac.Type).Msg("failed to start adapter, skipping")
			continue
		}
		startedAdapters = append(startedAdapters, a)
		log.Info().Str("type", ac.Type).Msg("adapter started")
	}

	activeSessions := make(map[string]string)

	// Reuse OSS uploader for file upload (already created for device screenshots if configured)
	ossUploader := ossUploaderForDevice
	if ossUploader == nil && cfg.OSS.AccessKeyID != "" && cfg.OSS.AccessKeySecret != "" {
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
			ossUploaderForDevice = ossUploader
			log.Info().Msg("OSS uploader initialized for file upload")
		}
	}

	// Set up message handler - call agent engine directly (in-process)
	server.SetMessageHandler(func(client *gateway.Client, msg *gateway.Message) error {
		startTime := time.Now() // Record start time for latency tracking

		log.Info().
			Str("type", string(msg.Type)).
			Str("user_id", msg.UserID).
			Str("session_id", msg.SessionID).
			Msg("received message")

		// Handle cancel message type
		if msg.Type == gateway.MessageTypeCancel {
			// Cancel pending AFK tasks for this session
			if msg.SessionID != "" && afkTaskRepo != nil {
				ctx := context.Background()
				if err := afkTaskRepo.CancelPendingTasksBySessionID(ctx, msg.SessionID); err != nil {
					log.Error().Err(err).Str("session_id", msg.SessionID).Msg("failed to cancel AFK tasks")
				} else {
					log.Info().Str("session_id", msg.SessionID).Msg("cancelled pending AFK tasks")
				}
			}

			// Send cancellation confirmation
			hub.SendToUser(client.UserID, &gateway.Message{
				Type:      gateway.MessageTypeError,
				UserID:    client.UserID,
				SessionID: msg.SessionID,
				Data:      map[string]interface{}{"error": "Request cancelled"},
				Timestamp: time.Now().Unix(),
			})
			return nil
		}

		if msg.Type != gateway.MessageTypeChat {
			return nil
		}

		// Parse content, content_parts, and settings (provider, vision_provider, plan_mode, work_dir)
		var content string
		var contentParts []gateway.ContentPart
		providerChoice := "zai" // default chat provider
		visionProviderChoice := "qwen" // default
		planMode := false
		var workDirFromMsg string
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
			if s, exists := data["vision_provider"]; exists {
				if vp, ok := s.(string); ok && (vp == "qwen" || vp == "zai") {
					visionProviderChoice = vp
				}
			}
			if p, exists := data["provider"]; exists {
				if prov, ok := p.(string); ok {
					if _, ok := chatProviders[prov]; ok {
						providerChoice = prov
					}
				}
			}
			if pm, exists := data["plan_mode"]; exists {
				if b, ok := pm.(bool); ok {
					planMode = b
				}
			}
			if wd, exists := data["work_dir"]; exists {
				if s, ok := wd.(string); ok {
					workDirFromMsg = s
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

		// Create or ensure session exists (avoids FK constraint when client provides stale session_id)
		originalSessionID := client.SessionID
		sessionID := client.SessionID
		if sessionID == "" {
			sessionID = uuid.New().String()
			client.SessionID = sessionID
			activeSessions[client.ID] = sessionID
			hub.AddClientToSession(client, sessionID)
		}
		isNewSession := originalSessionID == ""

		if sessionID != "" && sessionAPI != nil {
			ctx := context.Background()
			platformUserID := client.PlatformUserID
			if platformUserID == "" {
				platformUserID = "default"
			}
			createReq := &api.CreateSessionRequest{
				SessionID: sessionID,
				UserID:    platformUserID, // Must be platform_user_id (e.g. "default") for GetOrCreateByPlatformID
				Platform:  "web",
				Title:     truncateRunes(content, 50),
				Model:     "default",
			}
			if workDirFromMsg != "" {
				createReq.WorkDir = workDirFromMsg
			}
			_, err := sessionAPI.GetOrCreateSessionDirect(ctx, createReq)
			if err != nil {
				log.Error().Err(err).Str("session_id", sessionID).Msg("failed to get or create session")
				hub.SendToUser(client.UserID, &gateway.Message{
					Type:      gateway.MessageTypeError,
					UserID:    client.UserID,
					SessionID: sessionID,
					Data:      map[string]interface{}{"error": "无法创建会话，请刷新后重试"},
					Timestamp: time.Now().Unix(),
				})
				return fmt.Errorf("session creation failed: %w", err)
			}

			// Add client to session index for notifications
			hub.AddClientToSession(client, sessionID)
		}

		// Generate session title summary for new chats (async, non-blocking)
		// Use user's selected chat_provider so session title respects user preference (e.g. qwen vs zai)
		if isNewSession && sessionID != "" && sessionAPI != nil {
			var titleProvider provider.Provider
			if cp, ok := chatProviders[providerChoice]; ok {
				titleProvider = cp
			}
			go func(sid string, userContent string, uid string, p provider.Provider) {
				ctx := context.Background()
				title := engine.GenerateSessionTitle(ctx, userContent, p)
				if err := sessionAPI.UpdateSessionTitleDirect(ctx, sid, title); err != nil {
					log.Warn().Err(err).Str("session_id", sid).Msg("failed to update session title")
					return
				}
				hub.SendToUser(uid, &gateway.Message{
					Type:      gateway.MessageTypeSessionTitle,
					UserID:    uid,
					SessionID: sid,
					Data:      map[string]interface{}{"title": title},
					Timestamp: time.Now().Unix(),
				})
			}(sessionID, content, client.UserID, titleProvider)
		}

		// Save user message (assistant response is saved by engine.Chat/engine.ChatStreamWithCallback)
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
		log.Debug().Str("user_id", client.UserID).Str("session_id", client.SessionID).Msg("sending typing=true to client")
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
				log.Debug().Str("user_id", client.UserID).Str("session_id", sessionID).Msg("sending typing=false to client")
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
					provParts[i].ImageURL = &provider.ImageURLStruct{URL: p.ImageURL.URL}
				}
			}

			chatReq := &agent.ChatRequest{
				SessionID:   sessionID,
				UserID:      client.UserID,
				Messages:    []provider.Message{{Role: provider.RoleUser, Content: content, ContentParts: provParts}},
				PlanMode:    planMode,
			}
			// Use selected chat provider if available (priority: user selection > vision provider for images > default)
			if cp, ok := chatProviders[providerChoice]; ok {
				chatReq.ProviderOverride = cp
				log.Info().Str("provider", providerChoice).Msg("using selected chat provider")
			}
			// When message has images and no chat provider was selected, use vision provider + vision_model
			hasImages := false
			for _, p := range provParts {
				if p.Type == "image_url" && p.ImageURL != nil {
					hasImages = true
					break
				}
			}
			if hasImages && chatReq.ProviderOverride == nil {
				if vp, ok := visionProviders[visionProviderChoice]; ok {
					provCfg := getProviderConfig(cfg, visionProviderChoice)
					if provCfg != nil {
						if vm, ok := provCfg["vision_model"].(string); ok && vm != "" {
							chatReq.ProviderOverride = vp
							chatReq.Model = vm
						}
					}
				}
			}
			// Z.ai 与 zhipu 均为智谱 GLM 平台，均支持 thinking
			effectiveProv := prov
			if chatReq.ProviderOverride != nil {
				effectiveProv = chatReq.ProviderOverride
			}
			if effectiveProv.Name() == "zhipu" || effectiveProv.Name() == "zai" {
				chatReq.Thinking = &provider.ThinkingParams{Type: "enabled"}
			}

			ctx := context.WithValue(context.Background(), "current_time", time.Now().Format("2006-01-02 15:04:05"))
			ctx = context.WithValue(ctx, "user_id", client.UserID)
			ctx = context.WithValue(ctx, "session_id", sessionID)

			// Inject summary and memories into chat context (if available)
			if summaryService != nil || memoryService != nil {
				var contextParts []string

				// Add conversation summary if exists
				if summaryService != nil && sessionID != "" {
					if summary, recentMsgs, err := summaryService.GetSummaryWithRecent(ctx, sessionID, 5); err == nil {
						if summary != "" {
							contextParts = append(contextParts, fmt.Sprintf("[对话摘要]\n%s\n", summary))
						}
						// Use recent messages from summary service instead of full history.
						// Set SkipHistory to avoid engine adding history again (would cause duplication).
						if len(recentMsgs) > 0 {
							chatReq.Messages = recentMsgs
							chatReq.SkipHistory = true
						}
					}
				}

				// Add relevant memories
				if memoryService != nil && client.UserID != "" {
					// Extract keywords from user message for memory retrieval
					query := content
					if len(query) > 100 {
						query = query[:100]
					}
					if memories, err := memoryService.RetrieveRelevantMemories(ctx, client.UserID, query, ""); err == nil {
						if memoryText := memoryService.FormatMemoriesForPrompt(memories); memoryText != "" {
							contextParts = append(contextParts, memoryText)
						}
					}
				}

				// Inject context as system message
				if len(contextParts) > 0 {
					contextMsg := fmt.Sprintf("[参考信息]\n%s\n请基于以上信息回答用户的问题。", strings.Join(contextParts, "\n\n"))
					chatReq.Messages = append([]provider.Message{
						{Role: provider.RoleSystem, Content: contextMsg},
					}, chatReq.Messages...)
				}
			}

			onChunk := func(contentDelta, thinkingDelta string) {
				if thinkingDelta != "" {
					hub.SendToUser(client.UserID, &gateway.Message{
						Type:      gateway.MessageTypeThinking,
						UserID:    client.UserID,
						SessionID: sessionID,
						Data:      map[string]interface{}{"delta": thinkingDelta},
						Timestamp: time.Now().Unix(),
					})
				}
				if contentDelta != "" {
					hub.SendToUser(client.UserID, &gateway.Message{
						Type:      gateway.MessageTypeContent,
						UserID:    client.UserID,
						SessionID: sessionID,
						Data:      map[string]interface{}{"delta": contentDelta},
						Timestamp: time.Now().Unix(),
					})
				}
			}
			resp, err := executor.ExecuteWithToolsStream(ctx, chatReq, onChunk)
			if err != nil {
				log.Error().Err(err).Msg("failed to get agent response")

				// Save error message to chat history
				errorMsg := fmt.Sprintf("抱歉，处理您的请求时出错了：\n\n%s", err.Error())
				if sessionID != "" && sessionAPI != nil {
					ctx := context.Background()
					_ = sessionAPI.AddMessageToSession(ctx, sessionID, &api.AddMessageRequest{
						Role:    "assistant",
						Content: errorMsg,
					})
				}

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
				response = "（后端返回空响应，请检查 Agent 服务是否正常运行）"
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

				// Calculate latency
				latency := time.Since(startTime).Seconds()
				latencyFormatted := fmt.Sprintf("%.2f", latency)

				// Send the user-friendly message
				if userMessage != "" {
					chatData := map[string]interface{}{"content": userMessage}
					if resp.Usage.TotalTokens > 0 || resp.Usage.PromptTokens > 0 || resp.Usage.CompletionTokens > 0 {
						chatData["usage"] = map[string]interface{}{
							"prompt_tokens":     resp.Usage.PromptTokens,
							"completion_tokens": resp.Usage.CompletionTokens,
							"total_tokens":      resp.Usage.TotalTokens,
						}
					}
					// Add latency info
					chatData["latency"] = latencyFormatted
					chatData["latency_seconds"] = latency

					hub.SendToUser(client.UserID, &gateway.Message{
						Type:      gateway.MessageTypeChat,
						UserID:    client.UserID,
						SessionID: sessionID,
						Data:      chatData,
						Timestamp: time.Now().Unix(),
					})
				}
			} else {
				// Normal chat response
				// Note: Assistant message is saved by engine.Chat/engine.ChatStreamWithCallback
				sendTypingDone()

				// Calculate latency in seconds (with 2 decimal places)
				latency := time.Since(startTime).Seconds()
				latencyFormatted := fmt.Sprintf("%.2f", latency)

				chatData := map[string]interface{}{"content": response}
				if resp.Thinking != "" {
					chatData["thinking"] = resp.Thinking
				}
				if resp.Usage.TotalTokens > 0 || resp.Usage.PromptTokens > 0 || resp.Usage.CompletionTokens > 0 {
					chatData["usage"] = map[string]interface{}{
						"prompt_tokens":     resp.Usage.PromptTokens,
						"completion_tokens": resp.Usage.CompletionTokens,
						"total_tokens":      resp.Usage.TotalTokens,
					}
				}
				// Add latency info
				chatData["latency"] = latencyFormatted
				chatData["latency_seconds"] = latency

				hub.SendToUser(client.UserID, &gateway.Message{
					Type:      gateway.MessageTypeChat,
					UserID:    client.UserID,
					SessionID: sessionID,
					Data:      chatData,
					Timestamp: time.Now().Unix(),
				})

				// Resolve provider for background tasks (memory/summary) - use user's chat_provider choice
				var bgProvider provider.Provider
				if cp, ok := chatProviders[providerChoice]; ok {
					bgProvider = cp
				}

				// Auto-trigger: Check if summarization is needed
				if summaryService != nil && sessionID != "" {
					go func(sid string, p provider.Provider) {
						ctx := context.Background()
						if shouldSummarize, err := summaryService.ShouldSummarize(ctx, sid); err == nil && shouldSummarize {
							log.Info().Str("session_id", sid).Msg("auto-triggering conversation summarization")
							if err := summaryService.SummarizeAndArchive(ctx, sid, p); err != nil {
								log.Warn().Err(err).Str("session_id", sid).Msg("failed to summarize conversation")
							}
						}
					}(sessionID, bgProvider)
				}

				// Auto-trigger: Extract and save memories
				if memoryService != nil && sessionID != "" && client.UserID != "" {
					go func(sid, uid string, p provider.Provider) {
						ctx := context.Background()
						log.Info().Str("session_id", sid).Msg("auto-triggering memory extraction")
						if err := memoryService.ExtractAndSave(ctx, uid, sid, p); err != nil {
							log.Warn().Err(err).Str("session_id", sid).Msg("failed to extract memories")
						}
					}(sessionID, client.UserID, bgProvider)
				}
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
		c.Writer.Header().Set("X-Marstaff-Version", Version)

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	// Web UI
	router.Static("/static", "./web/static")
	router.GET("/", func(c *gin.Context) {
		c.File("./web/templates/welcome.html")
	})
	router.GET("/welcome", func(c *gin.Context) {
		c.File("./web/templates/welcome.html")
	})
	router.GET("/chat", func(c *gin.Context) {
		c.File("./web/templates/chat.html")
	})
	router.GET("/programming", func(c *gin.Context) {
		c.File("./web/templates/chat.html")
	})
	router.GET("/settings", func(c *gin.Context) {
		c.File("./web/templates/settings.html")
	})
	router.GET("/afk", func(c *gin.Context) {
		c.File("./web/templates/afk.html")
	})
	router.GET("/afk-settings", func(c *gin.Context) {
		c.File("./web/templates/afk-settings.html")
	})
	router.GET("/ws", server.ServeWebSocket)

	// API routes
	apiGroup := router.Group("/api")
	{
		apiGroup.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"status":   "ok",
				"version":  Version,
				"provider": prov.Name(),
				"clients":  server.GetClientCount(),
				"database": db != nil,
			})
		})

		apiGroup.GET("/settings", func(c *gin.Context) {
			visionProvidersList := []string{}
			for name := range visionProviders {
				visionProvidersList = append(visionProvidersList, name)
			}
			if len(visionProvidersList) == 0 {
				visionProvidersList = []string{"qwen", "zai"}
			}
			chatProvidersList := []string{}
			for name := range chatProviders {
				chatProvidersList = append(chatProvidersList, name)
			}
			if len(chatProvidersList) == 0 {
				chatProvidersList = []string{"zai", "qwen", "gemini", "deepseek", "minimax", "minimax_intl", "ollama", "vllm"}
			}
			c.JSON(http.StatusOK, gin.H{
				"vision_providers": visionProvidersList,
				"chat_providers":   chatProvidersList,
			})
		})

		// Chat API (for programmatic access)
		apiGroup.POST("/chat", func(c *gin.Context) {
			var req struct {
				SessionID      string                   `json:"session_id"`
				UserID         string                   `json:"user_id"`
				Messages       []chatMessage            `json:"messages"`
				Model          string                   `json:"model"`
				Temperature    float64                  `json:"temperature"`
				Stream         bool                     `json:"stream"`
				Tools          bool                     `json:"tools"`
				PlanMode       bool                     `json:"plan_mode"`
				Thinking       *provider.ThinkingParams `json:"thinking,omitempty"`
				VisionProvider string                   `json:"vision_provider"`
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

			visionChoice := req.VisionProvider
			if visionChoice != "qwen" && visionChoice != "zai" {
				visionChoice = "qwen"
			}

			modelName := req.Model
			var providerOverride provider.Provider
			if hasImages && modelName == "" {
				if vp, ok := visionProviders[visionChoice]; ok {
					if provCfg := getProviderConfig(cfg, visionChoice); provCfg != nil {
						if vm, ok := provCfg["vision_model"].(string); ok && vm != "" {
							providerOverride = vp
							modelName = vm
						}
					}
				}
				if modelName == "" {
					if provCfg := getProviderConfig(cfg, cfg.Provider.Default); provCfg != nil {
						if vm, ok := provCfg["vision_model"].(string); ok && vm != "" {
							modelName = vm
						}
					}
				}
			}

			chatReq := &agent.ChatRequest{
				SessionID:        req.SessionID,
				UserID:           req.UserID,
				Messages:         provMessages,
				Model:            modelName,
				Temperature:      req.Temperature,
				PlanMode:         req.PlanMode,
				Thinking:         req.Thinking,
				ProviderOverride: providerOverride,
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

		// Skills routes are handled by SkillsAPI below
		// Note: SkillsAPI also provides MCP, rules, and tool management

		// Workspace API for programming mode (create new project)
		workspaceAPI := api.NewWorkspaceAPI(cfg.Workspace.BasePath)
		apiGroup.POST("/workspaces", workspaceAPI.CreateWorkspace)

		sessions := apiGroup.Group("/sessions")
		if sessionAPI != nil {
			sessions.POST("", sessionAPI.CreateSession)
			sessions.GET("/:id", sessionAPI.GetSession)
			sessions.GET("", sessionAPI.ListSessions)
			sessions.PATCH("/:id", sessionAPI.UpdateSession)
			sessions.DELETE("/:id", sessionAPI.DeleteSession)
			sessions.POST("/:id/messages", sessionAPI.AddMessage)
			sessions.GET("/:id/messages", sessionAPI.GetMessages)

			// Summary and Memory endpoints
			sessions.GET("/:id/summary", sessionAPI.GetSessionSummary)
			sessions.POST("/:id/summarize", sessionAPI.TriggerSummary)
			sessions.POST("/:id/extract-memories", sessionAPI.TriggerMemoryExtraction)
		} else {
			sessions.GET("", func(c *gin.Context) {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not connected", "sessions": []interface{}{}})
			})
		}

		// Memory search endpoint
		apiGroup.GET("/memories/search", sessionAPI.SearchMemories)

		// Project API
		projects := apiGroup.Group("/projects")
		if projectAPI != nil {
			projects.POST("", projectAPI.CreateProject)
			projects.GET("", projectAPI.ListProjects)
			projects.GET("/templates", projectAPI.ListTemplates)
			projects.GET("/:id", projectAPI.GetProject)
			projects.PATCH("/:id", projectAPI.UpdateProject)
			projects.DELETE("/:id", projectAPI.DeleteProject)
			projects.GET("/:id/sessions", projectAPI.GetProjectSessions)
		}

		// Skills, Rules, and MCP API
		if skillsAPI != nil {
			skillsAPI.RegisterRoutes(apiGroup)
			log.Info().Msg("Skills, Rules, and MCP API routes registered")
		}

		// Token Usage API
		if tokenUsageAPI != nil {
			tokenUsageAPI.RegisterRoutes(apiGroup)
			log.Info().Msg("Token usage API routes registered")
		}

		// Pipeline API
		if pipelineAPI != nil {
			pipelineAPI.RegisterRoutes(apiGroup)
			log.Info().Msg("Pipeline API routes registered")
		}

		// AFK Task API
		if db != nil {
			afkTaskAPI := api.NewAFKTaskAPI(db)
			afkTasks := apiGroup.Group("/afk/tasks")
			{
				afkTasks.POST("", afkTaskAPI.CreateTask)
				afkTasks.POST("/default-heartbeat", afkTaskAPI.CreateDefaultHeartbeatTask)
				afkTasks.GET("", afkTaskAPI.ListTasks)
				afkTasks.GET("/:id", afkTaskAPI.GetTask)
				afkTasks.PATCH("/:id", afkTaskAPI.UpdateTask)
				afkTasks.DELETE("/:id", afkTaskAPI.DeleteTask)
				afkTasks.GET("/:id/executions", afkTaskAPI.GetTaskExecutions)
			}

			afkSettings := apiGroup.Group("/afk/notification-settings")
			{
				afkSettings.GET("/:user_id", afkTaskAPI.GetNotificationSettings)
				afkSettings.PATCH("/:user_id", afkTaskAPI.UpdateNotificationSettings)
			}

			// Test Telegram notification endpoint
			apiGroup.POST("/afk/test-telegram", func(c *gin.Context) {
				var req struct {
					BotToken string `json:"bot_token"`
					ChatID   string `json:"chat_id"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}

				// Send test message
				url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", req.BotToken)
				payload := map[string]interface{}{
					"chat_id": req.ChatID,
					"text":    "✅ Test notification from Marstaff AFK Task System\n\nYour Telegram bot is configured correctly!",
				}

				jsonPayload, _ := json.Marshal(payload)
				httpReq, err := http.NewRequest("POST", url, bytes.NewReader(jsonPayload))
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				httpReq.Header.Set("Content-Type", "application/json")

				client := &http.Client{Timeout: 10 * time.Second}
				resp, err := client.Do(httpReq)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					var result map[string]interface{}
					json.NewDecoder(resp.Body).Decode(&result)
					c.JSON(http.StatusBadRequest, gin.H{"error": result})
					return
				}

				c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Test notification sent"})
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

	// Stop AFK scheduler
	if afkScheduler != nil {
		afkScheduler.Stop()
		log.Info().Msg("AFK task scheduler stopped")
	}

	// Stop IM adapters
	for _, a := range startedAdapters {
		if err := a.Stop(context.Background()); err != nil {
			log.Warn().Err(err).Msg("adapter stop error")
		}
	}
	if len(startedAdapters) > 0 {
		log.Info().Msg("IM adapters stopped")
	}

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
	case "gemini":
		return cfg.Provider.Gemini
	case "deepseek":
		return cfg.Provider.DeepSeek
	case "minimax":
		return cfg.Provider.MiniMax
	case "minimax_intl":
		return cfg.Provider.MiniMaxIntl
	case "openai":
		return cfg.Provider.OpenAI
	case "zhipu":
		return cfg.Provider.Zhipu
	case "ollama":
		return cfg.Provider.Ollama
	case "vllm":
		return cfg.Provider.VLLM
	default:
		return nil
	}
}

func getMediaProviderConfig(cfg *config.Config) map[string]interface{} {
	log.Debug().Str("default", cfg.Media.Default).Msg("getting media provider config")

	switch cfg.Media.Default {
	case "wanxiang_2.6":
		log.Debug().Interface("config", cfg.Media.Wanxiang26).Msg("using wanxiang_2.6 config")
		return cfg.Media.Wanxiang26
	case "qwen_wanxiang": // Backward compatibility
		if cfg.Media.Wanxiang26 != nil {
			log.Debug().Msg("using wanxiang_2.6 config for qwen_wanxiang")
			return cfg.Media.Wanxiang26
		}
		log.Debug().Interface("config", cfg.Media.QWenWanxiang).Msg("using qwen_wanxiang config")
		return cfg.Media.QWenWanxiang
	default:
		log.Warn().Str("default", cfg.Media.Default).Msg("unknown media provider default")
		return nil
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ossVideoUploaderAdapter adapts gateway.OSSUploader to media.VideoUploader
type ossVideoUploaderAdapter struct {
	uploader *gateway.OSSUploader
}

func (a *ossVideoUploaderAdapter) UploadVideoFile(data []byte, filename string) (*media.UploadResult, error) {
	resp, err := a.uploader.UploadVideoFile(data, filename)
	if err != nil {
		return nil, err
	}
	return &media.UploadResult{
		URL:      resp.URL,
		Filename: resp.Filename,
		Size:     resp.Size,
	}, nil
}

// userRepoAdapter adapts repository.UserRepository to gateway.UserRepository
type userRepoAdapter struct {
	repo *repository.UserRepository
}

func (a *userRepoAdapter) GetByPlatformID(ctx context.Context, platform, platformUserID string) (*gateway.User, error) {
	user, err := a.repo.GetByPlatformID(ctx, platform, platformUserID)
	if err != nil {
		return nil, err
	}
	return &gateway.User{
		ID:       user.ID,
		Platform: user.Platform,
		PlatformUserID: user.PlatformUserID,
	}, nil
}

func newOSSVideoUploader(oss *gateway.OSSUploader) media.VideoUploader {
	return &ossVideoUploaderAdapter{uploader: oss}
}

// ossImageUploaderAdapter adapts gateway.OSSUploader to device.ImageUploader for screenshots
type ossImageUploaderAdapter struct {
	uploader *gateway.OSSUploader
}

func (a *ossImageUploaderAdapter) UploadImagePNG(data []byte, filename string) (string, error) {
	resp, err := a.uploader.UploadBytes(data, filename, "image/png")
	if err != nil {
		return "", err
	}
	return resp.URL, nil
}

func newOSSImageUploader(oss *gateway.OSSUploader) device.ImageUploader {
	return &ossImageUploaderAdapter{uploader: oss}
}

// ossMediaImageUploaderAdapter adapts gateway.OSSUploader to media.ImageUploader for generated images
type ossMediaImageUploaderAdapter struct {
	uploader *gateway.OSSUploader
}

func (a *ossMediaImageUploaderAdapter) UploadImage(data []byte, filename, contentType string) (string, error) {
	resp, err := a.uploader.UploadBytes(data, filename, contentType)
	if err != nil {
		return "", err
	}
	return resp.URL, nil
}

func newOSSMediaImageUploader(oss *gateway.OSSUploader) media.ImageUploader {
	return &ossMediaImageUploaderAdapter{uploader: oss}
}

// createAdapter creates an IM adapter and wires the message handler
func createAdapter(
	ac config.AdapterConfig,
	executor *agent.Executor,
	sessionAPI *api.SessionAPI,
	chatProviders map[string]provider.Provider,
	cfg *config.Config,
	engine *agent.Engine,
	summaryService *agent.SummaryService,
	memoryService *agent.MemoryService,
) (adapter.Adapter, error) {
	var a adapter.Adapter
	switch ac.Type {
	case "telegram":
		token := getAdapterConfigString(ac.Config, "bot_token", "TELEGRAM_BOT_TOKEN")
		if token == "" {
			log.Warn().Str("type", ac.Type).Msg("adapter skipped: bot_token not configured")
			return nil, fmt.Errorf("telegram: bot_token required (set TELEGRAM_BOT_TOKEN or config.bot_token)")
		}
		ta, err := adapter.NewTelegramAdapter(token)
		if err != nil {
			return nil, err
		}
		a = ta
	case "matrix":
		homeserver := getAdapterConfigString(ac.Config, "homeserver", "")
		username := getAdapterConfigString(ac.Config, "bot_username", "MATRIX_BOT_USERNAME")
		password := getAdapterConfigString(ac.Config, "bot_password", "MATRIX_BOT_PASSWORD")
		if homeserver == "" || username == "" || password == "" {
			log.Warn().Str("type", ac.Type).Msg("adapter skipped: homeserver/bot_username/bot_password not configured")
			return nil, fmt.Errorf("matrix: homeserver, bot_username, bot_password required")
		}
		ma, err := adapter.NewMatrixAdapter(homeserver, username, password)
		if err != nil {
			return nil, err
		}
		a = ma
	case "discord":
		token := getAdapterConfigString(ac.Config, "bot_token", "DISCORD_BOT_TOKEN")
		if token == "" {
			log.Warn().Str("type", ac.Type).Msg("adapter skipped: bot_token not configured")
			return nil, fmt.Errorf("discord: bot_token required (set DISCORD_BOT_TOKEN or config.bot_token)")
		}
		da, err := adapter.NewDiscordAdapter(token)
		if err != nil {
			return nil, err
		}
		a = da
	case "slack":
		token := getAdapterConfigString(ac.Config, "bot_token", "SLACK_BOT_TOKEN")
		if token == "" {
			log.Warn().Str("type", ac.Type).Msg("adapter skipped: bot_token not configured")
			return nil, fmt.Errorf("slack: bot_token required (set SLACK_BOT_TOKEN or config.bot_token)")
		}
		sa, err := adapter.NewSlackAdapter(token)
		if err != nil {
			return nil, err
		}
		a = sa
	default:
		return nil, fmt.Errorf("unknown adapter type: %s", ac.Type)
	}

	handler := makeAdapterMessageHandler(a, executor, sessionAPI, chatProviders, cfg, engine, summaryService, memoryService)
	a.SetMessageHandler(handler)
	return a, nil
}

// makeAdapterMessageHandler creates a MessageHandler that runs the agent and sends response via the adapter
func makeAdapterMessageHandler(
	adp adapter.Adapter,
	executor *agent.Executor,
	sessionAPI *api.SessionAPI,
	chatProviders map[string]provider.Provider,
	cfg *config.Config,
	engine *agent.Engine,
	summaryService *agent.SummaryService,
	memoryService *agent.MemoryService,
) adapter.MessageHandler {
	return func(ctx context.Context, msg *adapter.Message) error {
		platform := string(adp.Platform())
		platformUserID := msg.UserID
		if platformUserID == "" {
			platformUserID = "unknown"
		}
		sessionID := msg.SessionID
		if sessionID == "" {
			sessionID = uuid.New().String()
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			return nil
		}

		// Resolve destination for SendMessage (channel_id, chat_id, room_id)
		destID := msg.Metadata["channel_id"]
		if destID == "" {
			destID = msg.Metadata["chat_id"]
		}
		if destID == "" {
			destID = msg.Metadata["room_id"]
		}
		if destID == "" {
			destID = sessionID
		}

		if sessionAPI != nil {
			createReq := &api.CreateSessionRequest{
				SessionID: sessionID,
				UserID:    platformUserID,
				Platform:  platform,
				Title:     truncateRunes(content, 50),
				Model:     "default",
			}
			if platform == "discord" || platform == "slack" {
				// Group/channel sessions are non-main (sandbox applies)
				createReq.IsMainSession = ptr(false)
			}
			if _, err := sessionAPI.GetOrCreateSessionDirect(ctx, createReq); err != nil {
				log.Error().Err(err).Str("session_id", sessionID).Msg("failed to get or create session")
				_ = adp.SendMessage(ctx, destID, sessionID, "抱歉，创建会话失败，请稍后重试。")
				return err
			}
		}

		_ = adp.SendTypingIndicator(ctx, destID)

		providerChoice := cfg.Provider.Default
		if cp, ok := chatProviders[providerChoice]; !ok || cp == nil {
			providerChoice = "zai"
		}
		chatReq := &agent.ChatRequest{
			SessionID: sessionID,
			UserID:    platformUserID,
			Messages:  []provider.Message{{Role: provider.RoleUser, Content: content}},
		}
		if cp, ok := chatProviders[providerChoice]; ok {
			chatReq.ProviderOverride = cp
		}

		ctx = context.WithValue(ctx, "current_time", time.Now().Format("2006-01-02 15:04:05"))
		ctx = context.WithValue(ctx, "user_id", platformUserID)
		ctx = context.WithValue(ctx, "session_id", sessionID)

		if summaryService != nil || memoryService != nil {
			var contextParts []string
			if summaryService != nil && sessionID != "" {
				if summary, recentMsgs, err := summaryService.GetSummaryWithRecent(ctx, sessionID, 5); err == nil {
					if summary != "" {
						contextParts = append(contextParts, fmt.Sprintf("[对话摘要]\n%s", summary))
					}
					if len(recentMsgs) > 0 {
						chatReq.Messages = recentMsgs
					}
				}
			}
			if memoryService != nil && platformUserID != "" {
				query := content
				if len(query) > 100 {
					query = query[:100]
				}
				if memories, err := memoryService.RetrieveRelevantMemories(ctx, platformUserID, query, ""); err == nil {
					if mt := memoryService.FormatMemoriesForPrompt(memories); mt != "" {
						contextParts = append(contextParts, mt)
					}
				}
			}
			if len(contextParts) > 0 {
				contextMsg := fmt.Sprintf("[参考信息]\n%s\n请基于以上信息回答用户。", strings.Join(contextParts, "\n\n"))
				chatReq.Messages = append([]provider.Message{{Role: provider.RoleSystem, Content: contextMsg}}, chatReq.Messages...)
			}
		}

		if sessionAPI != nil {
			_ = sessionAPI.AddMessageToSession(ctx, sessionID, &api.AddMessageRequest{Role: "user", Content: content})
		}

		resp, err := executor.ExecuteWithToolsStream(ctx, chatReq, func(contentDelta, thinkingDelta string) {})
		if err != nil {
			log.Error().Err(err).Msg("adapter agent execution failed")
			_ = adp.SendMessage(ctx, destID, sessionID, "抱歉，处理您的请求时出错了："+err.Error())
			return err
		}

		response := resp.Content
		if response == "" {
			response = "（后端返回空响应）"
		}

		if sessionAPI != nil {
			_ = sessionAPI.AddMessageToSession(ctx, sessionID, &api.AddMessageRequest{Role: "assistant", Content: response})
		}

		return adp.SendMessage(ctx, destID, sessionID, response)
	}
}

func ptr(b bool) *bool { return &b }

func getAdapterConfigString(config map[string]interface{}, key, envFallback string) string {
	if config != nil {
		if v, ok := config[key].(string); ok && v != "" {
			expanded := os.Expand(v, func(k string) string {
				if val := os.Getenv(k); val != "" {
					return val
				}
				return "${" + k + "}"
			})
			if expanded != "" {
				return expanded
			}
		}
	}
	if envFallback != "" {
		return os.Getenv(envFallback)
	}
	return ""
}
