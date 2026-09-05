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
	Amap     AmapConfig     `yaml:"amap"`
	Garmin   GarminConfig   `yaml:"garmin"`
	WeCom    WeComConfig    `yaml:"wecom"`
	// v3.9.12: Apple 日历定时同步（osascript 读取 macOS Calendar.app → calendar_events 表）
	CalendarSync CalendarSyncConfig `yaml:"calendar_sync"`
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
	Path    string        `yaml:"path"`
	WALMode bool          `yaml:"wal_mode"`
	MaxOpen int           `yaml:"max_open_conns"`
	MaxIdle int           `yaml:"max_idle_conns"`
	MaxLife time.Duration `yaml:"conn_max_lifetime"`
}

// JWTConfig JWT 认证配置
type JWTConfig struct {
	Secret   string        `yaml:"secret"`
	ExpireIn time.Duration `yaml:"expire_in"`
	Issuer   string        `yaml:"issuer"`
}

// WechatConfig 微信配置
type WechatConfig struct {
	AppID     string `yaml:"app_id"`
	AppSecret string `yaml:"app_secret"`
	Token     string `yaml:"token"`
}

// MCPConfig MCP 服务器配置
type MCPConfig struct {
	Timeout time.Duration     `yaml:"timeout"`
	Servers []MCPServerConfig `yaml:"servers"`
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

// AmapConfig 高德地图配置（天气/地理）
type AmapConfig struct {
	APIKey  string `yaml:"api_key"`
	City    string `yaml:"city"` // adcode 区域编码，默认 110100（北京）
	BaseURL string `yaml:"base_url"`
}

// GarminConfig Garmin Connect 配置
type GarminConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	TokenDir string `yaml:"token_dir"` // token 持久化目录
	BaseURL  string `yaml:"base_url"`
	// v3.9.8: Python 脚本同步模式（绕过 Cloudflare，使用 garminconnect/garth）
	// 启用后调度器通过 exec 调用 homemate_health_sync.py，脚本直接写 SQLite，
	// 不再走 Go 版 garminClient.Login/GetDailyHealth（被 Cloudflare 拦截 429）。
	// 中国区开关由脚本读 GARMIN_IS_CN 环境变量控制（默认中国区 true）。
	UseScriptSync  bool          `yaml:"use_script_sync"`  // 启用脚本同步（推荐 true）
	SyncScriptPath string        `yaml:"sync_script_path"` // homemate_health_sync.py 路径
	PythonPath     string        `yaml:"python_path"`      // python3 可执行文件路径
	ScriptTimeout  time.Duration `yaml:"script_timeout"`   // 脚本执行超时
}

// WeComConfig 企业微信群机器人配置
type WeComConfig struct {
	WebhookURL string `yaml:"webhook_url"`
	EnablePush bool   `yaml:"enable_push"`
}

