package tools

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/rocky/marstaff/internal/agent"
)

// CronExecutor registers cron/scheduled task tools with the engine
type CronExecutor struct {
	engine *agent.Engine
}

// NewCronExecutor creates a new cron tool executor
func NewCronExecutor(engine *agent.Engine) *CronExecutor {
	return &CronExecutor{
		engine: engine,
	}
}

// RegisterBuiltInTools registers cron tools with the engine
func (e *CronExecutor) RegisterBuiltInTools() {
	e.engine.RegisterTool("cron_list",
		"Lists all scheduled cron jobs for the current user. Use this to view existing scheduled tasks.",
		map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}, e.toolCronList)

	e.engine.RegisterTool("cron_add",
		"Adds a new scheduled cron job. Schedule format: 'minute hour day month weekday' (e.g. '0 3 * * *' = daily at 3am, '*/5 * * * *' = every 5 min). Use 0-59 for minute, 0-23 for hour, 1-31 for day, 1-12 for month, 0-7 for weekday (0 and 7 = Sunday).",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"schedule": map[string]interface{}{
					"type":        "string",
					"description": "Cron schedule: minute hour day month weekday (e.g. '0 3 * * *' for 3am daily)",
				},
				"command": map[string]interface{}{
					"type":        "string",
					"description": "The command to execute (e.g. '/path/to/backup.sh' or 'cd /app && make deploy')",
				},
				"comment": map[string]interface{}{
					"type":        "string",
					"description": "Optional comment/label for this job (prefixed with # in crontab)",
				},
			},
			"required": []string{"schedule", "command"},
		}, e.toolCronAdd)

	e.engine.RegisterTool("cron_remove",
		"Removes a cron job by index (1-based, from cron_list output) or by matching command content.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"index": map[string]interface{}{
					"type":        "integer",
					"description": "1-based index of the job to remove (from cron_list). Use this OR command_match.",
				},
				"command_match": map[string]interface{}{
					"type":        "string",
					"description": "Remove jobs whose command contains this string. Use this OR index.",
				},
			},
		}, e.toolCronRemove)

	e.engine.RegisterTool("cron_logs",
		"Lists log files discovered from cron job commands (from >>, >, 2>> redirections). Shows path, size, last modified. Use before cron_log_read or cron_log_rotate.",
		map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}, e.toolCronLogs)

	e.engine.RegisterTool("cron_log_read",
		"Reads the last N lines of a cron log file. Path must be one from cron_logs output.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Log file path (from cron_logs)",
				},
				"lines": map[string]interface{}{
					"type":        "integer",
					"description": "Number of lines to read from end (default: 100)",
				},
			},
			"required": []string{"path"},
		}, e.toolCronLogRead)

	e.engine.RegisterTool("cron_log_rotate",
		"Rotates/truncates a cron log file: keeps last N lines, discards the rest. Frees disk space. Path must be from cron_logs.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Log file path (from cron_logs)",
				},
				"keep_lines": map[string]interface{}{
					"type":        "integer",
					"description": "Number of lines to keep from end (default: 1000, 0 = clear entirely)",
				},
			},
			"required": []string{"path"},
		}, e.toolCronLogRotate)
}

