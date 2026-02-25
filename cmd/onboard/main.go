package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rocky/marstaff/internal/config"
)

var (
	configPath string
	installDaemon bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "onboard",
		Short: "Marstaff 交互式配置向导",
		Long:  "引导式配置 Marstaff：数据库、AI 提供商、可选 OSS/Telegram 等，生成 config.yaml 和 .env.example",
		RunE:  runOnboard,
	}

	rootCmd.Flags().StringVarP(&configPath, "config", "c", "configs/config.yaml", "输出配置文件路径")
	rootCmd.Flags().BoolVar(&installDaemon, "install-daemon", false, "生成 systemd/launchd 服务文件")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runOnboard(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("\n=== Marstaff 配置向导 ===")

	// 1. 数据库连接
	fmt.Println("--- 数据库配置 ---")
	dbHost := prompt(reader, "MySQL 主机", "localhost")
	dbPortStr := prompt(reader, "MySQL 端口", "3306")
	dbPort, _ := strconv.Atoi(dbPortStr)
	dbName := prompt(reader, "数据库名", "marstaff")
	dbUser := prompt(reader, "数据库用户", "root")
	dbPass := promptSecret(reader, "数据库密码 (留空回车)", "")

	// 2. AI 提供商
	fmt.Println("\n--- AI 提供商 ---")
	provider := promptSelect(reader, "选择默认 AI 提供商", []string{"zai", "qwen", "gemini", "openai", "zhipu"}, "zai")
	apiKey := promptSecret(reader, "API Key (或使用环境变量 ${XXX})", "")

	// 3. 可选配置
	fmt.Println("\n--- 可选配置 ---")
	skillsPath := prompt(reader, "技能目录", "./skills")
	serverPortStr := prompt(reader, "服务端口", "18789")
	serverPort, _ := strconv.Atoi(serverPortStr)
	ossBucket := prompt(reader, "OSS Bucket (留空跳过)", "")
	telegramToken := promptSecret(reader, "Telegram Bot Token (留空跳过)", "")

	// 4. 生成配置
	cfg := buildConfig(dbHost, dbPort, dbName, dbUser, dbPass, provider, apiKey, skillsPath, serverPort, ossBucket, telegramToken)

	// 确保目录存在
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	// 写入 config.yaml (使用 viper 或手动写 YAML)
	if err := writeConfigYAML(configPath, cfg); err != nil {
		return fmt.Errorf("写入配置失败: %w", err)
	}
	fmt.Printf("\n✓ 配置已写入 %s\n", configPath)

	// 生成 .env.example
	envPath := filepath.Join(filepath.Dir(configPath), "..", ".env.example")
	if err := writeEnvExample(envPath); err != nil {
		fmt.Printf("警告: 写入 .env.example 失败: %v\n", err)
	} else {
		fmt.Printf("✓ 环境变量示例已写入 %s\n", envPath)
	}

	if installDaemon {
		if err := installDaemonService(); err != nil {
			return fmt.Errorf("生成服务文件失败: %w", err)
		}
		fmt.Println("✓ 服务文件已生成，请查看 deployments/ 目录")
	}

	fmt.Println("\n配置完成。运行 `make run` 或 `./bin/gateway -c configs/config.yaml` 启动服务。")
	return nil
}

func prompt(reader *bufio.Reader, label, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("%s: ", label)
	}
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" && defaultVal != "" {
		return defaultVal
	}
	return line
}

func promptSecret(reader *bufio.Reader, label, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("%s [***]: ", label)
	} else {
		fmt.Printf("%s: ", label)
	}
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" && defaultVal != "" {
		return defaultVal
	}
	return line
}

func promptSelect(reader *bufio.Reader, label string, options []string, defaultVal string) string {
	fmt.Printf("%s (%s) [%s]: ", label, strings.Join(options, "/"), defaultVal)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return defaultVal
	}
	for _, o := range options {
		if o == line {
			return o
		}
	}
	return defaultVal
}

func buildConfig(dbHost string, dbPort int, dbName, dbUser, dbPass, provider, apiKey, skillsPath string, serverPort int, ossBucket, telegramToken string) *config.Config {
	cfg := &config.Config{}
	cfg.Database.Host = dbHost
	cfg.Database.Port = dbPort
	cfg.Database.Database = dbName
	cfg.Database.Username = dbUser
	cfg.Database.Password = dbPass
	cfg.Provider.Default = provider
	cfg.Skills.Path = skillsPath
	cfg.Server.Port = serverPort
	cfg.Server.Host = "0.0.0.0"

	// 设置 API Key 占位符 (按所选提供商)
	envKey := "ZHIPU_API_KEY"
	switch provider {
	case "qwen":
		envKey = "QWEN_API_KEY"
	case "gemini":
		envKey = "GEMINI_API_KEY"
	case "openai":
		envKey = "OPENAI_API_KEY"
	case "zhipu":
		envKey = "ZHIPU_API_KEY"
	}
	if apiKey != "" {
		cfg.Provider.ZAI = map[string]interface{}{"api_key": apiKey}
		cfg.Provider.Qwen = map[string]interface{}{"api_key": apiKey}
	} else {
		cfg.Provider.ZAI = map[string]interface{}{"api_key": "${" + envKey + "}"}
		cfg.Provider.Qwen = map[string]interface{}{"api_key": "${" + envKey + "}"}
	}

	// OSS (可选)
	if ossBucket != "" {
		cfg.OSS.Bucket = ossBucket
		cfg.OSS.AccessKeyID = "${ALIYUN_ACCESS_KEY_ID}"
		cfg.OSS.AccessKeySecret = "${ALIYUN_ACCESS_KEY_SECRET}"
		cfg.OSS.Endpoint = "oss-accelerate.aliyuncs.com"
		cfg.OSS.Domain = "https://" + ossBucket + ".oss-accelerate.aliyuncs.com"
		cfg.OSS.PathPrefix = "uploads/"
	}

	// Telegram (可选，启用则生成 adapter 配置，token 用环境变量)
	if telegramToken != "" {
		cfg.Adapters = []config.AdapterConfig{
			{Type: "telegram", Enabled: true, Config: map[string]interface{}{"bot_token": "${TELEGRAM_BOT_TOKEN}"}},
			{Type: "websocket", Enabled: true, Config: map[string]interface{}{"path": "/ws"}},
		}
	}

	return cfg
}

