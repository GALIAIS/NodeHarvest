package metrics

import (
	"strings"
	"testing"
)

func TestMetricsUseBoundedTokenLabelsAndHistograms(t *testing.T) {
	registry := New()
	registry.IncSubToken("base64", 200, "1234567890-secret")
	registry.ObserveSubLatency(.15)
	rendered := registry.Render()
	if strings.Contains(rendered, "secret") || !strings.Contains(rendered, `token_id="12345678"`) {
		t.Fatalf("unsafe token metric: %s", rendered)
	}
	if !strings.Contains(rendered, `nh_sub_latency_seconds_bucket{le="0.2"} 1`) {
		t.Fatalf("missing latency histogram: %s", rendered)
	}
}
