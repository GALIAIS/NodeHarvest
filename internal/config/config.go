package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App       AppConfig       `yaml:"app"`
	Sources   []Source        `yaml:"sources"`
	Protocols []string        `yaml:"protocols"`
	Filter    FilterConfig    `yaml:"filter"`
	Export    ExportConfig    `yaml:"export"`
	Schedule  ScheduleConfig  `yaml:"schedule"`
	Publish   PublishConfig   `yaml:"publish"`
	Geo       GeoConfig       `yaml:"geo"`
	Dial      DialConfig      `yaml:"dial"`
	Security  SecurityConfig  `yaml:"security"`
	Server    ServerConfig    `yaml:"server"`
	SQLite    SQLiteConfig    `yaml:"sqlite"`
	Logging   LoggingConfig   `yaml:"logging"`
}

// DialConfig 真实协议拨测（sing-box / xray）
type DialConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Bin         string `yaml:"bin"`          // 空=自动查找
	Engine      string `yaml:"engine"`       // auto|sing-box|xray
	Concurrency int    `yaml:"concurrency"`  // 同时核心实例数
	TimeoutSec  int    `yaml:"timeout_sec"`
	TestURL     string `yaml:"test_url"`
	MaxNodes    int    `yaml:"max_nodes"`    // 单次最多拨测
	// AfterQuality 在 quality/full 后自动对 TopN 做真测
	AfterQuality bool `yaml:"after_quality"`
	// AfterQualityMax 自动真测数量
	AfterQualityMax int `yaml:"after_quality_max"`
}

// SecurityConfig 认证与限流
type SecurityConfig struct {
	AdminToken      string  `yaml:"admin_token"`       // 管理 API；空则回退 publish.token
	AllowQueryToken bool    `yaml:"allow_query_token"` // 允许 ?token=
	SubRPS          float64 `yaml:"sub_rps"`           // 订阅限流每 IP
	SubBurst        int     `yaml:"sub_burst"`
	APIRPS          float64 `yaml:"api_rps"`
	APIBurst        int     `yaml:"api_burst"`
}

// ServerConfig 运行时
type ServerConfig struct {
	ReadHeaderTimeoutSec int  `yaml:"read_header_timeout_sec"`
	ShutdownTimeoutSec   int  `yaml:"shutdown_timeout_sec"`
	EnableMetrics        bool `yaml:"enable_metrics"`
	EnablePprof          bool `yaml:"enable_pprof"`
}

// SQLiteConfig 企业持久化
type SQLiteConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
}

// GeoConfig IP 国家标注
type GeoConfig struct {
	Enabled     bool   `yaml:"enabled"`
	DBPath      string `yaml:"db_path"`       // 默认 data/GeoLite2-Country.mmdb
	DownloadURL string `yaml:"download_url"`  // 空则用内置镜像
	AutoDownload bool  `yaml:"auto_download"` // 缺库时自动下载
	// AnnotateAfterQuality 测速后自动标注
	AnnotateAfterQuality bool `yaml:"annotate_after_quality"`
	// RenameWithFlag 导出时名称加国旗前缀
	RenameWithFlag bool `yaml:"rename_with_flag"`
}

type AppConfig struct {
	Concurrency     int    `yaml:"concurrency"`
	FetchTimeoutSec int    `yaml:"fetch_timeout_sec"`
	TestTimeoutSec  int    `yaml:"test_timeout_sec"`
	UserAgent       string `yaml:"user_agent"`
}

type Source struct {
	Name    string `yaml:"name"`
	Type    string `yaml:"type"` // subscription | raw | clash
	URL     string `yaml:"url"`
	Enabled bool   `yaml:"enabled"`
}

type FilterConfig struct {
	MaxLatencyMS      int     `yaml:"max_latency_ms"`
	MinSuccess        bool    `yaml:"min_success"`
	DropInvalid       bool    `yaml:"drop_invalid"`
	PreferTLS         bool    `yaml:"prefer_tls"`
	MaxNodes          int     `yaml:"max_nodes"`
	SortBy            string  `yaml:"sort_by"`
	MinScore          float64 `yaml:"min_score"`           // 高质量阈值，默认 70
	PruneAfterQuality bool    `yaml:"prune_after_quality"` // 测速后剔除死亡节点
	MaxStoreNodes     int     `yaml:"max_store_nodes"`     // 快照最多保留节点数
}