// runCrontab runs crontab with given args, optionally piping stdin
func runCrontab(ctx context.Context, args []string, stdin string) (string, error) {
	cmd := exec.CommandContext(ctx, "crontab", args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("crontab failed: %w (stderr: %s)", err, strings.TrimSpace(errOut.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// getCrontabLines returns current crontab lines (excluding empty and pure comment lines for job count)
func (e *CronExecutor) getCrontabLines(ctx context.Context) ([]string, error) {
	output, err := runCrontab(ctx, []string{"-l"}, "")
	if err != nil {
		// "no crontab for user" is normal when empty
		if strings.Contains(err.Error(), "no crontab") {
			return nil, nil
		}
		return nil, err
	}
	if output == "" {
		return nil, nil
	}
	return strings.Split(output, "\n"), nil
}

// getJobLines returns only the actual cron job lines (format: schedule + command), with 1-based indices
func getJobLines(lines []string) (jobs []struct {
	Index int
	Line  string
}) {
	jobIndex := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		jobIndex++
		jobs = append(jobs, struct {
			Index int
			Line  string
		}{Index: jobIndex, Line: line})
	}
	return jobs
}

// validateSchedule checks basic cron schedule format (5 fields)
var cronScheduleRe = regexp.MustCompile(`^(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)`)

// logPathRe extracts log file paths from cron command redirections: >>, >, 2>>, 2>
// matches paths like /var/log/x.log, "/path with spaces". excludes 2>&1 (no space before &)
var logPathRe = regexp.MustCompile(`(?:>>|>|2>>|2>)\s+("([^"]+)"|([^\s&]+))`)

func validateSchedule(schedule string) error {
	if !cronScheduleRe.MatchString(strings.TrimSpace(schedule)) {
		return fmt.Errorf("invalid schedule format: need 5 fields (minute hour day month weekday), e.g. '0 3 * * *'")
	}
	return nil
}

// extractLogPaths parses a cron command and returns log file paths from >>, >, 2>>, 2> redirections
func extractLogPaths(command string) []string {
	seen := make(map[string]bool)
	var paths []string
	matches := logPathRe.FindAllStringSubmatch(command, -1)
	for _, m := range matches {
		var p string
		if len(m) >= 3 && m[2] != "" {
			p = m[2] // quoted path
		} else if len(m) >= 4 && m[3] != "" {
			p = m[3] // unquoted path
		}
		if p != "" && p != "/dev/null" && !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}
	return paths
}

// getCronLogPaths returns all log paths from current crontab
func (e *CronExecutor) getCronLogPaths(ctx context.Context) ([]string, error) {
	lines, err := e.getCrontabLines(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var paths []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Extract command part (after 5 schedule fields)
		parts := strings.Fields(trimmed)
		if len(parts) < 6 {
			continue
		}
		command := strings.Join(parts[5:], " ")
		for _, p := range extractLogPaths(command) {
			if !seen[p] {
				seen[p] = true
				paths = append(paths, p)
			}
		}
	}
	return paths, nil
}

func (e *CronExecutor) toolCronList(ctx context.Context, params map[string]interface{}) (string, error) {
	lines, err := e.getCrontabLines(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list crontab: %w", err)
	}
	if len(lines) == 0 {
		return "No cron jobs configured. Use cron_add to add one.", nil
	}

	jobs := getJobLines(lines)
	var b strings.Builder
	b.WriteString("Current cron jobs:\n")
	for _, j := range jobs {
		b.WriteString(fmt.Sprintf("  %d. %s\n", j.Index, j.Line))
	}
	return b.String(), nil
}

func (e *CronExecutor) toolCronAdd(ctx context.Context, params map[string]interface{}) (string, error) {
	schedule, err := getString(params, "schedule", true)
	if err != nil {
		return "", err
	}
	command, err := getString(params, "command", true)
	if err != nil {
		return "", err
	}
	comment, _ := getString(params, "comment", false)

	if err := validateSchedule(schedule); err != nil {
		return "", err
	}

	lines, err := e.getCrontabLines(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to read crontab: %w", err)
	}

	// Build new line
	newLine := schedule + " " + command
	if comment != "" {
		newLine = "# " + comment + "\n" + newLine
	}

	// Append and install
	newCrontab := ""
	if len(lines) > 0 {
		newCrontab = strings.Join(lines, "\n") + "\n"
	}
	newCrontab += newLine + "\n"

	_, err = runCrontab(ctx, []string{"-"}, newCrontab)
	if err != nil {
		return "", fmt.Errorf("failed to add cron job: %w", err)
	}

	log.Info().
		Str("schedule", schedule).
		Str("command", command).
		Msg("cron job added")

	return fmt.Sprintf("Added cron job: %s → %s", schedule, command), nil
}

func (e *CronExecutor) toolCronRemove(ctx context.Context, params map[string]interface{}) (string, error) {
	indexVal, indexOk := params["index"]
	commandMatch, matchOk := params["command_match"]

	if !indexOk && !matchOk {
		return "", fmt.Errorf("must specify either index or command_match")
	}
	if indexOk && matchOk {
		return "", fmt.Errorf("specify only one of index or command_match")
	}

	lines, err := e.getCrontabLines(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to read crontab: %w", err)
	}
	if len(lines) == 0 {
		return "No cron jobs to remove.", nil
	}

	jobs := getJobLines(lines)
	var toRemove []int

	if indexOk {
		var idx int
		switch v := indexVal.(type) {
		case int:
			idx = v
		case float64:
			idx = int(v)
		default:
			return "", fmt.Errorf("index must be a number")
		}
		if idx < 1 || idx > len(jobs) {
			return "", fmt.Errorf("index %d out of range (1-%d)", idx, len(jobs))
		}
		toRemove = []int{idx - 1} // convert to 0-based index in jobs slice
	} else {
		matchStr, ok := commandMatch.(string)
		if !ok || matchStr == "" {
			return "", fmt.Errorf("command_match must be a non-empty string")
		}
		for i, j := range jobs {
			if strings.Contains(j.Line, matchStr) {
				toRemove = append(toRemove, i)
			}
		}
		if len(toRemove) == 0 {
			return fmt.Sprintf("No cron job matches '%s'", matchStr), nil
		}
	}

	// Build new crontab without removed lines
	removeSet := make(map[int]bool)
	for _, i := range toRemove {
		removeSet[i] = true
	}

	var newLines []string
	currentJobIndex := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			newLines = append(newLines, line)
			continue
		}
		if removeSet[currentJobIndex] {
			currentJobIndex++
			continue
		}
		newLines = append(newLines, line)
		currentJobIndex++
	}

	newCrontab := strings.Join(newLines, "\n")
	if newCrontab != "" {
		newCrontab += "\n"
	}

	_, err = runCrontab(ctx, []string{"-"}, newCrontab)
	if err != nil {
		return "", fmt.Errorf("failed to remove cron job: %w", err)
	}

	removedCount := len(toRemove)
	log.Info().Int("removed", removedCount).Msg("cron job(s) removed")

	return fmt.Sprintf("Removed %d cron job(s).", removedCount), nil
}

func (e *CronExecutor) toolCronLogs(ctx context.Context, params map[string]interface{}) (string, error) {
	paths, err := e.getCronLogPaths(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get cron log paths: %w", err)
	}
	if len(paths) == 0 {
		return "No log files found in cron job commands. Add redirections like '>> /path/to/log 2>&1' to your cron commands.", nil
	}

	var b strings.Builder
	b.WriteString("Cron log files:\n")
	for _, p := range paths {
		absPath, _ := filepath.Abs(p)
		if absPath == "" {
			absPath = p
		}
		info, err := os.Stat(absPath)
		if err != nil {
			b.WriteString(fmt.Sprintf("  %s - (not found or not readable)\n", p))
			continue
		}
		if info.IsDir() {
			b.WriteString(fmt.Sprintf("  %s - (is directory, not a log file)\n", p))
			continue
		}
		sizeKB := info.Size() / 1024
		modTime := info.ModTime().Format("2006-01-02 15:04")
		b.WriteString(fmt.Sprintf("  %s - %d KB, modified %s\n", p, sizeKB, modTime))
	}
	return b.String(), nil
}

func (e *CronExecutor) toolCronLogRead(ctx context.Context, params map[string]interface{}) (string, error) {
	path, err := getString(params, "path", true)
	if err != nil {
		return "", err
	}
	lines, err := getInt(params, "lines", false, 100)
	if err != nil {
		return "", err
	}
	if lines < 1 {
		lines = 100
	}

	// Path must be from cron jobs (whitelist)
	allowedPaths, err := e.getCronLogPaths(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get allowed paths: %w", err)
	}
	pathAbs, _ := filepath.Abs(path)
	allowed := false
	for _, a := range allowedPaths {
		aAbs, _ := filepath.Abs(a)
		if pathAbs == aAbs || path == a {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", fmt.Errorf("path %s is not a cron log file (use cron_logs to see allowed paths)", path)
	}

	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open log: %w", err)
	}
	defer file.Close()

	// Read last N lines via tail
	var tailLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		tailLines = append(tailLines, scanner.Text())
		if len(tailLines) > lines {
			tailLines = tailLines[1:]
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to read log: %w", err)
	}

	if len(tailLines) == 0 {
		return fmt.Sprintf("Log file %s is empty.", path), nil
	}
	return fmt.Sprintf("Last %d lines of %s:\n%s", len(tailLines), path, strings.Join(tailLines, "\n")), nil
}

func (e *CronExecutor) toolCronLogRotate(ctx context.Context, params map[string]interface{}) (string, error) {
	path, err := getString(params, "path", true)
	if err != nil {
		return "", err
	}
	keepLines, err := getInt(params, "keep_lines", false, 1000)
	if err != nil {
		return "", err
	}

	// Path must be from cron jobs
	allowedPaths, err := e.getCronLogPaths(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get allowed paths: %w", err)
	}
	pathAbs, _ := filepath.Abs(path)
	allowed := false
	for _, a := range allowedPaths {
		aAbs, _ := filepath.Abs(a)
		if pathAbs == aAbs || path == a {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", fmt.Errorf("path %s is not a cron log file (use cron_logs to see allowed paths)", path)
	}

	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open log: %w", err)
	}

	var allLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}
	file.Close()
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to read log: %w", err)
	}

	total := len(allLines)
	var toKeep []string
	switch {
	case keepLines == 0:
		toKeep = nil
	case total <= keepLines:
		toKeep = allLines
	default:
		toKeep = allLines[total-keepLines:]
	}

	content := strings.Join(toKeep, "\n")
	if len(toKeep) > 0 {
		content += "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write log: %w", err)
	}

	removed := total - len(toKeep)
	log.Info().
		Str("path", path).
		Int("removed_lines", removed).
		Int("kept_lines", len(toKeep)).
		Msg("cron log rotated")

	if keepLines == 0 {
		return fmt.Sprintf("Cleared log file %s (was %d lines).", path, total), nil
	}
	return fmt.Sprintf("Rotated %s: kept last %d lines, removed %d lines.", path, len(toKeep), removed), nil
}
