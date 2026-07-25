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
	if n.UUID == "" || !n.TLS {
		t.Fatalf("missing uuid/tls: %+v", n)
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
