package config

import (
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

type Config struct {
	App       AppConfig       `mapstructure:"app"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Server    ServerConfig    `mapstructure:"server"`
	WhatsApp  WhatsAppConfig  `mapstructure:"whatsapp"`
	Translation TranslationConfig `mapstructure:"translation"`
	Risk      RiskConfig      `mapstructure:"risk"`
	Broadcast BroadcastConfig `mapstructure:"broadcast"`
	Log       LogConfig       `mapstructure:"log"`
}

type AppConfig struct {
	Name        string `mapstructure:"name"`
	Version     string `mapstructure:"version"`
	DataDir     string `mapstructure:"data_dir"`
	Debug       bool   `mapstructure:"debug"`
	AutoOpenBrowser bool `mapstructure:"auto_open_browser"`
}

type DatabaseConfig struct {
	MainDBPath     string `mapstructure:"main_db_path"`
	SessionsDir    string `mapstructure:"sessions_dir"`
	EnableWAL      bool   `mapstructure:"enable_wal"`
	MaxOpenConns   int    `mapstructure:"max_open_conns"`
	MaxIdleConns   int    `mapstructure:"max_idle_conns"`
}

type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"` // debug, release
	StaticDir string `mapstructure:"static_dir"`
}

type WhatsAppConfig struct {
	MaxAccounts       int    `mapstructure:"max_accounts"`
	ReconnectInterval int    `mapstructure:"reconnect_interval"` // seconds
	QRRefreshInterval int    `mapstructure:"qr_refresh_interval"` // seconds
	MessageRateLimit  int    `mapstructure:"message_rate_limit"` // per minute per account
	DefaultProxy      string `mapstructure:"default_proxy"`
	AutoReconnect     bool   `mapstructure:"auto_reconnect"`
	SyncContacts      bool   `mapstructure:"sync_contacts"`
}

type TranslationConfig struct {
	Enabled       bool   `mapstructure:"enabled"`
	Provider      string `mapstructure:"provider"` // openai_compatible
	BaseURL       string `mapstructure:"base_url"`
	APIKey        string `mapstructure:"api_key"`
	Model         string `mapstructure:"model"`
	PromptTemplate string `mapstructure:"prompt_template"`
	TargetLang    string `mapstructure:"target_lang"`
	Timeout       int    `mapstructure:"timeout"` // seconds
}

type RiskConfig struct {
	Enabled       bool   `mapstructure:"enabled"`
	L1Enabled     bool   `mapstructure:"l1_enabled"`
	L2Enabled     bool   `mapstructure:"l2_enabled"`
	L3Enabled     bool   `mapstructure:"l3_enabled"`
	WordsDir      string `mapstructure:"words_dir"`
	AutoAction    bool   `mapstructure:"auto_action"` // 自动执行动作
	DefaultAction string `mapstructure:"default_action"` // delete, warn, kick
	L3WindowSize  int    `mapstructure:"l3_window_size"` // seconds
	L3MaxMsgPerMin int   `mapstructure:"l3_max_msg_per_min"`
	L3MaxMentionPerMin int `mapstructure:"l3_max_mention_per_min"`
}

type BroadcastConfig struct {
	Enabled       bool `mapstructure:"enabled"`
	DefaultInterval int `mapstructure:"default_interval"` // ms between groups
	MaxConcurrent  int `mapstructure:"max_concurrent"` // concurrent sends per account
	RetryCount     int `mapstructure:"retry_count"`
	RetryInterval  int `mapstructure:"retry_interval"` // seconds
}

type LogConfig struct {
	Level      string `mapstructure:"level"`
	Dir        string `mapstructure:"dir"`
	MaxSize    int    `mapstructure:"max_size"` // MB
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"` // days
	Compress   bool   `mapstructure:"compress"`
}

var Cfg *Config

func Load() error {
	// 1. 确定数据目录
	exeDir, _ := os.Executable()
	baseDir := filepath.Dir(exeDir)
	dataDir := filepath.Join(baseDir, "data")

	// 2. 设置默认值
	setDefaults(dataDir)

	// 3. 读取 .env (如果存在)
	envPath := filepath.Join(baseDir, ".env")
	if _, err := os.Stat(envPath); err == nil {
		godotenv.Load(envPath)
	}

	// 4. 读取 config.yaml
	configPath := filepath.Join(baseDir, "config.yaml")
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")
	viper.AutomaticEnv()
	viper.SetEnvPrefix("WAOPS")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return err
		}
		// 配置文件不存在，使用默认值并生成示例
		if err := saveExample(baseDir); err != nil {
			return err
		}
	}

	Cfg = &Config{}
	if err := viper.Unmarshal(Cfg); err != nil {
		return err
	}

	// 5. 确保目录存在
	ensureDirs()

	// 6. 从环境变量覆盖敏感配置
	overrideFromEnv()

	return nil
}

