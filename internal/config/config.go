package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App           AppConfig           `yaml:"app"`
	Sources       []Source            `yaml:"sources"`
	Protocols     []string            `yaml:"protocols"`
	Filter        FilterConfig        `yaml:"filter"`
	Export        ExportConfig        `yaml:"export"`
	Schedule      ScheduleConfig      `yaml:"schedule"`
	Publish       PublishConfig       `yaml:"publish"`
	Geo           GeoConfig           `yaml:"geo"`
	Dial          DialConfig          `yaml:"dial"`
	Security      SecurityConfig      `yaml:"security"`
	Server        ServerConfig        `yaml:"server"`
	SQLite        SQLiteConfig        `yaml:"sqlite"`
	Database      DatabaseConfig      `yaml:"database"`
	Redis         RedisConfig         `yaml:"redis"`
	ObjectStore   ObjectStoreConfig   `yaml:"object_store"`
	Queue         QueueConfig         `yaml:"queue"`
	Auth          AuthConfig          `yaml:"auth"`
	SubStore      SubStoreConfig      `yaml:"sub_store"`
	Governance    GovernanceConfig    `yaml:"governance"`
	QualityV2     QualityV2Config     `yaml:"quality_v2"`
	Observability ObservabilityConfig `yaml:"observability"`
	Pools         []PoolConfig        `yaml:"pools"`
	Logging       LoggingConfig       `yaml:"logging"`
}

// DialConfig 真实协议拨测（sing-box / xray）
type DialConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Bin         string `yaml:"bin"`         // 空=自动查找
	Engine      string `yaml:"engine"`      // auto|sing-box|xray
	Concurrency int    `yaml:"concurrency"` // 同时核心实例数
	TimeoutSec  int    `yaml:"timeout_sec"`
	TestURL     string `yaml:"test_url"`
	// DownloadBytes is the maximum response payload read for throughput measurement.
	DownloadBytes int64 `yaml:"download_bytes"`
	// SamplePercent is used by automatic post-quality dial runs when no fixed maximum is set.
	SamplePercent float64 `yaml:"sample_percent"`
	// MaxNodes 单次最多拨测；0=不限制，对全部 HQ 分批真拨
	MaxNodes int `yaml:"max_nodes"`
	// BatchSize 多轮真拨每批数量（默认 200）
	BatchSize int `yaml:"batch_size"`
	// AfterQuality 在 quality/full 后自动真拨
	AfterQuality bool `yaml:"after_quality"`
	// AfterQualityMax 自动真测数量；0=全部 HQ（按 batch_size 多轮）
	AfterQualityMax int `yaml:"after_quality_max"`
}

// SecurityConfig 认证与限流
type SecurityConfig struct {
	AllowQueryToken bool    `yaml:"allow_query_token"` // 允许 ?token=
	SubRPS          float64 `yaml:"sub_rps"`           // 订阅限流每 IP
	SubBurst        int     `yaml:"sub_burst"`
	APIRPS          float64 `yaml:"api_rps"`
	APIBurst        int     `yaml:"api_burst"`
	LoginRPS        float64 `yaml:"login_rps"`
	LoginBurst      int     `yaml:"login_burst"`
}

// ServerConfig 运行时
type ServerConfig struct {
	ReadHeaderTimeoutSec int      `yaml:"read_header_timeout_sec"`
	ReadTimeoutSec       int      `yaml:"read_timeout_sec"`
	WriteTimeoutSec      int      `yaml:"write_timeout_sec"`
	IdleTimeoutSec       int      `yaml:"idle_timeout_sec"`
	ShutdownTimeoutSec   int      `yaml:"shutdown_timeout_sec"`
	MaxHeaderBytes       int      `yaml:"max_header_bytes"`
	TrustedProxies       []string `yaml:"trusted_proxies"`
	AllowedOrigins       []string `yaml:"allowed_origins"`
	EnableMetrics        bool     `yaml:"enable_metrics"`
	EnablePprof          bool     `yaml:"enable_pprof"`
}