type ExportConfig struct {
	Dir            string   `yaml:"dir"`
	Formats        []string `yaml:"formats"`
	FilenamePrefix string   `yaml:"filename_prefix"`
}

// ScheduleConfig 定时任务：采集 / 测速 / 全流程
type ScheduleConfig struct {
	Enabled        bool   `yaml:"enabled"`
	IntervalMin    int    `yaml:"interval_min"`     // 间隔分钟，默认 180
	Job            string `yaml:"job"`              // full | fetch | quality
	RunOnStart     bool   `yaml:"run_on_start"`     // 启动后是否立即跑一轮
	SkipAI         bool   `yaml:"skip_ai"`          // full 时跳过 AI 探测（更快）
	MaxTest        int    `yaml:"max_test"`         // 测速上限
	Rounds         int    `yaml:"rounds"`           // 测速轮数
	JitterSec      int    `yaml:"jitter_sec"`       // 启动抖动，避免整点齐打
}

// PublishConfig 对外订阅发布（VPS 远程拉取）
type PublishConfig struct {
	Enabled      bool     `yaml:"enabled"`
	Token        string   `yaml:"token"`       // 空=公开；非空则 ?token= 或 Bearer 必填
	PathPrefix   string   `yaml:"path_prefix"` // 默认 /sub
	MinScore     float64  `yaml:"min_score"`   // 默认用 filter.min_score
	MaxNodes     int      `yaml:"max_nodes"`   // 默认用 filter.max_nodes
	AliveOnly    bool     `yaml:"alive_only"`
	Formats      []string `yaml:"formats"`   // raw, base64, clash
	CacheSec     int      `yaml:"cache_sec"` // Cache-Control max-age
	PublicURL    string   `yaml:"public_url"`
	PreRender    bool     `yaml:"pre_render"`     // 任务后预渲染订阅
	MaxCountries int      `yaml:"max_countries"`  // 分国家缓存上限
}

type LoggingConfig struct {
	Level string `yaml:"level"`
}

// Default 返回合理默认配置
func Default() *Config {
	return &Config{
		App: AppConfig{
			Concurrency:     64,
			FetchTimeoutSec: 20,
			TestTimeoutSec:  6,
			UserAgent:       "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/122.0.0.0 Safari/537.36",
		},
		Protocols: []string{"vmess", "vless", "trojan", "ss", "ssr", "hysteria2", "hy2", "tuic"},
		Filter: FilterConfig{
			MaxLatencyMS:      2500,
			MinSuccess:        true,
			DropInvalid:       true,
			PreferTLS:         false,
			MaxNodes:          500,
			SortBy:            "score",
			MinScore:          70,
			PruneAfterQuality: true,
			MaxStoreNodes:     8000,
		},
		Export: ExportConfig{
			Dir:            "output",
			Formats:        []string{"raw", "base64", "clash", "json"},
			FilenamePrefix: "nodes",
		},
		Schedule: ScheduleConfig{
			Enabled:     false,
			IntervalMin: 180,
			Job:         "full",
			RunOnStart:  false,
			SkipAI:      true,
			MaxTest:     1200,
			Rounds:      2,
			JitterSec:   30,
		},
		Publish: PublishConfig{
			Enabled:      true,
			Token:        "",
			PathPrefix:   "/sub",
			AliveOnly:    true,
			Formats:      []string{"raw", "base64", "clash"},
			CacheSec:     120,
			PreRender:    true,
			MaxCountries: 30,
		},
		Geo: GeoConfig{
			Enabled:              true,
			DBPath:               "data/GeoLite2-Country.mmdb",
			AutoDownload:         true,
			AnnotateAfterQuality: true,
			RenameWithFlag:       true,
		},
		Dial: DialConfig{
			Enabled:         true,
			Engine:          "sing-box",
			Concurrency:     4,
			TimeoutSec:      18,
			TestURL:         "https://www.cloudflare.com/cdn-cgi/trace",
			MaxNodes:        80,
			AfterQuality:    false, // 手动或定时单独开；避免拖慢 3h full
			AfterQualityMax: 40,
		},
		Security: SecurityConfig{
			AllowQueryToken: true,
			SubRPS:          20,
			SubBurst:        40,
			APIRPS:          30,
			APIBurst:        60,
		},
		Server: ServerConfig{
			ReadHeaderTimeoutSec: 10,
			ShutdownTimeoutSec:   30,
			EnableMetrics:        true,
		},
		SQLite: SQLiteConfig{
			Enabled: true,
			Path:    "data/node-hunter.db",
		},
		Logging: LoggingConfig{Level: "info"},
	}
}

