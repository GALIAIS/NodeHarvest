package config

import (
	"net/url"
	"testing"
)

func TestRepositoryConfigSourcesAreNormalizedByPriority(t *testing.T) {
	cfg, err := Load("../../configs/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sources) < 130 {
		t.Fatalf("catalog sources=%d", len(cfg.Sources))
	}
	if cfg.Dial.Engine != "both" || cfg.Dial.VerifiedTTLHours != 6 || cfg.Dial.SamplePercent != 0 {
		t.Fatalf("repository dial policy is not full Mihomo verification: %+v", cfg.Dial)
	}
	sources := cfg.EnabledSources()
	if len(sources) < 125 {
		t.Fatalf("enabled sources=%d", len(sources))
	}
	names := make(map[string]struct{}, len(cfg.Sources))
	for _, source := range cfg.Sources {
		if _, exists := names[source.Name]; exists {
			t.Fatalf("duplicate source name %q", source.Name)
		}
		names[source.Name] = struct{}{}
		parsed, err := url.Parse(source.URL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			t.Fatalf("source %s has invalid HTTPS URL %q", source.Name, source.URL)
		}
		if source.MaxBytes <= 0 {
			t.Fatalf("source %s has no byte limit", source.Name)
		}
	}
	for i := 1; i < len(sources); i++ {
		if sources[i-1].Priority < sources[i].Priority {
			t.Fatalf("priority order broken at %d", i)
		}
	}
}

func TestAllowQueryTokenEnvOverride(t *testing.T) {
	// 未设置时保持配置文件的值；显式设置时覆盖。仓库默认是 false。
	cfg, err := Load("../../configs/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Security.AllowQueryToken {
		t.Fatal("repository config should keep query tokens disabled by default")
	}

	for _, tc := range []struct {
		env  string
		want bool
	}{
		{"1", true},
		{"true", true},
		{"enabled", true},
		{"0", false},
		{"off", false},
	} {
		t.Setenv("NODE_HARVEST_ALLOW_QUERY_TOKEN", tc.env)
		cfg, err := Load("../../configs/config.yaml")
		if err != nil {
			t.Fatalf("%s: %v", tc.env, err)
		}
		if cfg.Security.AllowQueryToken != tc.want {
			t.Fatalf("env=%q allow_query_token=%v want %v", tc.env, cfg.Security.AllowQueryToken, tc.want)
		}
	}

	// 无法识别的取值不应改变配置文件中的设定
	t.Setenv("NODE_HARVEST_ALLOW_QUERY_TOKEN", "maybe")
	cfg, err = Load("../../configs/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Security.AllowQueryToken {
		t.Fatal("unrecognized env value must not enable query tokens")
	}
}

func TestSubStoreEnvironmentAndCookieBoundary(t *testing.T) {
	t.Setenv("NODE_HARVEST_SUB_STORE_ENABLED", "1")
	t.Setenv("NODE_HARVEST_SUB_STORE_PUBLIC_URL", "https://store.node.example.com/")
	t.Setenv("NODE_HARVEST_SUB_STORE_BACKEND_PATH", "private-backend")
	t.Setenv("NODE_HARVEST_SESSION_COOKIE_DOMAIN", ".node.example.com")
	t.Setenv("NODE_HARVEST_ADMIN_HOST", "admin.node.example.com")
	t.Setenv("NODE_HARVEST_PUBLIC_HOST", "node.example.com")
	t.Setenv("NODE_HARVEST_TRUSTED_PROXIES", "172.16.0.0/12, 10.0.0.0/8")

	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SubStore.Enabled || cfg.SubStore.PublicURL != "https://store.node.example.com" ||
		cfg.SubStore.BackendPath != "/private-backend" ||
		cfg.Auth.SessionCookieDomain != "node.example.com" ||
		cfg.Auth.AdminHost != "admin.node.example.com" || cfg.Auth.PublicHost != "node.example.com" ||
		len(cfg.Server.TrustedProxies) != 2 {
		t.Fatalf("sub-store environment was not normalized: %+v %+v", cfg.SubStore, cfg.Auth)
	}

	cfg.SubStore.PublicURL = "https://store.example.net"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected cross-domain Sub-Store host to be rejected")
	}
	cfg.SubStore.PublicURL = "https://store.node.example.com"
	cfg.SubStore.BackendPath = "/nested/path"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected multi-segment Sub-Store backend path to be rejected")
	}
	cfg = Default()
	cfg.Auth.SessionCookieDomain = "co.uk"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected public-suffix cookie domain to be rejected")
	}
}

func TestPublishPrefixRejectsBuiltInRoutes(t *testing.T) {
	cfg := Default()
	cfg.Publish.PathPrefix = "/api/health"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected conflicting publish prefix error")
	}
}

func TestRuntimePatchValidatesAndCopies(t *testing.T) {
	cfg := Default()
	minScore := 88.0
	updated, err := ApplyRuntimePatch(cfg, RuntimePatch{PublishMinScore: &minScore})
	if err != nil {
		t.Fatal(err)
	}
	if updated == cfg || updated.Publish.MinScore != minScore || cfg.Publish.MinScore == minScore {
		t.Fatalf("runtime patch mutated original or was not applied")
	}
	invalid := 101.0
	if _, err := ApplyRuntimePatch(cfg, RuntimePatch{PublishMinScore: &invalid}); err == nil {
		t.Fatal("expected invalid runtime score")
	}
}
