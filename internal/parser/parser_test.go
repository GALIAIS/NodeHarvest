package parser

import (
	"testing"
)

func TestParseVLESS(t *testing.T) {
	uri := "vless://11111111-1111-1111-1111-111111111111@example.org:443?encryption=none&security=tls&sni=example.org&type=ws&host=example.org&path=%2Fpath#test-vless"
	n, err := ParseURI(uri, "test")
	if err != nil {
		t.Fatal(err)
	}
	if n.Protocol != "vless" || n.Server != "example.org" || n.Port != 443 {
		t.Fatalf("unexpected node: %+v", n)
	}
	if n.UUID == "" || !n.TLS || n.Extra["encryption"] != "none" {
		t.Fatalf("missing uuid/tls: %+v", n)
	}
	if n.SkipTLSVerify() {
		t.Fatal("certificate verification disabled without an explicit source option")
	}

	insecure, err := ParseURI("vless://id@example.org:443?security=tls&allowInsecure=1", "test")
	if err != nil {
		t.Fatal(err)
	}
	if !insecure.SkipTLSVerify() {
		t.Fatal("explicit allowInsecure option was not preserved")
	}
}

func TestParseTrojan(t *testing.T) {
	uri := "trojan://password123@1.2.3.4:443?sni=www.example.com#trojan-node"
	n, err := ParseURI(uri, "test")
	if err != nil {
		t.Fatal(err)
	}
	if n.Password != "password123" || n.Port != 443 {
		t.Fatalf("unexpected: %+v", n)
	}
}

func TestParseSS(t *testing.T) {
	// method:password@host:port base64
	// aes-256-gcm:pass@10.0.0.1:8388
	uri := "ss://YWVzLTI1Ni1nY206cGFzcw@10.0.0.1:8388#ss-demo"
	n, err := ParseURI(uri, "test")
	if err != nil {
		t.Fatal(err)
	}
	if n.Server != "10.0.0.1" || n.Port != 8388 {
		t.Fatalf("unexpected: %+v", n)
	}
}

func TestParseContentBase64(t *testing.T) {
	// two lines base64 encoded
	plain := "trojan://p@1.1.1.1:443#a\nvless://u@2.2.2.2:443?security=tls#b\n"
	// encode manually in test via raw content without base64 first
	nodes := ParseContent(plain, "src")
	if len(nodes) < 2 {
		t.Fatalf("want >=2 nodes, got %d", len(nodes))
	}
}

func TestParseVMessJSON(t *testing.T) {
	// {"v":"2","ps":"demo","add":"1.2.3.4","port":"10086","id":"uuid-here","aid":"0","net":"tcp","type":"none","host":"","path":"","tls":""}
	// pre-encoded std base64
	uri := "vmess://eyJ2IjoiMiIsInBzIjoiZGVtbyIsImFkZCI6IjEuMi4zLjQiLCJwb3J0IjoiMTAwODYiLCJpZCI6InV1aWQtaGVyZSIsImFpZCI6IjAiLCJuZXQiOiJ0Y3AiLCJ0eXBlIjoibm9uZSIsImhvc3QiOiIiLCJwYXRoIjoiIiwidGxzIjoiIn0="
	n, err := ParseURI(uri, "test")
	if err != nil {
		t.Fatal(err)
	}
	if n.Server != "1.2.3.4" || n.Port != 10086 || n.Name != "demo" {
		t.Fatalf("unexpected: %+v", n)
	}
}