// Load 从 YAML 加载配置，文件不存在时返回默认配置
func Load(path string) (*Config, error) {
	cfg := Default()
	if path == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.normalize()
	return cfg, nil
}

func (c *Config) normalize() {
	if c.App.Concurrency <= 0 {
		c.App.Concurrency = 64
	}
	if c.App.FetchTimeoutSec <= 0 {
		c.App.FetchTimeoutSec = 20
	}
	if c.App.TestTimeoutSec <= 0 {
		c.App.TestTimeoutSec = 6
	}
	if c.App.UserAgent == "" {
		c.App.UserAgent = Default().App.UserAgent
	}
	if c.Export.Dir == "" {
		c.Export.Dir = "output"
	}
	if c.Export.FilenamePrefix == "" {
		c.Export.FilenamePrefix = "nodes"
	}
	if len(c.Export.Formats) == 0 {
		c.Export.Formats = []string{"raw", "base64", "clash"}
	}
	if c.Filter.SortBy == "" {
		c.Filter.SortBy = "score"
	}
	if c.Filter.MinScore <= 0 {
		c.Filter.MinScore = 70
	}
	if c.Filter.MaxStoreNodes <= 0 {
		c.Filter.MaxStoreNodes = 8000
	}
	if c.Filter.MaxNodes <= 0 {
		c.Filter.MaxNodes = 500
	}
	if c.Schedule.IntervalMin <= 0 {
		c.Schedule.IntervalMin = 180
	}
	if c.Schedule.Job == "" {
		c.Schedule.Job = "full"
	}
	c.Schedule.Job = strings.ToLower(strings.TrimSpace(c.Schedule.Job))
	if c.Schedule.MaxTest <= 0 {
		c.Schedule.MaxTest = 1200
	}
	if c.Schedule.Rounds <= 0 {
		c.Schedule.Rounds = 2
	}
	if c.Publish.PathPrefix == "" {
		c.Publish.PathPrefix = "/sub"
	}
	if !strings.HasPrefix(c.Publish.PathPrefix, "/") {
		c.Publish.PathPrefix = "/" + c.Publish.PathPrefix
	}
	c.Publish.PathPrefix = strings.TrimRight(c.Publish.PathPrefix, "/")
	if c.Publish.MinScore <= 0 {
		c.Publish.MinScore = c.Filter.MinScore
	}
	if c.Publish.MaxNodes <= 0 {
		c.Publish.MaxNodes = c.Filter.MaxNodes
	}
	if len(c.Publish.Formats) == 0 {
		c.Publish.Formats = []string{"raw", "base64", "clash"}
	}
	if c.Publish.CacheSec < 0 {
		c.Publish.CacheSec = 0
	}
	if c.Geo.DBPath == "" {
		c.Geo.DBPath = "data/GeoLite2-Country.mmdb"
	}
	if c.Publish.MaxCountries <= 0 {
		c.Publish.MaxCountries = 30
	}
	if c.Security.SubRPS <= 0 {
		c.Security.SubRPS = 20
	}
	if c.Security.SubBurst <= 0 {
		c.Security.SubBurst = 40
	}
	if c.Security.APIRPS <= 0 {
		c.Security.APIRPS = 30
	}
	if c.Security.APIBurst <= 0 {
		c.Security.APIBurst = 60
	}
	if c.Server.ReadHeaderTimeoutSec <= 0 {
		c.Server.ReadHeaderTimeoutSec = 10
	}
	if c.Server.ShutdownTimeoutSec <= 0 {
		c.Server.ShutdownTimeoutSec = 30
	}
	if c.SQLite.Path == "" {
		c.SQLite.Path = "data/node-hunter.db"
	}
	if c.Dial.Concurrency <= 0 {
		c.Dial.Concurrency = 4
	}
	if c.Dial.TimeoutSec <= 0 {
		c.Dial.TimeoutSec = 18
	}
	if c.Dial.TestURL == "" {
		c.Dial.TestURL = "https://www.cloudflare.com/cdn-cgi/trace"
	}
	if c.Dial.MaxNodes <= 0 {
		c.Dial.MaxNodes = 80
	}
	if c.Dial.AfterQualityMax <= 0 {
		c.Dial.AfterQualityMax = 40
	}
	if c.Dial.Engine == "" {
		c.Dial.Engine = "sing-box"
	}
	if v := strings.TrimSpace(os.Getenv("NODE_HUNTER_SINGBOX")); v != "" {
		c.Dial.Bin = v
		c.Dial.Engine = "sing-box"
	}
	// 环境变量可覆盖 token / public url（部署友好）
	if v := strings.TrimSpace(os.Getenv("NODE_HUNTER_TOKEN")); v != "" {
		c.Publish.Token = v
	}
	if v := strings.TrimSpace(os.Getenv("NODE_HUNTER_PUBLIC_URL")); v != "" {
		c.Publish.PublicURL = strings.TrimRight(v, "/")
	}
	if v := strings.TrimSpace(os.Getenv("NODE_HUNTER_ADMIN_TOKEN")); v != "" {
		c.Security.AdminToken = v
	}
	if v := strings.TrimSpace(os.Getenv("NODE_HUNTER_SCHEDULE")); v != "" {
		switch strings.ToLower(v) {
		case "1", "true", "on", "yes", "enable", "enabled":
			c.Schedule.Enabled = true
		case "0", "false", "off", "no", "disable", "disabled":
			c.Schedule.Enabled = false
		}
	}
	for i := range c.Sources {
		if c.Sources[i].Type == "" {
			c.Sources[i].Type = "subscription"
		}
		if c.Sources[i].Name == "" {
			c.Sources[i].Name = fmt.Sprintf("source-%d", i+1)
		}
	}
}

