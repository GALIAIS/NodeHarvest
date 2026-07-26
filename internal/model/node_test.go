package model

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNodeJSONLatencyUnits(t *testing.T) {
	in := Node{Latency: 125 * time.Millisecond}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["latency_ms"] != float64(125) {
		t.Fatalf("latency_ms=%v json=%s", raw["latency_ms"], data)
	}

	var out Node
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out.Latency != in.Latency {
		t.Fatalf("round trip latency=%s", out.Latency)
	}

	var legacy Node
	if err := json.Unmarshal([]byte(`{"latency_ms":10000000}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Latency != 10*time.Millisecond {
		t.Fatalf("legacy latency=%s", legacy.Latency)
	}
}

func TestNodeKeyPreservesCaseSensitiveCredentialsAndExtra(t *testing.T) {
	a := Node{
		Protocol: ProtoSS, Server: "EXAMPLE.com", Port: 443, Password: "Secret",
		Extra: map[string]string{"obfs": "http"},
	}
	b := a
	b.Password = "secret"
	if a.Key() == b.Key() {
		t.Fatal("case-sensitive passwords produced the same key")
	}
	b = a
	b.Extra = map[string]string{"obfs": "tls"}
	if a.Key() == b.Key() {
		t.Fatal("connection options produced the same key")
	}
	if strings.Contains(a.Key(), a.Password) {
		t.Fatal("key exposes credentials")
	}
}

func TestNodeSkipTLSVerifyRequiresExplicitOptIn(t *testing.T) {
	for _, node := range []*Node{
		nil,
		{},
		{Extra: map[string]string{"insecure": "false"}},
		{Extra: map[string]string{"allowInsecure": "0"}},
	} {
		if node.SkipTLSVerify() {
			t.Fatalf("unexpected insecure TLS for %#v", node)
		}
	}
	for _, extra := range []map[string]string{
		{"insecure": "true"},
		{"allowInsecure": "1"},
		{"skip-cert-verify": "YES"},
	} {
		if !(&Node{Extra: extra}).SkipTLSVerify() {
			t.Fatalf("explicit insecure TLS ignored for %#v", extra)
		}
	}
}