func writeConfigYAML(path string, cfg *config.Config) error {
	// 密码建议用环境变量，这里用占位符
	dbPass := "${MARSTAFF_DB_PASSWORD}"
	if cfg.Database.Password != "" {
		dbPass = cfg.Database.Password
	}

	ossBlock := ""
	if cfg.OSS.Bucket != "" {
		ossBlock = fmt.Sprintf(`
oss:
  access_key_id: "${ALIYUN_ACCESS_KEY_ID}"
  access_key_secret: "${ALIYUN_ACCESS_KEY_SECRET}"
  bucket: "%s"
  endpoint: "oss-accelerate.aliyuncs.com"
  domain: "https://%s.oss-accelerate.aliyuncs.com"
  path_prefix: "uploads/"
`, cfg.OSS.Bucket, cfg.OSS.Bucket)
	}

	adaptersBlock := `
adapters:
  - type: "websocket"
    enabled: true
    config:
      path: "/ws"
`
	if len(cfg.Adapters) > 0 {
		var sb strings.Builder
		sb.WriteString("\nadapters:\n")
		for _, a := range cfg.Adapters {
			if a.Type == "telegram" && a.Enabled {
				sb.WriteString("  - type: \"telegram\"\n    enabled: true\n    config:\n      bot_token: \"${TELEGRAM_BOT_TOKEN}\"\n")
			}
		}
		sb.WriteString("  - type: \"websocket\"\n    enabled: true\n    config:\n      path: \"/ws\"\n")
		adaptersBlock = "\n" + sb.String()
	}

	content := fmt.Sprintf(`# Marstaff 配置 (由 onboard 向导生成)

server:
  host: "%s"
  port: %d
  mode: "debug"

database:
  driver: "mysql"
  host: "%s"
  port: %d
  database: "%s"
  username: "%s"
  password: "%s"

provider:
  default: "%s"
  zai:
    api_key: "${ZHIPU_API_KEY}"
    base_url: "https://api.z.ai/api/paas/v4"
    model: "glm-4-flash"
  qwen:
    api_key: "${QWEN_API_KEY}"
    base_url: "https://dashscope.aliyuncs.com"
    model: "qwen-max"

skills:
  path: "%s"
  auto_load: true
%s%s

log:
  level: "info"
  format: "console"
  output: "stdout"
`,
		cfg.Server.Host, cfg.Server.Port,
		cfg.Database.Host, cfg.Database.Port, cfg.Database.Database, cfg.Database.Username, dbPass,
		cfg.Provider.Default,
		cfg.Skills.Path,
		ossBlock,
		adaptersBlock,
	)
	return os.WriteFile(path, []byte(content), 0644)
}

func writeEnvExample(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	content := `# Marstaff 环境变量示例
# 复制为 .env 并填入实际值

# 数据库 (可选，也可在 config.yaml 中配置)
MARSTAFF_DB_PASSWORD=

# AI 提供商 API Key (按使用的提供商配置)
ZHIPU_API_KEY=
QWEN_API_KEY=
GEMINI_API_KEY=
OPENAI_API_KEY=

# 可选：OSS 存储 (截图、媒体)
ALIYUN_ACCESS_KEY_ID=
ALIYUN_ACCESS_KEY_SECRET=

# 可选：Telegram Bot
TELEGRAM_BOT_TOKEN=
`
	return os.WriteFile(path, []byte(content), 0644)
}

func installDaemonService() error {
	// 创建 deployments 目录
	if err := os.MkdirAll("deployments/systemd", 0755); err != nil {
		return err
	}
	if err := os.MkdirAll("deployments/launchd", 0755); err != nil {
		return err
	}

	// systemd
	systemdContent := `[Unit]
Description=Marstaff AI Agent Gateway
After=network.target mysql.service

[Service]
Type=simple
User=%s
WorkingDirectory=%s
ExecStart=%s -c configs/config.yaml
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
`
	wd, _ := os.Getwd()
	user := os.Getenv("USER")
	if user == "" {
		user = "marstaff"
	}
	gatewayPath := filepath.Join(wd, "bin", "gateway")
	systemdContent = fmt.Sprintf(systemdContent, user, wd, gatewayPath)
	if err := os.WriteFile("deployments/systemd/marstaff.service", []byte(systemdContent), 0644); err != nil {
		return err
	}

	// launchd (macOS)
	launchdContent := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.marstaff.gateway</string>
    <key>ProgramArguments</key>
    <array>
        <string>` + gatewayPath + `</string>
        <string>-c</string>
        <string>` + filepath.Join(wd, "configs", "config.yaml") + `</string>
    </array>
    <key>WorkingDirectory</key>
    <string>` + wd + `</string>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
`
	if err := os.WriteFile("deployments/launchd/com.marstaff.gateway.plist", []byte(launchdContent), 0644); err != nil {
		return err
	}

	return nil
}