// SQLiteConfig 企业持久化
type SQLiteConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
}

// GeoConfig IP 国家标注
type GeoConfig struct {
	Enabled        bool   `yaml:"enabled"`
	DBPath         string `yaml:"db_path"` // 默认 data/GeoLite2-Country.mmdb
	ASNDBPath      string `yaml:"asn_db_path"`
	DownloadURL    string `yaml:"download_url"` // 空则用内置镜像
	ASNDownloadURL string `yaml:"asn_download_url"`
	AutoDownload   bool   `yaml:"auto_download"` // 缺库时自动下载
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
	Name         string `yaml:"name"`
	Type         string `yaml:"type"` // subscription | raw | clash
	URL          string `yaml:"url"`
	Enabled      bool   `yaml:"enabled"`
	Priority     int    `yaml:"priority"`  // 越大越先采集；默认 50
	MaxBytes     int64  `yaml:"max_bytes"` // 单源响应上限；默认 32 MiB
	Jurisdiction string `yaml:"jurisdiction"`
}

type FilterConfig struct {
	MaxLatencyMS        int     `yaml:"max_latency_ms"`
	MinSuccess          bool    `yaml:"min_success"`
	DropInvalid         bool    `yaml:"drop_invalid"`
	PreferTLS           bool    `yaml:"prefer_tls"`
	MaxNodes            int     `yaml:"max_nodes"`
	SortBy              string  `yaml:"sort_by"`
	MinScore            float64 `yaml:"min_score"`           // 高质量阈值，默认 70
	PruneAfterQuality   bool    `yaml:"prune_after_quality"` // 测速后剔除死亡节点
	MaxStoreNodes       int     `yaml:"max_store_nodes"`     // 快照最多保留节点数
	CollapseSameIPPorts bool    `yaml:"collapse_same_ip_ports"`
}

type ExportConfig struct {
	Dir            string   `yaml:"dir"`
	Formats        []string `yaml:"formats"`
	FilenamePrefix string   `yaml:"filename_prefix"`
	KeepRuns       int      `yaml:"keep_runs"`
}

// ScheduleConfig 定时任务：采集 / 测速 / 全流程
type ScheduleConfig struct {
	Enabled     bool   `yaml:"enabled"`
	IntervalMin int    `yaml:"interval_min"` // 间隔分钟，默认 180
	Job         string `yaml:"job"`          // full | fetch | quality
	RunOnStart  bool   `yaml:"run_on_start"` // 启动后是否立即跑一轮
	SkipAI      bool   `yaml:"skip_ai"`      // full 时跳过 AI 探测（更快）
	MaxTest     int    `yaml:"max_test"`     // 测速上限
	Rounds      int    `yaml:"rounds"`       // 测速轮数
	JitterSec   int    `yaml:"jitter_sec"`   // 启动抖动，避免整点齐打
}