// CalendarSyncConfig Apple 日历同步配置
// 调度器通过 exec 调用 homemate_calendar_sync.py，脚本用 osascript 读取
// macOS Calendar.app 中 ±30 天事件，写入 calendar_events 表（source='apple_calendar'）。
// 事件按 (source_account, external_event_id) 唯一索引去重，循环事件展开为独立行。
// 仅 macOS 可用（需 TCC 自动化授权），非 macOS 环境应置 enable: false。
type CalendarSyncConfig struct {
	Enable        bool          `yaml:"enable"`         // 启用 Apple 日历定时同步
	ScriptPath    string        `yaml:"script_path"`    // homemate_calendar_sync.py 路径
	PythonPath    string        `yaml:"python_path"`    // python3 可执行文件路径
	ScriptTimeout time.Duration `yaml:"script_timeout"` // 脚本执行超时
	Interval      time.Duration `yaml:"interval"`       // 同步间隔（默认 1 小时）
	// LookbackDays 同步回看天数（脚本读取 [now-lookback, now+lookahead] 范围事件）
	LookbackDays  int `yaml:"lookback_days"`
	LookaheadDays int `yaml:"lookahead_days"`
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
	if c.Amap.BaseURL == "" {
		c.Amap.BaseURL = "https://restapi.amap.com/v3"
	}
	if c.Amap.City == "" {
		c.Amap.City = "110100" // 北京
	}
	if c.Garmin.BaseURL == "" {
		c.Garmin.BaseURL = "https://connect.garmin.com"
	}
	if c.Garmin.TokenDir == "" {
		c.Garmin.TokenDir = "./data"
	}
	// v3.9.8: 脚本同步默认值
	if c.Garmin.PythonPath == "" {
		c.Garmin.PythonPath = "/usr/bin/python3"
	}
	if c.Garmin.SyncScriptPath == "" {
		c.Garmin.SyncScriptPath = "./scripts/homemate_health_sync.py"
	}
	if c.Garmin.ScriptTimeout == 0 {
		c.Garmin.ScriptTimeout = 3 * time.Minute
	}
	// v3.9.12: Apple 日历同步默认值
	if c.CalendarSync.PythonPath == "" {
		c.CalendarSync.PythonPath = "/usr/bin/python3"
	}
	if c.CalendarSync.ScriptPath == "" {
		c.CalendarSync.ScriptPath = "./scripts/homemate_calendar_sync.py"
	}
	if c.CalendarSync.ScriptTimeout == 0 {
		c.CalendarSync.ScriptTimeout = 2 * time.Minute
	}
	if c.CalendarSync.Interval == 0 {
		c.CalendarSync.Interval = 1 * time.Hour
	}
	if c.CalendarSync.LookbackDays == 0 {
		c.CalendarSync.LookbackDays = 30
	}
	if c.CalendarSync.LookaheadDays == 0 {
		c.CalendarSync.LookaheadDays = 30
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
	if v := os.Getenv("HOMEMATE_AMAP_API_KEY"); v != "" {
		c.Amap.APIKey = v
	}
	if v := os.Getenv("HOMEMATE_GARMIN_USERNAME"); v != "" {
		c.Garmin.Username = v
	}
	if v := os.Getenv("HOMEMATE_GARMIN_PASSWORD"); v != "" {
		c.Garmin.Password = v
	}
	// v3.9.7: 兼容无前缀的环境变量名（用户常用 GARMIN_USERNAME 而非 HOMEMATE_GARMIN_USERNAME）
	if c.Garmin.Username == "" {
		if v := os.Getenv("GARMIN_USERNAME"); v != "" {
			c.Garmin.Username = v
		}
	}
	if c.Garmin.Password == "" {
		if v := os.Getenv("GARMIN_PASSWORD"); v != "" {
			c.Garmin.Password = v
		}
	}
	if v := os.Getenv("HOMEMATE_WECOM_WEBHOOK"); v != "" {
		c.WeCom.WebhookURL = v
	}
	if v := os.Getenv("HOMEMATE_WECOM_ENABLE_PUSH"); v == "true" || v == "1" {
		c.WeCom.EnablePush = true
	}
	// v3.9.12: Apple 日历同步环境变量覆盖
	if v := os.Getenv("HOMEMATE_CALENDAR_SYNC_ENABLE"); v == "true" || v == "1" {
		c.CalendarSync.Enable = true
	} else if v == "false" || v == "0" {
		c.CalendarSync.Enable = false
	}
	if v := os.Getenv("HOMEMATE_CALENDAR_SYNC_SCRIPT"); v != "" {
		c.CalendarSync.ScriptPath = v
	}
	if v := os.Getenv("HOMEMATE_CALENDAR_SYNC_PYTHON"); v != "" {
		c.CalendarSync.PythonPath = v
	}
	if v := os.Getenv("HOMEMATE_CALENDAR_SYNC_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.CalendarSync.Interval = d
		}
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

// Load 从默认路径（config.yaml，可用 HOMEMATE_CONFIG 环境变量覆盖）加载配置
func Load() *Config {
	path := os.Getenv("HOMEMATE_CONFIG")
	if path == "" {
		path = "config.yaml"
	}
	return LoadFrom(path)
}

// LoadFrom 从指定配置文件和环境变量加载配置（v4.0: -config 参数生效）
func LoadFrom(path string) *Config {
	cfg := &Config{}
	cfg.setDefaults()

	// 尝试从指定配置文件读取
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[INFO] 未找到 %s，使用环境变量和默认值: %v", path, err)
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
