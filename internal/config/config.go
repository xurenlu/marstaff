package config

import (
	"github.com/spf13/viper"
)

// Config is the main configuration structure
type Config struct {
	Server   ServerConfig      `mapstructure:"server"`
	Database DatabaseConfig    `mapstructure:"database"`
	Provider ProviderConfig    `mapstructure:"provider"`
	Skills   SkillsConfig      `mapstructure:"skills"`
	Adapters []AdapterConfig   `mapstructure:"adapters"`
	Log      LogConfig         `mapstructure:"log"`
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
	OpenAI   map[string]interface{} `mapstructure:"openai"`
	Fallback []string               `mapstructure:"fallback"`
}

// SkillsConfig holds skills configuration
type SkillsConfig struct {
	Path    string `mapstructure:"path"`
	AutoLoad bool   `mapstructure:"auto_load"`
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

	// Set defaults
	setDefaults(&config)

	return &config, nil
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
}
