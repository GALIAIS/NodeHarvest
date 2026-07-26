package config

import (
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"strings"
)

// DatabaseConfig selects the durable metadata and history database.
// SQLite remains the zero-service default; PostgreSQL is used by API/worker deployments.
type DatabaseConfig struct {
	Enabled             bool   `yaml:"enabled"`
	Driver              string `yaml:"driver"` // sqlite|postgres
	DSN                 string `yaml:"dsn"`
	MaxOpenConns        int    `yaml:"max_open_conns"`
	MaxIdleConns        int    `yaml:"max_idle_conns"`
	JobRetentionDays    int    `yaml:"job_retention_days"`
	AuditRetentionDays  int    `yaml:"audit_retention_days"`
	MetricRetentionDays int    `yaml:"metric_retention_days"`
}

type RedisConfig struct {
	Enabled     bool   `yaml:"enabled"`
	URL         string `yaml:"url"`
	Prefix      string `yaml:"prefix"`
	CacheTTLSec int    `yaml:"cache_ttl_sec"`
	LockTTLSec  int    `yaml:"lock_ttl_sec"`
}

type ObjectStoreConfig struct {
	Enabled          bool   `yaml:"enabled"`
	Endpoint         string `yaml:"endpoint"`
	AccessKey        string `yaml:"access_key"`
	SecretKey        string `yaml:"secret_key"`
	SessionToken     string `yaml:"session_token"`
	Bucket           string `yaml:"bucket"`
	Prefix           string `yaml:"prefix"`
	Region           string `yaml:"region"`
	AutoCreateBucket bool   `yaml:"auto_create_bucket"`
}

type QueueConfig struct {
	Enabled         bool `yaml:"enabled"`
	EmbeddedWorkers int  `yaml:"embedded_workers"`
	MaxPending      int  `yaml:"max_pending"`
	LeaseSec        int  `yaml:"lease_sec"`
	PollMS          int  `yaml:"poll_ms"`
	MaxAttempts     int  `yaml:"max_attempts"`
	RetryBaseSec    int  `yaml:"retry_base_sec"`
}

type AuthConfig struct {
	LocalEnabled      bool       `yaml:"local_enabled"`
	BootstrapUser     string     `yaml:"bootstrap_user"`
	BootstrapTenant   string     `yaml:"bootstrap_tenant"`
	BootstrapHash     string     `yaml:"bootstrap_password_hash"`
	SessionSecret     string     `yaml:"session_secret"`
	SessionTTLHours   int        `yaml:"session_ttl_hours"`
	SessionCookieName string     `yaml:"session_cookie_name"`
	DefaultRole       string     `yaml:"default_role"`
	RoleClaim         string     `yaml:"role_claim"`
	TenantClaim       string     `yaml:"tenant_claim"`
	AdminHost         string     `yaml:"admin_host"`
	PublicHost        string     `yaml:"public_host"`
	AdminCIDRs        []string   `yaml:"admin_cidrs"`
	OIDC              OIDCConfig `yaml:"oidc"`
}

type OIDCConfig struct {
	Enabled             bool     `yaml:"enabled"`
	IssuerURL           string   `yaml:"issuer_url"`
	ClientID            string   `yaml:"client_id"`
	ClientSecret        string   `yaml:"client_secret"`
	RedirectURL         string   `yaml:"redirect_url"`
	Scopes              []string `yaml:"scopes"`
	AllowedEmailDomains []string `yaml:"allowed_email_domains"`
}

type GovernanceConfig struct {
	DisabledSourceCountries []string `yaml:"disabled_source_countries"`
	DisableAfterFailures    int      `yaml:"disable_after_failures"`
	CooldownHours           int      `yaml:"cooldown_hours"`
	HQDropPercent           float64  `yaml:"hq_drop_percent"`
	CountrySharePercent     float64  `yaml:"country_share_percent"`
	JobFailureThreshold     int      `yaml:"job_failure_threshold"`
	AlertWebhookURL         string   `yaml:"alert_webhook_url"`
	AlertWebhookSecret      string   `yaml:"alert_webhook_secret"`
	TermsURL                string   `yaml:"terms_url"`
}

// QualityV2Config contains relative weights; Compute normalizes their sum.
type QualityV2Config struct {
	Latency    float64 `yaml:"latency"`
	Success    float64 `yaml:"success"`
	Stability  float64 `yaml:"stability"`
	TLS        float64 `yaml:"tls"`
	HTTP       float64 `yaml:"http"`
	Throughput float64 `yaml:"throughput"`
}

