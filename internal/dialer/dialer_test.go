package dialer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/GALIAIS/NodeHarvest/internal/exporter"
	"github.com/GALIAIS/NodeHarvest/internal/model"
	"gopkg.in/yaml.v3"
)

func TestMeasureHTTPAndXrayOutbound(t *testing.T) {
	payload := make([]byte, 32<<10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := measureHTTP(server.Client(), req, 8<<10)
	if err != nil {
		t.Fatal(err)
	}
	if got.StatusCode != http.StatusOK || got.Bytes != 8<<10 || got.ThroughputBPS <= 0 {
		t.Fatalf("measurement=%+v", got)
	}

	outbound, err := BuildXrayOutbound(&model.Node{
		Protocol: model.ProtoVLESS, Server: "example.test", Port: 443, UUID: "id",
		Network: "ws", TLS: true, SNI: "cdn.example.test", Path: "/socket",
	})
	if err != nil {
		t.Fatal(err)
	}
	if outbound["protocol"] != "vless" || outbound["streamSettings"] == nil {
		t.Fatalf("xray outbound=%+v", outbound)
	}
	stream := outbound["streamSettings"].(map[string]any)
	tlsSettings := stream["tlsSettings"].(map[string]any)
	if _, exists := tlsSettings["allowInsecure"]; exists {
		t.Fatalf("certificate verification disabled by default: %+v", tlsSettings)
	}

	insecureOutbound, err := BuildOutbound(&model.Node{
		Protocol: model.ProtoTrojan, Server: "example.test", Port: 443, Password: "secret",
		Extra: map[string]string{"insecure": "true"},
	})
	if err != nil {
		t.Fatal(err)
	}
	tls := insecureOutbound["tls"].(map[string]any)
	if tls["insecure"] != true {
		t.Fatalf("explicit insecure option ignored: %+v", tls)
	}
}

func TestMihomoConfigUsesExportedProxyObject(t *testing.T) {
	d := &Dialer{engine: "mihomo"}
	raw, err := d.coreConfig(&model.Node{
		Protocol: model.ProtoVLESS, Name: "node", Server: "edge.example.com", Port: 443, UUID: "uuid",
		Network: "grpc", TLS: true, SNI: "origin.example.com",
		Extra: map[string]string{"serviceName": "service", "fp": "chrome"},
	}, 19000, "")
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	proxies := cfg["proxies"].([]any)
	proxyConfig := proxies[0].(map[string]any)
	if proxyConfig["network"] != "grpc" || proxyConfig["client-fingerprint"] != "chrome" {
		t.Fatalf("mihomo proxy fields lost: %s", raw)
	}
	if cfg["mixed-port"] != 19000 {
		t.Fatalf("mixed port missing: %s", raw)
	}
}

func TestCanceledDialMarksFailureAndReportsProgress(t *testing.T) {
	progress := 0
	d := &Dialer{
		engine: "mihomo",
		opts: Options{Concurrency: 1, OnProgress: func(done, _ int) {
			progress = done
		}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	node := &model.Node{Tags: []string{"verified"}}
	d.TestAll(ctx, []*model.Node{node})
	if node.Dial == nil || node.Dial.Error != "canceled" || node.Verified ||
		len(node.Tags) != 1 || node.Tags[0] != "dial-fail" || progress != 1 {
		t.Fatalf("canceled result=%+v tags=%v progress=%d", node.Dial, node.Tags, progress)
	}
}

func TestSingBoxHysteria2PortHopping(t *testing.T) {
	outbound, err := BuildOutbound(&model.Node{
		Protocol: model.ProtoHysteria2, Server: "hy2.example.com", Port: 443, Password: "secret",
		Extra: map[string]string{"ports": "443,5000-5002", "hop-interval": "30"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := outbound["server_port"]; exists {
		t.Fatalf("server_port conflicts with port hopping: %+v", outbound)
	}
	ports, ok := outbound["server_ports"].([]string)
	if !ok || len(ports) != 2 || ports[0] != "443" || ports[1] != "5000:5002" ||
		outbound["hop_interval"] != "30s" {
		t.Fatalf("port hopping fields lost: %+v", outbound)
	}
}

func TestMihomoGeneratedConfigIntegration(t *testing.T) {
	bin := os.Getenv("MIHOMO_BIN")
	if bin == "" {
		t.Skip("MIHOMO_BIN is not set")
	}
	dir := t.TempDir()
	d := &Dialer{engine: "mihomo"}
	raw, err := d.coreConfig(&model.Node{
		Protocol: model.ProtoVLESS, Name: "node", Server: "edge.example.com", Port: 443,
		UUID: "11111111-1111-1111-1111-111111111111", Network: "ws", TLS: true,
		SNI: "origin.example.com", Path: "/socket", Host: "cdn.example.com",
		Extra: map[string]string{"fp": "chrome"},
	}, 19000, "")
	if err != nil {
		t.Fatal(err)
	}
	configs := map[string][]byte{
		"dial": raw,
		"subscription": []byte(exporter.RenderClash([]*model.Node{
			{
				Protocol: model.ProtoVLESS, Name: "vless-ws", Server: "edge.example.com", Port: 443,
				UUID: "11111111-1111-1111-1111-111111111111", Network: "ws", TLS: true,
				SNI: "origin.example.com", Path: "/socket", Host: "cdn.example.com",
				Extra: map[string]string{"fp": "chrome"},
			},
			{
				Protocol: model.ProtoTrojan, Name: "trojan-grpc", Server: "trojan.example.com", Port: 443,
				Password: "secret", Network: "grpc", SNI: "trojan.example.com",
				Extra: map[string]string{"serviceName": "service"},
			},
			{
				Protocol: model.ProtoHysteria2, Name: "hy2", Server: "hy2.example.com", Port: 443,
				Password: "secret", SNI: "hy2.example.com",
				Extra: map[string]string{
					"ports": "443-445", "hop-interval": "30",
					"obfs": "salamander", "obfs-password": "obfs-secret",
				},
			},
			{
				Protocol: model.ProtoTUIC, Name: "tuic", Server: "tuic.example.com", Port: 443,
				UUID: "22222222-2222-2222-2222-222222222222", Password: "secret", SNI: "tuic.example.com",
			},
			{
				Protocol: model.ProtoTUIC, Name: "tuic-v4", Server: "tuic-v4.example.com", Port: 443,
				Extra: map[string]string{"token": "token-secret", "reduce-rtt": "true"},
			},
			{
				Protocol: model.ProtoSSR, Name: "ssr", Server: "ssr.example.com", Port: 443,
				Method: "aes-256-cfb", Password: "secret",
				Extra: map[string]string{"protocol": "auth_sha1_v4", "obfs": "tls1.2_ticket_auth"},
			},
		})),
	}
	for name, body := range configs {
		path := filepath.Join(dir, name+".yaml")
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		stateDir := filepath.Join(dir, name)
		if err := os.MkdirAll(stateDir, 0o700); err != nil {
			t.Fatal(err)
		}
		// #nosec G204 -- MIHOMO_BIN is an explicit developer-controlled integration-test input.
		if output, err := exec.Command(bin, "-t", "-d", stateDir, "-f", path).CombinedOutput(); err != nil {
			t.Fatalf("mihomo rejected %s config: %v\n%s", name, err, output)
		}
	}
}
