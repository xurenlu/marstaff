package afk

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/rocky/marstaff/internal/envvars"
	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/repository"
)

// parseOutputPath extracts -o or --output path from shell command (e.g. firecrawl -o .firecrawl/result.json)
var outputPathRe = regexp.MustCompile(`(?:^|\s)(?:-o|--output)\s+(?:"([^"]+)"|'([^']+)'|([^\s]+))`)

func parseOutputPath(command, workDir string) string {
	m := outputPathRe.FindStringSubmatch(command)
	if m == nil {
		return ""
	}
	var p string
	for i := 1; i < len(m); i++ {
		if m[i] != "" {
			p = m[i]
			break
		}
	}
	if p == "" {
		return ""
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(workDir, p)
	}
	return p
}

const defaultCommandTimeout = 1800 // 30 minutes

// OneOffFileUploader uploads files to cloud storage (e.g. OSS) for one-off task results.
// When set, log files are uploaded and the public URL is sent in Feishu/email notifications.
type OneOffFileUploader interface {
	UploadBytes(data []byte, filename, contentType string) (url string, err error)
}

// OneOffRunner runs one-off long-running commands in the background
type OneOffRunner struct {
	taskRepo      *repository.AFKTaskRepository
	sessionRepo   *repository.SessionRepository
	notifier      *NotificationService
	asyncNotifier AsyncTaskNotifier
	fileUploader  OneOffFileUploader // optional: upload log to OSS for Feishu link
	envProvider   envvars.Provider   // optional: inject env vars from settings
}

// NewOneOffRunner creates a new one-off task runner
func NewOneOffRunner(
	taskRepo *repository.AFKTaskRepository,
	sessionRepo *repository.SessionRepository,
	notifier *NotificationService,
	asyncNotifier AsyncTaskNotifier,
	fileUploader OneOffFileUploader,
	envProvider envvars.Provider,
) *OneOffRunner {
	return &OneOffRunner{
		taskRepo:      taskRepo,
		sessionRepo:   sessionRepo,
		notifier:      notifier,
		asyncNotifier: asyncNotifier,
		fileUploader:  fileUploader,
		envProvider:   envProvider,
	}
}

// SetEnvProvider sets the env vars provider for command execution
func (r *OneOffRunner) SetEnvProvider(p envvars.Provider) {
	r.envProvider = p
}

// RunOneOffTask starts a command in the background and handles completion.
// Call with: go runner.RunOneOffTask(context.Background(), task)
func (r *OneOffRunner) RunOneOffTask(ctx context.Context, task *model.AFKTask) {
	config := task.TriggerConfig.AsyncTaskConfig
	if config == nil || config.Command == "" {
		log.Error().Str("task_id", task.ID).Msg("oneoff task missing command")
		r.markTaskFailed(ctx, task, "missing command in task config")
		return
	}

	workDir := config.WorkDir
	if workDir == "" {
		workDir = "."
	}
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		r.markTaskFailed(ctx, task, fmt.Sprintf("invalid work_dir: %v", err))
		return
	}
	if _, err := os.Stat(absWorkDir); os.IsNotExist(err) {
		r.markTaskFailed(ctx, task, fmt.Sprintf("work_dir does not exist: %s", absWorkDir))
		return
	}

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultCommandTimeout
	}

	// Create log file for output
	logDir := filepath.Join(absWorkDir, ".firecrawl")
	_ = os.MkdirAll(logDir, 0755)
	logPath := filepath.Join(logDir, fmt.Sprintf("afk-%s.log", task.ID))
	logFile, err := os.Create(logPath)
	if err != nil {
		log.Warn().Err(err).Str("task_id", task.ID).Msg("failed to create log file, using stderr")
		logFile = nil
	} else {
		defer logFile.Close()
	}

	cmdCtx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	// Use login shell (bash -l -c) so PATH includes ~/.nvm, /usr/local/bin, etc.
	// Exit 127 = command not found; often caused by minimal PATH when gateway runs as service.
	shell := "sh"
	args := []string{"-c", config.Command}
	if path, err := exec.LookPath("bash"); err == nil {
		shell = path
		args = []string{"-l", "-c", config.Command}
	}
	cmd := exec.CommandContext(cmdCtx, shell, args...)
	cmd.Dir = absWorkDir
	if logFile != nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	// Inject env vars from settings (overrides os.Environ)
	if r.envProvider != nil {
		if env, err := r.envProvider.GetMergedEnv(ctx); err == nil && len(env) > 0 {
			cmd.Env = env
			hasFirecrawlKey := false
			for _, e := range env {
				if strings.HasPrefix(e, "FIRECRAWL_API_KEY=") {
					hasFirecrawlKey = true
					break
				}
			}
			log.Info().
				Str("task_id", task.ID).
				Int("env_count", len(env)).
				Bool("has_firecrawl_key", hasFirecrawlKey).
				Msg("injected env vars from settings")
		} else {
			log.Warn().Err(err).Str("task_id", task.ID).Msg("failed to get env vars from settings, using parent env")
		}
	} else {
		log.Warn().Str("task_id", task.ID).Msg("env provider not configured, oneoff commands use parent process env only")
	}

	// When running firecrawl, set NO_PROXY to bypass proxy for api.firecrawl.dev.
	// "Error: protocol mismatch" from follow-redirects often occurs when HTTP_PROXY/HTTPS_PROXY
	// causes redirect protocol issues with the Firecrawl API.
	if strings.Contains(strings.ToLower(config.Command), "firecrawl") {
		if cmd.Env == nil {
			cmd.Env = os.Environ()
		}
		noProxy := "api.firecrawl.dev,*.firecrawl.dev,firecrawl.dev"
		for i, e := range cmd.Env {
			if strings.HasPrefix(e, "NO_PROXY=") {
				cmd.Env[i] = "NO_PROXY=" + noProxy + "," + strings.TrimPrefix(e, "NO_PROXY=")
				goto firecrawlEnvDone
			}
		}
		cmd.Env = append(cmd.Env, "NO_PROXY="+noProxy)
	firecrawlEnvDone:
	}

	log.Info().
		Str("task_id", task.ID).
		Str("command", config.Command).
		Str("work_dir", absWorkDir).
		Msg("starting oneoff command")

	err = cmd.Run()

	now := time.Now()
	task.LastExecutionTime = &now
	task.ExecutionCount++

	if err != nil {
		errMsg := err.Error()
		if cmdCtx.Err() == context.DeadlineExceeded {
			errMsg = fmt.Sprintf("command timed out after %d seconds", timeout)
		}
		log.Error().Err(err).Str("task_id", task.ID).Msg("oneoff command failed")
		r.markTaskFailed(ctx, task, errMsg)
		return
	}

	// Success
	task.Status = model.AFKTaskStatusCompleted
	task.ResultURL = logPath
	task.ErrorMessage = ""

	// Upload log or output file to OSS if configured, so Feishu/email can include a clickable URL.
	// Firecrawl and similar tools write results to -o path, not stdout; so log may be empty.
	// Prefer uploading the actual output file when log is empty.
	if r.fileUploader != nil {
		data, readErr := os.ReadFile(logPath)
		if readErr != nil {
			log.Warn().Err(readErr).Str("task_id", task.ID).Str("log_path", logPath).Msg("failed to read log for OSS upload")
		} else {
			uploadPath := logPath
			uploadData := data
			if len(data) == 0 {
				// Log empty: try -o output file (e.g. firecrawl -o .firecrawl/potential_clients.json)
				if outPath := parseOutputPath(config.Command, absWorkDir); outPath != "" {
					if outData, outErr := os.ReadFile(outPath); outErr == nil && len(outData) > 0 {
						uploadPath = outPath
						uploadData = outData
						log.Info().Str("task_id", task.ID).Str("output_path", outPath).Msg("log empty, uploading -o output file instead")
					}
				}
			}
			if len(uploadData) > 0 {
				filename := filepath.Base(uploadPath)
				contentType := "text/plain"
				if strings.HasSuffix(filename, ".json") {
					contentType = "application/json"
				} else if strings.HasSuffix(filename, ".md") {
					contentType = "text/markdown"
				}
				ossURL, uploadErr := r.fileUploader.UploadBytes(uploadData, filename, contentType)
				if uploadErr != nil {
					log.Warn().Err(uploadErr).Str("task_id", task.ID).Msg("failed to upload to OSS, using local path in notification")
				} else {
					task.ResultURL = ossURL
					log.Info().Str("task_id", task.ID).Str("oss_url", ossURL).Msg("oneoff result uploaded to OSS")
				}
			} else {
				log.Warn().Str("task_id", task.ID).Msg("log and output file empty, no OSS upload")
			}
		}
	}

	if err := r.taskRepo.Update(ctx, task); err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("failed to update task on success")
	}

	r.handleCompletion(ctx, task, true)
}