type ObservabilityConfig struct {
	OTLPEndpoint     string            `yaml:"otlp_endpoint"`
	OTLPInsecure     bool              `yaml:"otlp_insecure"`
	ServiceName      string            `yaml:"service_name"`
	SampleRatio      float64           `yaml:"sample_ratio"`
	ExportTimeoutSec int               `yaml:"export_timeout_sec"`
	ResourceAttrs    map[string]string `yaml:"resource_attrs"`
}

type PoolConfig struct {
	Key             string   `yaml:"key" json:"key"`
	Name            string   `yaml:"name" json:"name"`
	MinScore        float64  `yaml:"min_score" json:"min_score"`
	MaxLatencyMS    int      `yaml:"max_latency_ms" json:"max_latency_ms"`
	Countries       []string `yaml:"countries" json:"countries,omitempty"`
	Protocols       []string `yaml:"protocols" json:"protocols,omitempty"`
	RequireAI       bool     `yaml:"require_ai" json:"require_ai"`
	RequireVerified bool     `yaml:"require_verified" json:"require_verified"`
	RefreshSec      int      `yaml:"refresh_sec" json:"refresh_sec"`
	MaxNodes        int      `yaml:"max_nodes" json:"max_nodes"`
}

func (c *Config) normalizeEnterprise() {
	if c.Database.Driver == "" {
		c.Database.Driver = "sqlite"
	}
	c.Database.Driver = strings.ToLower(strings.TrimSpace(c.Database.Driver))
	if c.Database.Driver == "sqlite" && c.Database.DSN == "" {
		c.Database.DSN = c.SQLite.Path
	}
	if c.Database.MaxOpenConns <= 0 {
		c.Database.MaxOpenConns = 10
	}
	if c.Database.MaxIdleConns < 0 {
		c.Database.MaxIdleConns = 0
	}
	if c.Database.JobRetentionDays <= 0 {
		c.Database.JobRetentionDays = 30
	}
	if c.Database.AuditRetentionDays <= 0 {
		c.Database.AuditRetentionDays = 90
	}
	if c.Database.MetricRetentionDays <= 0 {
		c.Database.MetricRetentionDays = 30
	}
	if c.Redis.Prefix == "" {
		c.Redis.Prefix = "nodeharvest"
	}
	c.Redis.Prefix = strings.Trim(c.Redis.Prefix, ": ")
	if c.Redis.CacheTTLSec <= 0 {
		c.Redis.CacheTTLSec = 300
	}
	if c.Redis.LockTTLSec <= 0 {
		c.Redis.LockTTLSec = 120
	}
	c.ObjectStore.Prefix = strings.Trim(c.ObjectStore.Prefix, "/ ")
	if c.ObjectStore.Bucket == "" {
		c.ObjectStore.Bucket = "nodeharvest"
	}
	if c.Queue.MaxPending <= 0 {
		c.Queue.MaxPending = 1000
	}
	if c.Queue.LeaseSec <= 0 {
		c.Queue.LeaseSec = 120
	}
	if c.Queue.PollMS <= 0 {
		c.Queue.PollMS = 500
	}
	if c.Queue.MaxAttempts <= 0 {
		c.Queue.MaxAttempts = 3
	}
	if c.Queue.RetryBaseSec <= 0 {
		c.Queue.RetryBaseSec = 5
	}
	if c.Auth.SessionTTLHours <= 0 {
		c.Auth.SessionTTLHours = 12
	}
	if c.Auth.BootstrapTenant == "" {
		c.Auth.BootstrapTenant = "default"
	}
	if c.Auth.SessionCookieName == "" {
		c.Auth.SessionCookieName = "nh_session"
	}
	if c.Auth.DefaultRole == "" {
		c.Auth.DefaultRole = "viewer"
	}
	if c.Auth.RoleClaim == "" {
		c.Auth.RoleClaim = "role"
	}
	if c.Auth.TenantClaim == "" {
		c.Auth.TenantClaim = "tenant"
	}
	if len(c.Auth.OIDC.Scopes) == 0 {
		c.Auth.OIDC.Scopes = []string{"openid", "profile", "email"}
	}
	if c.Governance.DisableAfterFailures <= 0 {
		c.Governance.DisableAfterFailures = 5
	}
	if c.Governance.CooldownHours <= 0 {
		c.Governance.CooldownHours = 6
	}
	if c.Governance.HQDropPercent <= 0 {
		c.Governance.HQDropPercent = 40
	}
	if c.Governance.CountrySharePercent <= 0 {
		c.Governance.CountrySharePercent = 80
	}
	if c.Governance.JobFailureThreshold <= 0 {
		c.Governance.JobFailureThreshold = 2
	}
	if c.Observability.ServiceName == "" {
		c.Observability.ServiceName = "nodeharvest"
	}
	if c.Observability.SampleRatio < 0 || c.Observability.SampleRatio > 1 {
		c.Observability.SampleRatio = 0.1
	}
	if c.Observability.ExportTimeoutSec <= 0 {
		c.Observability.ExportTimeoutSec = 5
	}
	if v := strings.TrimSpace(os.Getenv("NODE_HARVEST_DATABASE_URL")); v != "" {
		c.Database.Driver, c.Database.DSN = "postgres", v
	}
	if v := strings.TrimSpace(os.Getenv("NODE_HARVEST_REDIS_URL")); v != "" {
		c.Redis.Enabled, c.Redis.URL = true, v
	}
	if v := strings.TrimSpace(os.Getenv("NODE_HARVEST_OBJECT_STORE_ENDPOINT")); v != "" {
		c.ObjectStore.Enabled, c.ObjectStore.Endpoint = true, v
	}
	if v := strings.TrimSpace(os.Getenv("NODE_HARVEST_OBJECT_STORE_ACCESS_KEY")); v != "" {
		c.ObjectStore.AccessKey = v
	}
	if v := strings.TrimSpace(os.Getenv("NODE_HARVEST_OBJECT_STORE_SECRET_KEY")); v != "" {
		c.ObjectStore.SecretKey = v
	}
	if v := strings.TrimSpace(os.Getenv("NODE_HARVEST_OBJECT_STORE_SESSION_TOKEN")); v != "" {
		c.ObjectStore.SessionToken = v
	}
	if v := strings.TrimSpace(os.Getenv("NODE_HARVEST_OBJECT_STORE_BUCKET")); v != "" {
		c.ObjectStore.Bucket = v
	}
	if v := strings.TrimSpace(os.Getenv("NODE_HARVEST_OBJECT_STORE_AUTO_CREATE")); v != "" {
		switch strings.ToLower(v) {
		case "1", "true", "on", "yes":
			c.ObjectStore.AutoCreateBucket = true
		case "0", "false", "off", "no":
			c.ObjectStore.AutoCreateBucket = false
		}
	}
	if v := strings.TrimSpace(os.Getenv("NODE_HARVEST_EMBEDDED_WORKERS")); v != "" {
		var workers int
		if _, err := fmt.Sscanf(v, "%d", &workers); err == nil {
			c.Queue.EmbeddedWorkers = workers
		}
	}
	if v := strings.TrimSpace(os.Getenv("NODE_HARVEST_SESSION_SECRET")); v != "" {
		c.Auth.SessionSecret = v
	}
	if v := strings.TrimSpace(os.Getenv("NODE_HARVEST_BOOTSTRAP_PASSWORD_HASH")); v != "" {
		c.Auth.BootstrapHash = v
	}
	if v := strings.TrimSpace(os.Getenv("NODE_HARVEST_OIDC_CLIENT_SECRET")); v != "" {
		c.Auth.OIDC.ClientSecret = v
	}
	if v := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")); v != "" {
		c.Observability.OTLPEndpoint = v
	}
	if v := strings.TrimSpace(os.Getenv("NODE_HARVEST_ALERT_WEBHOOK_SECRET")); v != "" {
		c.Governance.AlertWebhookSecret = v
	}
}

