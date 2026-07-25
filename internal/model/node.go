package model

import (
	"fmt"
	"strings"
	"time"
)

// 说明：Job / Node 中的 time.Time 经 encoding/json 序列化时，
// 会带上该 Time 的 Location。进程 TZ=Asia/Shanghai 时一般为 +08:00。
// 手写字符串时间请统一用 internal/timex（Asia/Shanghai RFC3339）。
// 勿再对对外字段使用 .UTC().Format。

// Protocol 支持的代理协议
type Protocol string

const (
	ProtoVMess     Protocol = "vmess"
	ProtoVLESS     Protocol = "vless"
	ProtoTrojan    Protocol = "trojan"
	ProtoSS        Protocol = "ss"
	ProtoSSR       Protocol = "ssr"
	ProtoHysteria2 Protocol = "hysteria2"
	ProtoTUIC      Protocol = "tuic"
	ProtoUnknown   Protocol = "unknown"
)

// Node 统一节点模型
type Node struct {
	ID       string   `json:"id"`
	Protocol Protocol `json:"protocol"`
	Name     string   `json:"name"`
	Server   string   `json:"server"`
	Port     int      `json:"port"`
	UUID     string   `json:"uuid,omitempty"`
	Password string   `json:"password,omitempty"`
	Method   string   `json:"method,omitempty"`
	Network  string   `json:"network,omitempty"`
	TLS      bool     `json:"tls"`
	SNI      string   `json:"sni,omitempty"`
	Path     string   `json:"path,omitempty"`
	Host     string   `json:"host,omitempty"`
	Flow     string   `json:"flow,omitempty"`
	Security string   `json:"security,omitempty"`
	ALPN     string   `json:"alpn,omitempty"`
	Extra    map[string]string `json:"extra,omitempty"`

	RawURI string `json:"raw_uri"`
	Source string `json:"source"`

	// 基础测活
	Alive    bool          `json:"alive"`
	Latency  time.Duration `json:"latency_ms"`
	Error    string        `json:"error,omitempty"`
	TestedAt time.Time     `json:"tested_at,omitempty"`

	// 智能质量
	Quality *Quality `json:"quality,omitempty"`

	// AI 站点可达（需代理链路或启发式）
	AIAccess map[string]*AIProbeResult `json:"ai_access,omitempty"`

	// 真实协议拨测（xray/sing-box）
	Dial     *DialResult `json:"dial,omitempty"`
	Verified bool        `json:"verified"` // 最近一次真实拨测通过

	// IP 纯净度 / Cloudflare 挑战探测（经代理真实出口）
	Purity *PurityResult `json:"purity,omitempty"`

	Score       float64 `json:"score"`
	Grade       string  `json:"grade"` // S A B C D F
	Fingerprint string  `json:"fingerprint"`
	Country     string  `json:"country,omitempty"`
	City        string  `json:"city,omitempty"`
	ISP         string  `json:"isp,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// PurityResult 出口 IP 纯净度与 Cloudflare 挑战启发式结果
// 说明：无法代替真人完成 Turnstile/验证码，只能判断是否被要求挑战、以及常见风险库标记。
type PurityResult struct {
	OK           bool      `json:"ok"`                      // 代理链路可用且完成至少一项检测
	ExitIP       string    `json:"exit_ip,omitempty"`
	Country      string    `json:"country,omitempty"`
	ISP          string    `json:"isp,omitempty"`
	AS           string    `json:"as,omitempty"`
	IsProxy      bool      `json:"is_proxy"`                 // 第三方标记为代理
	IsHosting    bool      `json:"is_hosting"`               // 机房/托管
	IsMobile     bool      `json:"is_mobile"`
	RiskScore    int       `json:"risk_score"`               // 0-100，越高越脏
	CleanScore   int       `json:"clean_score"`              // 0-100，越高越干净
	Grade        string    `json:"grade,omitempty"`          // S A B C D F
	CFTraceOK    bool      `json:"cf_trace_ok"`              // cdn-cgi/trace 成功
	CFChallenge  string    `json:"cf_challenge,omitempty"`   // none|soft|hard|blocked|error
	CFHumanLikely bool     `json:"cf_human_likely"`          // 启发式：未出挑战且页面可达
	Notes        []string  `json:"notes,omitempty"`
	Error        string    `json:"error,omitempty"`
	LatencyMS    int64     `json:"latency_ms,omitempty"`
	TestedAt     time.Time `json:"tested_at,omitempty"`
}

// DialResult 真实协议拨测结果
type DialResult struct {
	OK         bool      `json:"ok"`
	LatencyMS  int64     `json:"latency_ms"`
	StatusCode int       `json:"status_code,omitempty"`
	Target     string    `json:"target,omitempty"`
	Engine     string    `json:"engine,omitempty"` // sing-box | xray
	Error      string    `json:"error,omitempty"`
	TestedAt   time.Time `json:"tested_at,omitempty"`
}

// Quality 多维质量指标
type Quality struct {
	Rounds       int     `json:"rounds"`
	SuccessRate  float64 `json:"success_rate"` // 0-1
	AvgLatencyMS int64   `json:"avg_latency_ms"`
	MinLatencyMS int64   `json:"min_latency_ms"`
	MaxLatencyMS int64   `json:"max_latency_ms"`
	JitterMS     int64   `json:"jitter_ms"`
	TLSOK        bool    `json:"tls_ok"`
	TLSMS        int64   `json:"tls_ms,omitempty"`
	// 对常见 CDN/AI 边缘 IP 的 TCP 连通启发分 0-100
	EdgeScore float64 `json:"edge_score"`
	// 综合质量分 0-100
	Score float64 `json:"score"`
	Notes []string `json:"notes,omitempty"`
}

// AIProbeResult 单个 AI 站点探测结果
type AIProbeResult struct {
	Target     string `json:"target"`
	URL        string `json:"url"`
	OK         bool   `json:"ok"`
	StatusCode int    `json:"status_code,omitempty"`
	LatencyMS  int64  `json:"latency_ms"`
	Error      string `json:"error,omitempty"`
	Mode       string `json:"mode"` // direct | via_proxy | heuristic
}

// Address 返回 host:port
func (n *Node) Address() string {
	return fmt.Sprintf("%s:%d", n.Server, n.Port)
}

// IsValid 基础字段是否可用
func (n *Node) IsValid() bool {
	if n == nil {
		return false
	}
	if n.Protocol == "" || n.Protocol == ProtoUnknown {
		return false
	}
	if strings.TrimSpace(n.Server) == "" || n.Port <= 0 || n.Port > 65535 {
		return false
	}
	return true
}

// Key 用于去重的指纹键
func (n *Node) Key() string {
	if n.Fingerprint != "" {
		return n.Fingerprint
	}
	return strings.ToLower(fmt.Sprintf("%s|%s|%d|%s|%s|%s",
		n.Protocol, n.Server, n.Port, n.UUID, n.Password, n.Method))
}

// LatencyMS 毫秒延迟
func (n *Node) LatencyMS() int64 {
	if n.Latency <= 0 {
		return 0
	}
	return n.Latency.Milliseconds()
}

// AssignGrade 根据分数定级
func AssignGrade(score float64) string {
	switch {
	case score >= 90:
		return "S"
	case score >= 80:
		return "A"
	case score >= 65:
		return "B"
	case score >= 50:
		return "C"
	case score >= 35:
		return "D"
	default:
		return "F"
	}
}

// Result 管道运行结果摘要
type Result struct {
	FetchedSources int     `json:"fetched_sources"`
	RawCount       int     `json:"raw_count"`
	ParsedCount    int     `json:"parsed_count"`
	UniqueCount    int     `json:"unique_count"`
	AliveCount     int     `json:"alive_count"`
	ExportedCount  int     `json:"exported_count"`
	HighQuality    int     `json:"high_quality"`
	Nodes          []*Node `json:"nodes"`
	Duration       string  `json:"duration"`
}

// JobStatus 异步任务状态
type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobRunning   JobStatus = "running"
	JobCompleted JobStatus = "completed"
	JobFailed    JobStatus = "failed"
	JobCanceled  JobStatus = "canceled"
)

// Job 后台任务
type Job struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"` // fetch | quality | ai | full
	Status    JobStatus              `json:"status"`
	Progress  float64                `json:"progress"` // 0-100
	Message   string                 `json:"message"`
	Error     string                 `json:"error,omitempty"`
	Stats     map[string]any         `json:"stats,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
	StartedAt *time.Time             `json:"started_at,omitempty"`
	EndedAt   *time.Time             `json:"ended_at,omitempty"`
	Options   map[string]any         `json:"options,omitempty"`
}

// DashboardStats 仪表盘统计
type DashboardStats struct {
	TotalNodes     int            `json:"total_nodes"`
	AliveNodes     int            `json:"alive_nodes"`
	HighQuality    int            `json:"high_quality"` // score >= 70
	ByProtocol     map[string]int `json:"by_protocol"`
	ByGrade        map[string]int `json:"by_grade"`
	BySource       map[string]int `json:"by_source"`
	ByCountry      map[string]int `json:"by_country"`    // 全部
	ByCountryHQ    map[string]int `json:"by_country_hq"` // 高质量存活
	AvgLatencyMS   int64          `json:"avg_latency_ms"`
	AIPassRate     map[string]float64 `json:"ai_pass_rate"`
	LastFetchAt    *time.Time     `json:"last_fetch_at,omitempty"`
	LastQualityAt  *time.Time     `json:"last_quality_at,omitempty"`
	SourcesEnabled int            `json:"sources_enabled"`
}

// AITarget 预置 AI 探测目标
type AITarget struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	URL  string `json:"url"`
	Host string `json:"host"`
}

// DefaultAITargets 默认 AI 站点
func DefaultAITargets() []AITarget {
	return []AITarget{
		{Key: "chatgpt", Name: "ChatGPT", URL: "https://chatgpt.com", Host: "chatgpt.com"},
		{Key: "openai", Name: "OpenAI API", URL: "https://api.openai.com/v1/models", Host: "api.openai.com"},
		{Key: "gemini", Name: "Gemini", URL: "https://gemini.google.com", Host: "gemini.google.com"},
		{Key: "claude", Name: "Claude", URL: "https://claude.ai", Host: "claude.ai"},
		{Key: "grok", Name: "Grok", URL: "https://grok.x.ai", Host: "grok.x.ai"},
		{Key: "xai", Name: "xAI", URL: "https://x.ai", Host: "x.ai"},
		{Key: "copilot", Name: "Copilot", URL: "https://copilot.microsoft.com", Host: "copilot.microsoft.com"},
		{Key: "perplexity", Name: "Perplexity", URL: "https://www.perplexity.ai", Host: "www.perplexity.ai"},
	}
}
