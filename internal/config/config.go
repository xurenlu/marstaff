package config

import (
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// Config is the main configuration structure
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Provider  ProviderConfig  `mapstructure:"provider"`
	Media     MediaConfig     `mapstructure:"media"`
	OSS       OSSConfig       `mapstructure:"oss"`
	Workspace WorkspaceConfig `mapstructure:"workspace"`
	Skills    SkillsConfig    `mapstructure:"skills"`
	Security  SecurityConfig  `mapstructure:"security"`
	Adapters  []AdapterConfig `mapstructure:"adapters"`
	Log       LogConfig       `mapstructure:"log"`
}

// SecurityConfig holds security-related settings
type SecurityConfig struct {
	Sandbox SandboxConfig `mapstructure:"sandbox"`
}

// SandboxConfig controls Docker sandbox for non-main sessions
type SandboxConfig struct {
	Mode  string `mapstructure:"mode"`  // "off" (default), "non_main" (run non-main sessions in Docker)
	Image string `mapstructure:"image"` // Docker image (default: alpine:latest)
}

// WorkspaceConfig holds workspace configuration for programming mode
type WorkspaceConfig struct {
	BasePath string `mapstructure:"base_path"` // root dir for new projects
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Host         string `mapstructure:"host"`
	Port         int    `mapstructure:"port"`
	Mode         string `mapstructure:"mode"` // debug, release
	ReadTimeout  int    `mapstructure:"read_timeout"`
	WriteTimeout int    `mapstructure:"write_timeout"`
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Driver          string `mapstructure:"driver"`
	Host            string `mapstructure:"host"`
	Port            int    `mapstructure:"port"`
	Database        string `mapstructure:"database"`
	Username        string `mapstructure:"username"`
	Password        string `mapstructure:"password"`
	MaxOpenConns    int    `mapstructure:"max_open_conns"`
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`
	ConnMaxLifetime int    `mapstructure:"conn_max_lifetime"`
}

// ProviderConfig holds AI provider configuration
type ProviderConfig struct {
	Default  string                 `mapstructure:"default"`
	ZAI      map[string]interface{} `mapstructure:"zai"`
	Qwen     map[string]interface{} `mapstructure:"qwen"`
	Gemini   map[string]interface{} `mapstructure:"gemini"`
	DeepSeek map[string]interface{} `mapstructure:"deepseek"`
	MiniMax  map[string]interface{} `mapstructure:"minimax"`
	MiniMaxIntl map[string]interface{} `mapstructure:"minimax_intl"`
	OpenAI   map[string]interface{} `mapstructure:"openai"`
	Zhipu    map[string]interface{} `mapstructure:"zhipu"`
	Ollama   map[string]interface{} `mapstructure:"ollama"`
	VLLM     map[string]interface{} `mapstructure:"vllm"`
	Poe      map[string]interface{} `mapstructure:"poe"`
	Fallback []string               `mapstructure:"fallback"`
}

// MediaConfig holds media generation provider configuration
type MediaConfig struct {
	Default      string                 `mapstructure:"default"`
	Wanxiang26   map[string]interface{} `mapstructure:"wanxiang_2_6"` // Use underscore for YAML compatibility
	QWenWanxiang map[string]interface{} `mapstructure:"qwen_wanxiang"` // Deprecated, use Wanxiang26
}

// OSSConfig holds Aliyun OSS configuration
type OSSConfig struct {
	AccessKeyID     string `mapstructure:"access_key_id"`
	AccessKeySecret string `mapstructure:"access_key_secret"`
	Bucket          string `mapstructure:"bucket"`
	Endpoint        string `mapstructure:"endpoint"`
	Domain          string `mapstructure:"domain"`
	PathPrefix      string `mapstructure:"path_prefix"`
}

// SkillsConfig holds skills configuration
type SkillsConfig struct {
	Path        string `mapstructure:"path"`
	AutoLoad    bool   `mapstructure:"auto_load"`
	RegistryURL string `mapstructure:"registry_url"` // Optional: URL for remote skill discovery (e.g. ClawHub)
}

// AdapterConfig holds IM adapter configuration
type AdapterConfig struct {
	Type    string                 `mapstructure:"type"`
	Enabled bool                   `mapstructure:"enabled"`
	Config  map[string]interface{} `mapstructure:"config"`
}

// LogConfig holds logging configuration
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"` // json, console
	Output string `mapstructure:"output"` // stdout, file
}

