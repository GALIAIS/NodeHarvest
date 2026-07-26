package dialer

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GALIAIS/NodeHarvest/internal/model"
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