// PublishConfig 对外订阅发布（VPS 远程拉取）
type PublishConfig struct {
	Enabled         bool     `yaml:"enabled"`
	Token           string   `yaml:"token"`       // 空=公开；非空则 Bearer / X-Sub-Token 必填
	PathPrefix      string   `yaml:"path_prefix"` // 默认 /sub
	MinScore        float64  `yaml:"min_score"`   // 默认用 filter.min_score
	MaxNodes        int      `yaml:"max_nodes"`   // 默认用 filter.max_nodes
	MaxNodeAgeHours int      `yaml:"max_node_age_hours"`
	AliveOnly       bool     `yaml:"alive_only"`
	Formats         []string `yaml:"formats"`   // raw, base64, clash
	CacheSec        int      `yaml:"cache_sec"` // Cache-Control max-age
	PublicURL       string   `yaml:"public_url"`
	PreRender       bool     `yaml:"pre_render"`    // 任务后预渲染订阅
	MaxCountries    int      `yaml:"max_countries"` // 分国家缓存上限
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
			KeepRuns:       48,
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
			Enabled:         true,
			Token:           "",
			PathPrefix:      "/sub",
			MaxNodeAgeHours: 24,
			AliveOnly:       true,
			Formats:         []string{"raw", "base64", "clash"},
			CacheSec:        120,
			PreRender:       true,
			MaxCountries:    30,
		},
		Geo: GeoConfig{
			Enabled:              true,
			DBPath:               "data/GeoLite2-Country.mmdb",
			ASNDBPath:            "data/GeoLite2-ASN.mmdb",
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
			DownloadBytes:   256 << 10,
			SamplePercent:   10,
			MaxNodes:        0, // 0=全部 HQ
			BatchSize:       200,
			AfterQuality:    false,
			AfterQualityMax: 0, // 0=quality 后对全部 HQ 多轮真拨
		},
		Security: SecurityConfig{
			AllowQueryToken: false,
			SubRPS:          20,
			SubBurst:        40,
			APIRPS:          30,
			APIBurst:        60,
			LoginRPS:        0.2,
			LoginBurst:      5,
		},
		Server: ServerConfig{
			ReadHeaderTimeoutSec: 10,
			ReadTimeoutSec:       30,
			WriteTimeoutSec:      60,
			IdleTimeoutSec:       120,
			ShutdownTimeoutSec:   30,
			MaxHeaderBytes:       1 << 20,
			TrustedProxies:       []string{"127.0.0.1/32", "::1/128"},
			EnableMetrics:        true,
		},
		SQLite: SQLiteConfig{
			Enabled: true,
			Path:    "data/nodeharvest.db",
		},
		Database: DatabaseConfig{
			Enabled:             true,
			Driver:              "sqlite",
			DSN:                 "data/nodeharvest.db",
			MaxOpenConns:        10,
			MaxIdleConns:        5,
			JobRetentionDays:    30,
			AuditRetentionDays:  90,
			MetricRetentionDays: 30,
		},
		Redis: RedisConfig{
			Prefix:      "nodeharvest",
			CacheTTLSec: 300,
			LockTTLSec:  120,
		},
		ObjectStore: ObjectStoreConfig{Bucket: "nodeharvest", Prefix: "artifacts"},
		Queue: QueueConfig{
			MaxPending:   1000,
			LeaseSec:     120,
			PollMS:       500,
			MaxAttempts:  3,
			RetryBaseSec: 5,
		},
		Auth: AuthConfig{
			SessionTTLHours:   12,
			SessionCookieName: "nh_session",
			BootstrapTenant:   "default",
		},
		SubStore: SubStoreConfig{Version: "2.36.22"},
		Governance: GovernanceConfig{
			DisableAfterFailures: 5,
			CooldownHours:        6,
			HQDropPercent:        40,
			CountrySharePercent:  80,
			JobFailureThreshold:  2,
		},
		QualityV2: QualityV2Config{
			Latency:    0.25,
			Success:    0.25,
			Stability:  0.15,
			TLS:        0.10,
			HTTP:       0.15,
			Throughput: 0.10,
		},
		Observability: ObservabilityConfig{
			ServiceName:      "nodeharvest",
			SampleRatio:      0.1,
			ExportTimeoutSec: 5,
		},
		Pools: []PoolConfig{
			{Key: "global-hq", Name: "Global HQ", MinScore: 70, RefreshSec: 120, MaxNodes: 500},
			{Key: "verified", Name: "Verified", MinScore: 60, RequireVerified: true, RefreshSec: 300, MaxNodes: 300},
			{Key: "streaming", Name: "Streaming", MinScore: 70, MaxLatencyMS: 800, RefreshSec: 180, MaxNodes: 300},
			{Key: "ai-friendly", Name: "AI Friendly", MinScore: 60, RequireAI: true, RefreshSec: 300, MaxNodes: 300},
			{Key: "low-latency", Name: "Low Latency", MinScore: 50, MaxLatencyMS: 300, RefreshSec: 120, MaxNodes: 200},
		},
		Logging: LoggingConfig{Level: "info"},
	}
}