// Load loads configuration from file
func Load(configPath string) (*Config, error) {
	// Load .env file if exists (search in current directory and parent directories)
	_ = godotenv.Load()

	v := viper.New()

	// Set config file
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	// Environment variable support
	v.SetEnvPrefix("MARSTAFF")
	v.AutomaticEnv()

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, err
	}

	// Expand environment variables in provider configs
	expandEnvInProviderConfig(config.Provider.Qwen)
	expandEnvInProviderConfig(config.Provider.ZAI)
	expandEnvInProviderConfig(config.Provider.OpenAI)
	expandEnvInProviderConfig(config.Provider.Zhipu)
	expandEnvInProviderConfig(config.Provider.Gemini)
	expandEnvInProviderConfig(config.Provider.DeepSeek)
	expandEnvInProviderConfig(config.Provider.MiniMax)
	expandEnvInProviderConfig(config.Provider.MiniMaxIntl)
	expandEnvInProviderConfig(config.Provider.Ollama)
	expandEnvInProviderConfig(config.Provider.VLLM)
	expandEnvInProviderConfig(config.Provider.Poe)

	// Expand environment variables in media configs
	expandEnvInProviderConfig(config.Media.Wanxiang26)
	expandEnvInProviderConfig(config.Media.QWenWanxiang) // Backward compatibility

	// Expand environment variables in OSS config
	config.OSS.AccessKeyID = expandEnv(config.OSS.AccessKeyID)
	config.OSS.AccessKeySecret = expandEnv(config.OSS.AccessKeySecret)

	// Set defaults
	setDefaults(&config)

	return &config, nil
}

// expandEnvInProviderConfig expands ${VAR} environment variables in provider config
func expandEnvInProviderConfig(cfg map[string]interface{}) {
	for key, value := range cfg {
		if str, ok := value.(string); ok {
			cfg[key] = expandEnv(str)
		}
	}
}

// expandEnv expands ${VAR} environment variables in a string
func expandEnv(s string) string {
	return os.Expand(s, func(key string) string {
		// Try environment variable first
		if val := os.Getenv(key); val != "" {
			return val
		}
		// Return original if not found
		return "${" + key + "}"
	})
}

// setDefaults sets default values for configuration
func setDefaults(c *Config) {
	if c.Server.Host == "" {
		c.Server.Host = "0.0.0.0"
	}
	if c.Server.Port == 0 {
		c.Server.Port = 18789
	}
	if c.Server.Mode == "" {
		c.Server.Mode = "debug"
	}
	if c.Database.Driver == "" {
		c.Database.Driver = "mysql"
	}
	if c.Database.MaxOpenConns == 0 {
		c.Database.MaxOpenConns = 25
	}
	if c.Database.MaxIdleConns == 0 {
		c.Database.MaxIdleConns = 25
	}
	if c.Provider.Default == "" {
		c.Provider.Default = "zai"
	}
	if c.Workspace.BasePath == "" {
		c.Workspace.BasePath = "./workspace/projects"
	}
	if c.Skills.Path == "" {
		c.Skills.Path = "./skills"
	}
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.Log.Format == "" {
		c.Log.Format = "console"
	}
	if c.Log.Output == "" {
		c.Log.Output = "stdout"
	}
	if c.Security.Sandbox.Mode == "" {
		c.Security.Sandbox.Mode = "off"
	}
	if c.Security.Sandbox.Mode == "non_main" && c.Security.Sandbox.Image == "" {
		c.Security.Sandbox.Image = "alpine:latest"
	}
}