func (c *Config) validateEnterprise() error {
	if c.Security.LoginRPS <= 0 || c.Security.LoginRPS > 100 || c.Security.LoginBurst < 1 || c.Security.LoginBurst > 1000 {
		return fmt.Errorf("security login rate limits are invalid")
	}
	if c.Database.Enabled {
		switch c.Database.Driver {
		case "sqlite":
			if strings.TrimSpace(c.Database.DSN) == "" {
				return fmt.Errorf("database.dsn is required for sqlite")
			}
		case "postgres":
			u, err := url.Parse(c.Database.DSN)
			if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Host == "" {
				return fmt.Errorf("database.dsn must be a postgres URL")
			}
		default:
			return fmt.Errorf("database.driver must be sqlite or postgres")
		}
	}
	if c.Redis.Enabled {
		u, err := url.Parse(c.Redis.URL)
		if err != nil || (u.Scheme != "redis" && u.Scheme != "rediss") || u.Host == "" {
			return fmt.Errorf("redis.url must be a redis or rediss URL")
		}
	}
	if c.ObjectStore.Enabled {
		u, err := url.Parse(c.ObjectStore.Endpoint)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("object_store.endpoint must be an HTTP(S) URL")
		}
		if c.ObjectStore.AccessKey == "" || c.ObjectStore.SecretKey == "" || c.ObjectStore.Bucket == "" {
			return fmt.Errorf("object_store access_key, secret_key, and bucket are required")
		}
	}
	if c.Queue.EmbeddedWorkers < 0 || c.Queue.EmbeddedWorkers > 64 {
		return fmt.Errorf("queue.embedded_workers must be between 0 and 64")
	}
	if c.Queue.Enabled && !c.Database.Enabled {
		return fmt.Errorf("queue.enabled requires database.enabled")
	}
	for _, role := range []string{c.Auth.DefaultRole} {
		if role != "viewer" && role != "operator" && role != "admin" {
			return fmt.Errorf("auth.default_role must be viewer, operator, or admin")
		}
	}
	if c.Auth.LocalEnabled && (c.Auth.BootstrapUser == "" || c.Auth.BootstrapHash == "") {
		return fmt.Errorf("local auth requires bootstrap_user and bootstrap_password_hash")
	}
	if !validTenant(c.Auth.BootstrapTenant) {
		return fmt.Errorf("auth.bootstrap_tenant must be a lowercase tenant identifier")
	}
	if strings.ContainsAny(c.Auth.SessionCookieName, " ;,\t\r\n") {
		return fmt.Errorf("auth.session_cookie_name contains invalid characters")
	}
	for _, host := range []string{c.Auth.AdminHost, c.Auth.PublicHost} {
		if strings.Contains(host, "://") || strings.ContainsAny(host, "/?#") {
			return fmt.Errorf("auth hosts must contain a hostname only")
		}
	}
	for _, cidr := range c.Auth.AdminCIDRs {
		if _, err := netip.ParsePrefix(strings.TrimSpace(cidr)); err != nil {
			if _, addrErr := netip.ParseAddr(strings.TrimSpace(cidr)); addrErr != nil {
				return fmt.Errorf("invalid auth.admin_cidrs entry %q", cidr)
			}
		}
	}
	if (c.Auth.LocalEnabled || c.Auth.OIDC.Enabled) && len(c.Auth.SessionSecret) < 32 {
		return fmt.Errorf("auth.session_secret must contain at least 32 characters")
	}
	if c.Auth.OIDC.Enabled {
		if c.Auth.OIDC.IssuerURL == "" || c.Auth.OIDC.ClientID == "" ||
			c.Auth.OIDC.ClientSecret == "" || c.Auth.OIDC.RedirectURL == "" {
			return fmt.Errorf("OIDC requires issuer_url, client_id, client_secret, and redirect_url")
		}
	}
	if c.Governance.AlertWebhookURL != "" {
		u, err := url.Parse(c.Governance.AlertWebhookURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return fmt.Errorf("governance.alert_webhook_url must be an HTTP(S) URL")
		}
	}
	weights := c.QualityV2
	if weights.Latency < 0 || weights.Success < 0 || weights.Stability < 0 ||
		weights.TLS < 0 || weights.HTTP < 0 || weights.Throughput < 0 {
		return fmt.Errorf("quality_v2 weights must not be negative")
	}
	seen := make(map[string]bool, len(c.Pools))
	for _, pool := range c.Pools {
		key := strings.ToLower(strings.TrimSpace(pool.Key))
		if key == "" || seen[key] {
			return fmt.Errorf("pool keys must be non-empty and unique")
		}
		seen[key] = true
		if pool.MinScore < 0 || pool.MinScore > 100 || pool.MaxLatencyMS < 0 ||
			pool.RefreshSec < 0 || pool.MaxNodes < 0 {
			return fmt.Errorf("pool %q has invalid limits", pool.Key)
		}
	}
	return nil
}

func validTenant(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for i, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (i > 0 && (r == '-' || r == '_')) {
			continue
		}
		return false
	}
	return true
}