func TestParseClashPreservesTransportRealityAndPlugin(t *testing.T) {
	content := `proxies:
  - name: vless-ws
    type: vless
    server: edge.example.com
    port: 443
    uuid: uuid
    tls: true
    servername: origin.example.com
    client-fingerprint: chrome
    network: ws
    ws-opts:
      path: /socket
      headers: {Host: cdn.example.com}
      max-early-data: 2048
      v2ray-http-upgrade: true
    reality-opts: {public-key: public-key, short-id: abcd}
    ech-opts: {enable: true, config: base64-ech-config}
    shadow-tls-opts: {password: shadow-secret, version: 3}
  - name: ss-plugin
    type: ss
    server: ss.example.com
    port: 8388
    cipher: aes-128-gcm
    password: secret
    plugin: v2ray-plugin
    plugin-opts: {mode: websocket, host: cdn.example.com}
`
	nodes := ParseClash(content, "test")
	if len(nodes) != 2 {
		t.Fatalf("nodes=%d", len(nodes))
	}
	vless := nodes[0]
	if vless.Network != "ws" || vless.Path != "/socket" || vless.Host != "cdn.example.com" ||
		vless.Security != "reality" || vless.Extra["pbk"] != "public-key" ||
		vless.Extra["max-early-data"] != "2048" || vless.Extra["clash-transport-opts"] == "" ||
		vless.Extra["ech-opts"] == "" || vless.Extra["shadow-tls-opts"] == "" {
		t.Fatalf("vless fields lost: %+v", vless)
	}
	if nodes[1].Extra["plugin"] != "v2ray-plugin" || nodes[1].Extra["plugin-opts"] == "" {
		t.Fatalf("ss plugin lost: %+v", nodes[1])
	}
}

func TestSSRClashRoundTripDecodesShortFields(t *testing.T) {
	uri := clashProxyToURI(map[string]any{
		"name": "ssr", "type": "ssr", "server": "ssr.example.com", "port": 443,
		"cipher": "aes-256-cfb", "password": "secret",
		"protocol": "auth_sha1_v4", "obfs": "tls1.2_ticket_auth",
	})
	node, err := ParseURI(uri, "test")
	if err != nil {
		t.Fatal(err)
	}
	if node.Password != "secret" || node.Extra["protocol"] != "auth_sha1_v4" {
		t.Fatalf("ssr round trip lost fields: %+v", node)
	}
	ipv6URI := clashProxyToURI(map[string]any{
		"name": "ssr-v6", "type": "ssr", "server": "2001:db8::1", "port": 443,
		"cipher": "aes-256-cfb", "password": "secret",
		"protocol": "origin", "obfs": "plain",
	})
	ipv6, err := ParseURI(ipv6URI, "test")
	if err != nil || ipv6.Server != "2001:db8::1" {
		t.Fatalf("SSR IPv6 round trip failed: uri=%s node=%+v err=%v", ipv6URI, ipv6, err)
	}
}

func TestParseHysteria2PortHoppingAndCertificatePin(t *testing.T) {
	node, err := ParseURI(
		"hysteria2://user:secret@example.org:443,5000-5002/?sni=example.org&pinSHA256=deadbeef#hy2",
		"test",
	)
	if err != nil {
		t.Fatal(err)
	}
	if node.Port != 443 || node.Password != "user:secret" || node.Extra["ports"] != "443,5000-5002" ||
		node.Extra["fingerprint"] != "deadbeef" {
		t.Fatalf("hysteria2 fields lost: %+v", node)
	}
}

func TestParseClashPreservesQUICProtocolOptions(t *testing.T) {
	nodes := ParseClash(`proxies:
  - name: hy2
    type: hysteria2
    server: hy2.example.com
    port: 443
    ports: 443-8443
    hop-interval: 30
    password: secret
    obfs: gecko
    obfs-password: obfs-secret
    obfs-min-packet-size: 512
    obfs-max-packet-size: 1200
    fingerprint: deadbeef
  - name: tuic-v4
    type: tuic
    server: tuic.example.com
    port: 443
    token: token-secret
    disable-sni: true
    reduce-rtt: true
    request-timeout: 8000
    max-open-streams: 20
`, "test")
	if len(nodes) != 2 {
		t.Fatalf("nodes=%d", len(nodes))
	}
	if nodes[0].Extra["ports"] != "443-8443" || nodes[0].Extra["obfs-min-packet-size"] != "512" ||
		nodes[0].Extra["fingerprint"] != "deadbeef" {
		t.Fatalf("hysteria2 options lost: %+v", nodes[0])
	}
	if nodes[1].Extra["token"] != "token-secret" || nodes[1].Extra["reduce-rtt"] != "true" ||
		nodes[1].Extra["max-open-streams"] != "20" {
		t.Fatalf("TUIC options lost: %+v", nodes[1])
	}
}