func setDefaults(dataDir string) {
	viper.SetDefault("app.name", "WhatsApp Ops")
	viper.SetDefault("app.version", "1.0.0")
	viper.SetDefault("app.data_dir", dataDir)
	viper.SetDefault("app.debug", false)
	viper.SetDefault("app.auto_open_browser", true)

	viper.SetDefault("database.main_db_path", filepath.Join(dataDir, "main.db"))
	viper.SetDefault("database.sessions_dir", filepath.Join(dataDir, "sessions"))
	viper.SetDefault("database.enable_wal", true)
	viper.SetDefault("database.max_open_conns", 10)
	viper.SetDefault("database.max_idle_conns", 5)

	viper.SetDefault("server.host", "127.0.0.1")
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.mode", "release")
	viper.SetDefault("server.static_dir", "./web")

	viper.SetDefault("whatsapp.max_accounts", 10)
	viper.SetDefault("whatsapp.reconnect_interval", 30)
	viper.SetDefault("whatsapp.qr_refresh_interval", 20)
	viper.SetDefault("whatsapp.message_rate_limit", 30)
	viper.SetDefault("whatsapp.default_proxy", "")
	viper.SetDefault("whatsapp.auto_reconnect", true)
	viper.SetDefault("whatsapp.sync_contacts", true)

	viper.SetDefault("translation.enabled", true)
	viper.SetDefault("translation.provider", "openai_compatible")
	viper.SetDefault("translation.base_url", "https://api.openai.com/v1")
	viper.SetDefault("translation.api_key", "")
	viper.SetDefault("translation.model", "gpt-4o-mini")
	viper.SetDefault("translation.prompt_template", defaultPromptTemplate())
	viper.SetDefault("translation.target_lang", "zh-CN")
	viper.SetDefault("translation.timeout", 30)

	viper.SetDefault("risk.enabled", true)
	viper.SetDefault("risk.l1_enabled", true)
	viper.SetDefault("risk.l2_enabled", true)
	viper.SetDefault("risk.l3_enabled", true)
	viper.SetDefault("risk.words_dir", filepath.Join(dataDir, "risk_words"))
	viper.SetDefault("risk.auto_action", false)
	viper.SetDefault("risk.default_action", "warn")
	viper.SetDefault("risk.l3_window_size", 60)
	viper.SetDefault("risk.l3_max_msg_per_min", 20)
	viper.SetDefault("risk.l3_max_mention_per_min", 5)

	viper.SetDefault("broadcast.enabled", true)
	viper.SetDefault("broadcast.default_interval", 2000)
	viper.SetDefault("broadcast.max_concurrent", 3)
	viper.SetDefault("broadcast.retry_count", 2)
	viper.SetDefault("broadcast.retry_interval", 5)

	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.dir", filepath.Join(dataDir, "logs"))
	viper.SetDefault("log.max_size", 100)
	viper.SetDefault("log.max_backups", 7)
	viper.SetDefault("log.max_age", 30)
	viper.SetDefault("log.compress", true)
}

