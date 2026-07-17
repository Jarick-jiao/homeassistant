package config

import (
	"fmt"
	"log"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 应用配置结构体
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	JWT      JWTConfig      `yaml:"jwt"`
	Wechat   WechatConfig   `yaml:"wechat"`
	MCP      MCPConfig      `yaml:"mcp"`
	OpenAI   OpenAIConfig   `yaml:"openai"`
	Family   FamilyConfig   `yaml:"family"`
}

// ServerConfig HTTP 服务配置
type ServerConfig struct {
	Port    string `yaml:"port"`
	Mode    string `yaml:"mode"` // debug / release / test
	LogLvl  string `yaml:"log_level"`
	Timeout struct {
		Read  time.Duration `yaml:"read"`
		Write time.Duration `yaml:"write"`
		Idle  time.Duration `yaml:"idle"`
	} `yaml:"timeout"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Path     string `yaml:"path"`
	WALMode  bool   `yaml:"wal_mode"`
	MaxOpen  int    `yaml:"max_open_conns"`
	MaxIdle  int    `yaml:"max_idle_conns"`
	MaxLife  time.Duration `yaml:"conn_max_lifetime"`
}

// JWTConfig JWT 认证配置
type JWTConfig struct {
	Secret     string        `yaml:"secret"`
	ExpireIn   time.Duration `yaml:"expire_in"`
	Issuer     string        `yaml:"issuer"`
}

// WechatConfig 微信配置
type WechatConfig struct {
	AppID     string `yaml:"app_id"`
	AppSecret string `yaml:"app_secret"`
	Token     string `yaml:"token"`
}

// MCPConfig MCP 服务器配置
type MCPConfig struct {
	Timeout      time.Duration    `yaml:"timeout"`
	Servers      []MCPServerConfig `yaml:"servers"`
}

// MCPServerConfig 单个 MCP 服务器配置
type MCPServerConfig struct {
	Name      string   `yaml:"name"`
	Transport string   `yaml:"transport"` // stdio / sse
	Command   string   `yaml:"command"`
	Args      []string `yaml:"args"`
	Env       []string `yaml:"env"`
	SSEURL    string   `yaml:"sse_url"`
	Enabled   bool     `yaml:"enabled"`
}

// OpenAIConfig 大模型配置
type OpenAIConfig struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
	Model   string `yaml:"model"`
}

// FamilyConfig 家庭配置
type FamilyConfig struct {
	Name string `yaml:"name"`
}

// setDefaults 设置默认值
func (c *Config) setDefaults() {
	if c.Server.Port == "" {
		c.Server.Port = "8080"
	}
	if c.Server.Mode == "" {
		c.Server.Mode = "release"
	}
	if c.Server.LogLvl == "" {
		c.Server.LogLvl = "info"
	}
	if c.Server.Timeout.Read == 0 {
		c.Server.Timeout.Read = 10 * time.Second
	}
	if c.Server.Timeout.Write == 0 {
		c.Server.Timeout.Write = 30 * time.Second
	}
	if c.Server.Timeout.Idle == 0 {
		c.Server.Timeout.Idle = 120 * time.Second
	}
	if c.Database.Path == "" {
		c.Database.Path = "./homemate.db"
	}
	c.Database.WALMode = true
	if c.Database.MaxOpen == 0 {
		c.Database.MaxOpen = 10
	}
	if c.Database.MaxIdle == 0 {
		c.Database.MaxIdle = 5
	}
	if c.Database.MaxLife == 0 {
		c.Database.MaxLife = 30 * time.Minute
	}
	if c.JWT.Secret == "" {
		c.JWT.Secret = "homemate-default-secret-change-me"
		log.Println("[WARN] JWT Secret 使用默认值，请通过配置文件或环境变量 HOMEMATE_JWT_SECRET 设置")
	}
	if c.JWT.ExpireIn == 0 {
		c.JWT.ExpireIn = 24 * time.Hour
	}
	if c.MCP.Timeout == 0 {
		c.MCP.Timeout = 30 * time.Second
	}
	if c.OpenAI.Model == "" {
		c.OpenAI.Model = "gpt-4o"
	}
	if c.Family.Name == "" {
		c.Family.Name = "My Family"
	}
}

// applyEnv 用环境变量覆盖配置
func (c *Config) applyEnv() {
	if v := os.Getenv("HOMEMATE_SERVER_PORT"); v != "" {
		c.Server.Port = v
	}
	if v := os.Getenv("HOMEMATE_SERVER_MODE"); v != "" {
		c.Server.Mode = v
	}
	if v := os.Getenv("HOMEMATE_DATABASE_PATH"); v != "" {
		c.Database.Path = v
	}
	if v := os.Getenv("HOMEMATE_JWT_SECRET"); v != "" {
		c.JWT.Secret = v
	}
	if v := os.Getenv("HOMEMATE_OPENAI_KEY"); v != "" {
		c.OpenAI.APIKey = v
	}
	if v := os.Getenv("HOMEMATE_OPENAI_BASE_URL"); v != "" {
		c.OpenAI.BaseURL = v
	}
	if v := os.Getenv("HOMEMATE_OPENAI_MODEL"); v != "" {
		c.OpenAI.Model = v
	}
	if v := os.Getenv("HOMEMATE_WECHAT_APP_ID"); v != "" {
		c.Wechat.AppID = v
	}
	if v := os.Getenv("HOMEMATE_WECHAT_APP_SECRET"); v != "" {
		c.Wechat.AppSecret = v
	}
}

// Validate 校验必要配置项
func (c *Config) Validate() error {
	if c.JWT.Secret == "homemate-default-secret-change-me" {
		log.Println("[WARN] 生产环境请务必修改 JWT Secret")
	}
	if c.Server.Port == "" {
		return fmt.Errorf("server.port 不能为空")
	}
	return nil
}

// Load 从配置文件和环境变量加载配置
func Load() *Config {
	cfg := &Config{}
	cfg.setDefaults()

	// 尝试从 config.yaml 读取
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		log.Printf("[INFO] 未找到 config.yaml，使用环境变量和默认值: %v", err)
	} else {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			log.Fatalf("[FATAL] 配置文件解析失败: %v", err)
		}
	}

	// 环境变量覆盖
	cfg.applyEnv()
	cfg.setDefaults() // 重新应用默认值（防止 YAML 部分字段覆盖为空）

	if err := cfg.Validate(); err != nil {
		log.Fatalf("[FATAL] 配置校验失败: %v", err)
	}

	return cfg
}