func (r *OneOffRunner) markTaskFailed(ctx context.Context, task *model.AFKTask, errMsg string) {
	task.Status = model.AFKTaskStatusFailed
	task.ErrorMessage = errMsg
	if err := r.taskRepo.Update(ctx, task); err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("failed to update task on failure")
	}
	r.handleCompletion(ctx, task, false)
}

func (r *OneOffRunner) handleCompletion(ctx context.Context, task *model.AFKTask, success bool) {
	if task.SessionID == nil || *task.SessionID == "" || r.sessionRepo == nil {
		r.sendNotification(ctx, task, success)
		return
	}

	session, err := r.sessionRepo.GetByID(ctx, *task.SessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", *task.SessionID).Msg("failed to get session for oneoff completion")
		r.sendNotification(ctx, task, success)
		return
	}

	allComplete := session.OnTaskComplete()
	if err := r.sessionRepo.Update(ctx, session); err != nil {
		log.Error().Err(err).Str("session_id", session.ID).Msg("failed to update session")
	}

	// WebSocket notification
	if r.asyncNotifier != nil {
		if success {
			r.asyncNotifier.NotifyTaskCompleted(*task.SessionID, task, task.ResultURL)
		} else {
			r.asyncNotifier.NotifyTaskFailed(*task.SessionID, task, task.ErrorMessage)
		}
		if allComplete {
			r.asyncNotifier.NotifyAFKStatusChanged(session.ID, false, 0, nil)
		}
	}

	r.sendNotification(ctx, task, success)
}

func (r *OneOffRunner) sendNotification(ctx context.Context, task *model.AFKTask, success bool) {
	if r.notifier == nil {
		return
	}

	var msg string
	if success {
		if strings.HasPrefix(task.ResultURL, "http://") || strings.HasPrefix(task.ResultURL, "https://") {
			msg = fmt.Sprintf("✅ 挂机任务「%s」已完成。\n\n结果文件（可点击）: %s", task.Name, task.ResultURL)
		} else {
			msg = fmt.Sprintf("✅ 挂机任务「%s」已完成。\n\n结果文件: %s", task.Name, task.ResultURL)
		}
	} else {
		msg = fmt.Sprintf("❌ 挂机任务「%s」失败。\n\n错误: %s", task.Name, task.ErrorMessage)
	}

	if err := r.notifier.SendDirectNotification(ctx, task.UserID, msg, nil); err != nil {
		log.Warn().Err(err).Str("task_id", task.ID).Msg("failed to send oneoff task notification")
	}
}

