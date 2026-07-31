package exporter

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
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

func TestRenderClashMakesProxyNamesUnique(t *testing.T) {
	nodes := []*model.Node{
		{Protocol: model.ProtoSS, Name: "duplicate", Server: "one.example.com", Port: 443, Method: "aes-128-gcm", Password: "secret"},
		{Protocol: model.ProtoSS, Name: "duplicate", Server: "two.example.com", Port: 443, Method: "aes-128-gcm", Password: "secret"},
		{Protocol: model.ProtoSS, Name: "duplicate #2", Server: "three.example.com", Port: 443, Method: "aes-128-gcm", Password: "secret"},
		{Protocol: model.ProtoSS, Name: "duplicate", Server: "four.example.com", Port: 443, Method: "aes-128-gcm", Password: "secret"},
		{Protocol: model.ProtoSS, Name: "NodeHarvest", Server: "five.example.com", Port: 443, Method: "aes-128-gcm", Password: "secret"},
		{Protocol: model.ProtoSS, Name: "NodeHarvest #2", Server: "six.example.com", Port: 443, Method: "aes-128-gcm", Password: "secret"},
		{Protocol: model.ProtoSS, Name: "REJECT", Server: "seven.example.com", Port: 443, Method: "aes-128-gcm", Password: "secret"},
	}
	var parsed struct {
		Proxies []struct {
			Name string `yaml:"name"`
		} `yaml:"proxies"`
		ProxyGroups []struct {
			Name    string   `yaml:"name"`
			Proxies []string `yaml:"proxies"`
		} `yaml:"proxy-groups"`
		Rules []string `yaml:"rules"`
	}
	if err := yaml.Unmarshal([]byte(RenderClash(nodes)), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Proxies) != len(nodes) {
		t.Fatalf("got %d proxies, want %d", len(parsed.Proxies), len(nodes))
	}
	names := make(map[string]struct{}, len(parsed.Proxies))
	for _, proxy := range parsed.Proxies {
		if proxy.Name == "REJECT" {
			t.Fatal("reserved proxy name was not rewritten")
		}
		if _, exists := names[proxy.Name]; exists {
			t.Fatalf("duplicate proxy name %q", proxy.Name)
		}
		names[proxy.Name] = struct{}{}
	}
	if len(parsed.ProxyGroups) != 1 || len(parsed.ProxyGroups[0].Proxies) != len(nodes) || len(parsed.Rules) != 1 {
		t.Fatalf("profile is not directly importable: %s", RenderClash(nodes))
	}
	if _, collision := names[parsed.ProxyGroups[0].Name]; collision {
		t.Fatalf("proxy group collides with proxy name %q", parsed.ProxyGroups[0].Name)
	}
}