// Load 从 YAML 加载配置，文件不存在时返回默认配置
func Load(path string) (*Config, error) {
	cfg := Default()
	if path == "" {
		cfg.normalize()
		return cfg, cfg.Validate()
	}
	// #nosec G304 -- path is the operator-supplied process configuration file, not request input.
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg.normalize()
			return cfg, cfg.Validate()
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.normalize()
	return cfg, cfg.Validate()
}

func (c *Config) Validate() error {
	if c.App.Concurrency <= 0 || c.App.Concurrency > 4096 {
		return fmt.Errorf("app.concurrency must be between 1 and 4096")
	}
	if c.Filter.MaxNodes < 0 || c.Filter.MaxStoreNodes < 0 {
		return fmt.Errorf("filter node limits must not be negative")
	}
	if c.Dial.DownloadBytes < 1024 || c.Dial.DownloadBytes > 64<<20 {
		return fmt.Errorf("dial.download_bytes must be between 1024 and 67108864")
	}
	if c.Dial.SamplePercent < 0 || c.Dial.SamplePercent > 100 {
		return fmt.Errorf("dial.sample_percent must be between 0 and 100")
	}
	if c.Export.FilenamePrefix == "." || c.Export.FilenamePrefix == ".." ||
		strings.ContainsAny(c.Export.FilenamePrefix, `/\`) {
		return fmt.Errorf("export.filename_prefix must be a file name")
	}
	seenFormats := make(map[string]bool, len(c.Export.Formats))
	for _, format := range c.Export.Formats {
		var canonical string
		switch strings.ToLower(strings.TrimSpace(format)) {
		case "raw", "uri", "txt":
			canonical = "raw"
		case "base64", "sub", "subscription":
			canonical = "base64"
		case "clash", "yaml", "yml":
			canonical = "clash"
		case "json":
			canonical = "json"
		default:
			return fmt.Errorf("unsupported export format %q", format)
		}
		if seenFormats[canonical] {
			return fmt.Errorf("duplicate export format %q", format)
		}
		seenFormats[canonical] = true
	}
	switch c.Schedule.Job {
	case "full", "fetch", "quality":
	default:
		return fmt.Errorf("unsupported schedule.job %q", c.Schedule.Job)
	}
	prefix := c.Publish.PathPrefix
	if strings.ContainsAny(prefix, "{}?# \t\r\n") {
		return fmt.Errorf("publish.path_prefix contains unsupported characters")
	}
	if prefix == "/metrics" || strings.HasPrefix(prefix, "/debug") ||
		(prefix != "/api/sub" && (prefix == "/api" || strings.HasPrefix(prefix, "/api/"))) ||
		(prefix != "/sub" && strings.HasPrefix(prefix, "/sub/")) {
		return fmt.Errorf("publish.path_prefix %q conflicts with a built-in route", prefix)
	}
	seenNames := make(map[string]bool, len(c.Sources))
	for _, source := range c.Sources {
		if seenNames[source.Name] {
			return fmt.Errorf("duplicate source name %q", source.Name)
		}
		seenNames[source.Name] = true
		switch source.Type {
		case "subscription", "raw", "clash":
		default:
			return fmt.Errorf("source %q has unsupported type %q", source.Name, source.Type)
		}
		u, err := url.Parse(source.URL)
		if source.Enabled && (err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "") {
			return fmt.Errorf("source %q has invalid URL", source.Name)
		}
		if source.MaxBytes <= 0 || source.MaxBytes > 128<<20 {
			return fmt.Errorf("source %q max_bytes must be between 1 and 134217728", source.Name)
		}
		if source.Priority < 1 || source.Priority > 1000 {
			return fmt.Errorf("source %q priority must be between 1 and 1000", source.Name)
		}
	}
	return c.validateEnterprise()
}

func (c *Config) normalize() {
	c.normalizeEnterprise()
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
	if c.Export.KeepRuns <= 0 {
		c.Export.KeepRuns = 48
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
	if c.Publish.PathPrefix == "" {
		c.Publish.PathPrefix = "/sub"
	}
	if c.Publish.MinScore <= 0 {
		c.Publish.MinScore = c.Filter.MinScore
	}
	if c.Publish.MaxNodes <= 0 {
		c.Publish.MaxNodes = c.Filter.MaxNodes
	}
	if c.Publish.MaxNodeAgeHours <= 0 {
		c.Publish.MaxNodeAgeHours = 24
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
	if c.Geo.ASNDBPath == "" {
		c.Geo.ASNDBPath = "data/GeoLite2-ASN.mmdb"
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
	if c.Server.ReadTimeoutSec <= 0 {
		c.Server.ReadTimeoutSec = 30
	}
	if c.Server.WriteTimeoutSec <= 0 {
		c.Server.WriteTimeoutSec = 60
	}
	if c.Server.IdleTimeoutSec <= 0 {
		c.Server.IdleTimeoutSec = 120
	}
	if c.Server.ShutdownTimeoutSec <= 0 {
		c.Server.ShutdownTimeoutSec = 30
	}
	if c.Server.MaxHeaderBytes <= 0 {
		c.Server.MaxHeaderBytes = 1 << 20
	}
	if c.SQLite.Path == "" {
		c.SQLite.Path = "data/nodeharvest.db"
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
	// MaxNodes / AfterQualityMax: 0 表示不限制（全部 HQ），不再强制改成正数
	if c.Dial.BatchSize <= 0 {
		c.Dial.BatchSize = 200
	}
	if c.Dial.Engine == "" {
		c.Dial.Engine = "sing-box"
	}
	if v := strings.TrimSpace(os.Getenv("NODE_HARVEST_SINGBOX")); v != "" {
		c.Dial.Bin = v
		c.Dial.Engine = "sing-box"
	}
	// 环境变量可覆盖 token / public url（部署友好）
	if v := strings.TrimSpace(os.Getenv("NODE_HARVEST_TOKEN")); v != "" {
		c.Publish.Token = v
	}
	if v := strings.TrimSpace(os.Getenv("NODE_HARVEST_PUBLIC_URL")); v != "" {
		c.Publish.PublicURL = strings.TrimRight(v, "/")
	}
	if v, ok := envBool("NODE_HARVEST_SCHEDULE"); ok {
		c.Schedule.Enabled = v
	}
	// 多数订阅客户端（Clash、Resin 等）只能把 token 放进 URL，需要按部署放开，
	// 否则只能重建镜像才能改动这一项。
	if v, ok := envBool("NODE_HARVEST_ALLOW_QUERY_TOKEN"); ok {
		c.Security.AllowQueryToken = v
	}
	for i := range c.Sources {
		if c.Sources[i].Type == "" {
			c.Sources[i].Type = "subscription"
		}
		if c.Sources[i].Name == "" {
			c.Sources[i].Name = fmt.Sprintf("source-%d", i+1)
		}
		if c.Sources[i].Priority == 0 {
			c.Sources[i].Priority = 50
		}
		if c.Sources[i].MaxBytes <= 0 {
			c.Sources[i].MaxBytes = 32 << 20
		}
	}
}

// envBool 解析部署常用的布尔环境变量；第二个返回值表示该变量是否已显式设置，
// 未设置或取值无法识别时保持配置文件中的原值。
func envBool(key string) (value bool, set bool) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "on", "yes", "enable", "enabled":
		return true, true
	case "0", "false", "off", "no", "disable", "disabled":
		return false, true
	}
	return false, false
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
			blocked := false
			for _, country := range c.Governance.DisabledSourceCountries {
				if strings.EqualFold(strings.TrimSpace(country), strings.TrimSpace(s.Jurisdiction)) &&
					strings.TrimSpace(s.Jurisdiction) != "" {
					blocked = true
					break
				}
			}
			if blocked {
				continue
			}
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Priority > out[j].Priority
	})
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
