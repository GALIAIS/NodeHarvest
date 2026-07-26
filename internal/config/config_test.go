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