func TestRenderRawMakesURIProxyNamesUnique(t *testing.T) {
	vmessURI := func(server string) string {
		body, err := json.Marshal(map[string]any{
			"v": "2", "ps": "vmess", "add": server, "port": "443",
			"id": "uuid", "aid": "0", "net": "tcp",
		})
		if err != nil {
			t.Fatal(err)
		}
		return "vmess://" + base64.StdEncoding.EncodeToString(body)
	}
	ssrURI := func(server string) string {
		password := base64.RawURLEncoding.EncodeToString([]byte("secret"))
		remarks := base64.RawURLEncoding.EncodeToString([]byte("ssr"))
		body := server + ":443:origin:aes-256-cfb:plain:" + password + "/?remarks=" + remarks
		return "ssr://" + base64.RawURLEncoding.EncodeToString([]byte(body))
	}
	nodes := []*model.Node{
		{Protocol: model.ProtoVLESS, Name: "uri", RawURI: "vless://uuid@one.example:443#uri"},
		{Protocol: model.ProtoVLESS, Name: "uri", RawURI: "vless://uuid@two.example:443#uri"},
		{Protocol: model.ProtoVMess, Name: "vmess", RawURI: vmessURI("one.example")},
		{Protocol: model.ProtoVMess, Name: "vmess", RawURI: vmessURI("two.example")},
		{Protocol: model.ProtoSSR, Name: "ssr", RawURI: ssrURI("one.example")},
		{Protocol: model.ProtoSSR, Name: "ssr", RawURI: ssrURI("two.example")},
		{Protocol: model.ProtoHysteria2, Name: "hy2", RawURI: "hysteria2://secret@one.example:443,5000-5002/#hy2"},
		{Protocol: model.ProtoHysteria2, Name: "hy2", RawURI: "hysteria2://secret@two.example:443,5000-5002/#hy2"},
		{Protocol: model.ProtoVLESS, Name: "REJECT", RawURI: "vless://uuid@reserved.example:443#REJECT"},
	}
	lines := strings.Split(strings.TrimSpace(RenderRaw(nodes)), "\n")
	if len(lines) != len(nodes) {
		t.Fatalf("lines=%v", lines)
	}
	first, _ := url.Parse(lines[0])
	second, _ := url.Parse(lines[1])
	if first.Fragment != "uri" || second.Fragment != "uri #2" {
		t.Fatalf("URI names are not unique: %q, %q", first.Fragment, second.Fragment)
	}
	for i, want := range []string{"vmess", "vmess #2"} {
		payload, ok := decodeBase64Payload(strings.TrimPrefix(lines[i+2], "vmess://"))
		var config map[string]any
		if !ok || json.Unmarshal(payload, &config) != nil || config["ps"] != want {
			t.Fatalf("VMess name %d was not rewritten: %s", i, lines[i+2])
		}
	}
	for i, want := range []string{"ssr", "ssr #2"} {
		payload, ok := decodeBase64Payload(strings.TrimPrefix(lines[i+4], "ssr://"))
		_, query, _ := strings.Cut(string(payload), "/?")
		values, err := url.ParseQuery(query)
		remarks, decoded := decodeBase64Payload(values.Get("remarks"))
		if !ok || err != nil || !decoded || string(remarks) != want {
			t.Fatalf("SSR name %d was not rewritten: %s", i, lines[i+4])
		}
	}
	if !strings.HasSuffix(lines[7], "#hy2%20%232") {
		t.Fatalf("Hysteria2 multi-port name was not rewritten: %s", lines[7])
	}
	if !strings.HasSuffix(lines[8], "#REJECT%20%232") {
		t.Fatalf("reserved URI proxy name was not rewritten: %s", lines[8])
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

func TestBuildClashProxyPreservesVLESSRealityWebSocket(t *testing.T) {
	proxy, err := BuildClashProxy(&model.Node{
		Protocol: model.ProtoVLESS, Server: "edge.example.com", Port: 443, UUID: "uuid",
		Network: "ws", TLS: true, Security: "reality", SNI: "origin.example.com",
		Path: "/socket", Host: "cdn.example.com", ALPN: "h2,http/1.1",
		Extra: map[string]string{
			"fp": "chrome", "pbk": "public-key", "sid": "abcd", "encryption": "none",
			"max-early-data": "2048", "early-data-header-name": "Sec-WebSocket-Protocol",
			"clash-transport-opts": `{"headers":{"X-Test":"value"},"v2ray-http-upgrade":true}`,
		},
	}, "full-fields")
	if err != nil {
		t.Fatal(err)
	}
	if proxy["network"] != "ws" || proxy["client-fingerprint"] != "chrome" || proxy["encryption"] != "none" ||
		proxy["servername"] != "origin.example.com" {
		t.Fatalf("common fields lost: %+v", proxy)
	}
	ws := proxy["ws-opts"].(map[string]any)
	if ws["path"] != "/socket" || ws["max-early-data"] != 2048 {
		t.Fatalf("ws fields lost: %+v", ws)
	}
	headers := ws["headers"].(map[string]any)
	if headers["Host"] != "cdn.example.com" || headers["X-Test"] != "value" || ws["v2ray-http-upgrade"] != true {
		t.Fatalf("unknown ws options lost: %+v", ws)
	}
	reality := proxy["reality-opts"].(map[string]any)
	if reality["public-key"] != "public-key" || reality["short-id"] != "abcd" {
		t.Fatalf("reality fields lost: %+v", reality)
	}
}

func TestBuildClashProxyPreservesHysteria2TLSOptions(t *testing.T) {
	proxy, err := BuildClashProxy(&model.Node{
		Protocol: model.ProtoHysteria2, Server: "hy2.example.com", Port: 443, Password: "secret",
		Extra: map[string]string{
			"fp": "chrome", "fingerprint": "certificate-pin", "ech": "base64-ech-config",
			"shadow-tls-opts": `{"password":"shadow-secret","version":3}`,
		},
	}, "hy2")
	if err != nil {
		t.Fatal(err)
	}
	ech := proxy["ech-opts"].(map[string]any)
	shadowTLS := proxy["shadow-tls-opts"].(map[string]any)
	if proxy["client-fingerprint"] != nil || proxy["fingerprint"] != "certificate-pin" ||
		ech["enable"] != true || ech["config"] != "base64-ech-config" ||
		shadowTLS["password"] != "shadow-secret" || shadowTLS["version"] != 3 {
		t.Fatalf("hysteria2 TLS options lost or invalid: %+v", proxy)
	}
}

func TestRenderClashIncludesTUICAndSSR(t *testing.T) {
	body := RenderClash([]*model.Node{
		{Protocol: model.ProtoTUIC, Name: "tuic", Server: "tuic.example.com", Port: 443, UUID: "uuid", Password: "secret"},
		{Protocol: model.ProtoSSR, Name: "ssr", Server: "ssr.example.com", Port: 443, Method: "aes-256-cfb", Password: "secret",
			Extra: map[string]string{"protocol": "auth_sha1_v4", "obfs": "tls1.2_ticket_auth"}},
	})
	var parsed struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	if err := yaml.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Proxies) != 2 || parsed.Proxies[0]["type"] != "tuic" || parsed.Proxies[1]["type"] != "ssr" {
		t.Fatalf("protocols omitted: %s", body)
	}
}

func TestBuildClashProxyPreservesQUICProtocolOptions(t *testing.T) {
	hy2, err := BuildClashProxy(&model.Node{
		Protocol: model.ProtoHysteria2, Server: "hy2.example.com", Port: 443, Password: "secret",
		Extra: map[string]string{
			"ports": "443-8443", "hop-interval": "30", "obfs": "gecko",
			"obfs-min-packet-size": "512", "fingerprint": "deadbeef",
		},
	}, "hy2")
	if err != nil {
		t.Fatal(err)
	}
	if hy2["ports"] != "443-8443" || hy2["hop-interval"] != "30" ||
		hy2["obfs-min-packet-size"] != 512 || hy2["fingerprint"] != "deadbeef" {
		t.Fatalf("hysteria2 options lost: %+v", hy2)
	}

	tuic, err := BuildClashProxy(&model.Node{
		Protocol: model.ProtoTUIC, Server: "tuic.example.com", Port: 443,
		Extra: map[string]string{
			"token": "token-secret", "disable-sni": "true", "reduce-rtt": "true",
			"request-timeout": "8000", "max-open-streams": "20",
		},
	}, "tuic-v4")
	if err != nil {
		t.Fatal(err)
	}
	if tuic["token"] != "token-secret" || tuic["disable-sni"] != true ||
		tuic["request-timeout"] != 8000 || tuic["max-open-streams"] != 20 {
		t.Fatalf("TUIC options lost: %+v", tuic)
	}
}