func (c *Config) FetchTimeout() time.Duration {
	return time.Duration(c.App.FetchTimeoutSec) * time.Second
}

func (c *Config) TestTimeout() time.Duration {
	return time.Duration(c.App.TestTimeoutSec) * time.Second
}

func (c *Config) ScheduleInterval() time.Duration {
	return time.Duration(c.Schedule.IntervalMin) * time.Minute
}

// ProtocolAllowed 协议是否在白名单中
func (c *Config) ProtocolAllowed(p string) bool {
	if len(c.Protocols) == 0 {
		return true
	}
	p = strings.ToLower(strings.TrimSpace(p))
	if p == "hy2" {
		p = "hysteria2"
	}
	for _, allowed := range c.Protocols {
		a := strings.ToLower(allowed)
		if a == "hy2" {
			a = "hysteria2"
		}
		if a == p {
			return true
		}
	}
	return false
}

// EnabledSources 返回启用的源
func (c *Config) EnabledSources() []Source {
	out := make([]Source, 0, len(c.Sources))
	for _, s := range c.Sources {
		if s.Enabled && strings.TrimSpace(s.URL) != "" {
			out = append(out, s)
		}
	}
	return out
}

// ResolveConfigPath 尝试常见配置路径
func ResolveConfigPath(flagPath string) string {
	if flagPath != "" {
		return flagPath
	}
	candidates := []string{
		"configs/config.yaml",
		"config.yaml",
		filepath.Join("configs", "config.yml"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "configs/config.yaml"
}
