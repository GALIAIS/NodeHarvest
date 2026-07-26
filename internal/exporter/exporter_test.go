package exporter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GALIAIS/NodeHarvest/internal/model"
	"gopkg.in/yaml.v3"
)

func TestCleanupOldRunsKeepsNewestFiles(t *testing.T) {
	dir := t.TempDir()
	for i, name := range []string{
		"nodes-20260101-000000.txt",
		"nodes-20260102-000000.txt",
		"nodes-20260103-000000.txt",
		"nodes-latest.txt",
		"nodes-ai-friendly.txt",
		"sub.txt",
	} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		at := time.Unix(int64(i+1), 0)
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}

	cleanupOldRuns(dir, "nodes", 2)

	for _, name := range []string{
		"nodes-20260102-000000.txt",
		"nodes-20260103-000000.txt",
		"nodes-latest.txt",
		"nodes-ai-friendly.txt",
		"sub.txt",
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s was removed: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "nodes-20260101-000000.txt")); !os.IsNotExist(err) {
		t.Errorf("oldest file still exists: %v", err)
	}
}

func TestRenderClashQuotesUntrustedScalars(t *testing.T) {
	body := RenderClash([]*model.Node{{
		Protocol: model.ProtoSS,
		Name:     "name\n  - injected",
		Server:   "example.com",
		Port:     443,
		Method:   "aes-128-gcm",
		Password: "secret\"\n  - name: injected",
	}})
	var parsed struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	if err := yaml.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Proxies) != 1 {
		t.Fatalf("unexpected proxies: %s", body)
	}
}

func TestRenderClashKeepsTLSVerificationByDefault(t *testing.T) {
	secure := RenderClash([]*model.Node{{
		Protocol: model.ProtoTrojan, Name: "secure", Server: "example.com", Port: 443, Password: "secret",
	}})
	if strings.Contains(secure, "skip-cert-verify") {
		t.Fatalf("certificate verification disabled by default: %s", secure)
	}
	insecure := RenderClash([]*model.Node{{
		Protocol: model.ProtoTrojan, Name: "legacy", Server: "example.com", Port: 443, Password: "secret",
		Extra: map[string]string{"allowInsecure": "true"},
	}})
	if !strings.Contains(insecure, "skip-cert-verify: true") {
		t.Fatalf("explicit insecure option missing: %s", insecure)
	}
	insecureVLESS := RenderClash([]*model.Node{{
		Protocol: model.ProtoVLESS, Name: "legacy-vless", Server: "example.com", Port: 443,
		UUID: "id", TLS: true, Extra: map[string]string{"insecure": "1"},
	}})
	if !strings.Contains(insecureVLESS, "skip-cert-verify: true") {
		t.Fatalf("explicit VLESS insecure option missing: %s", insecureVLESS)
	}
}