func defaultPromptTemplate() string {
	return `你是专业的跨境电商翻译助手。
将以下内容翻译为 {{.TargetLang}}，保持商务礼貌语气，
专业术语保留英文，表情符号保留原样。
只返回翻译结果，不要解释。
原文: {{.Text}}`
}

func ensureDirs() {
	dirs := []string{
		Cfg.App.DataDir,
		Cfg.Database.SessionsDir,
		Cfg.Risk.WordsDir,
		Cfg.Log.Dir,
	}
	for _, d := range dirs {
		os.MkdirAll(d, 0755)
	}
}

func overrideFromEnv() {
	if v := os.Getenv("WAOPS_TRANSLATION_API_KEY"); v != "" {
		Cfg.Translation.APIKey = v
	}
	if v := os.Getenv("WAOPS_WHATSAPP_DEFAULT_PROXY"); v != "" {
		Cfg.WhatsApp.DefaultProxy = v
	}
}

func saveExample(baseDir string) error {
	example := `# WhatsApp Ops 配置文件示例
# 复制为 config.yaml 并修改

app:
  name: "WhatsApp Ops"
  version: "1.0.0"
  data_dir: "./data"
  debug: false
  auto_open_browser: true

database:
  main_db_path: "./data/main.db"
  sessions_dir: "./data/sessions"
  enable_wal: true
  max_open_conns: 10
  max_idle_conns: 5

server:
  host: "127.0.0.1"
  port: 8080
  mode: "release"
  static_dir: "./web"

whatsapp:
  max_accounts: 10
  reconnect_interval: 30
  qr_refresh_interval: 20
  message_rate_limit: 30
  default_proxy: ""
  auto_reconnect: true
  sync_contacts: true

translation:
  enabled: true
  provider: "openai_compatible"
  base_url: "https://api.openai.com/v1"
  api_key: ""  # 建议通过环境变量 WAOPS_TRANSLATION_API_KEY 设置
  model: "gpt-4o-mini"
  prompt_template: |
    你是专业的跨境电商翻译助手。
    将以下内容翻译为 {{.TargetLang}}，保持商务礼貌语气，
    专业术语保留英文，表情符号保留原样。
    只返回翻译结果，不要解释。
    原文: {{.Text}}
  target_lang: "zh-CN"
  timeout: 30

risk:
  enabled: true
  l1_enabled: true
  l2_enabled: true
  l3_enabled: true
  words_dir: "./data/risk_words"
  auto_action: false
  default_action: "warn"
  l3_window_size: 60
  l3_max_msg_per_min: 20
  l3_max_mention_per_min: 5

broadcast:
  enabled: true
  default_interval: 2000
  max_concurrent: 3
  retry_count: 2
  retry_interval: 5

log:
  level: "info"
  dir: "./data/logs"
  max_size: 100
  max_backups: 7
  max_age: 30
  compress: true
`
	path := filepath.Join(baseDir, "config.yaml.example")
	return os.WriteFile(path, []byte(example), 0644)
}

func Get() *Config {
	return Cfg
}

// Save 保存配置到文件
func Save(cfg *Config) error {
	cfgPath := viper.ConfigFileUsed()
	if cfgPath == "" {
		// 如果没有找到配置文件，使用默认路径
		cfgPath = "./config.yaml"
	}

	// 将配置写入 viper
	viper.Set("app", cfg.App)
	viper.Set("database", cfg.Database)
	viper.Set("server", cfg.Server)
	viper.Set("whatsapp", cfg.WhatsApp)
	viper.Set("translation", cfg.Translation)
	viper.Set("risk", cfg.Risk)
	viper.Set("broadcast", cfg.Broadcast)
	viper.Set("log", cfg.Log)

	return viper.WriteConfigAs(cfgPath)
}