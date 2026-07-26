package quality

import (
	"testing"

	"github.com/GALIAIS/NodeHarvest/internal/model"
)

func TestComputeV2UsesHistoryAndRealHTTPMetrics(t *testing.T) {
	base := &model.Node{
		Protocol: model.ProtoVLESS, UUID: "id", Network: "ws", TLS: true, Alive: true,
		Quality: &model.Quality{SuccessRate: 1, AvgLatencyMS: 100, TLSOK: true, Stability7D: .95},
	}
	ApplyV2(base)
	withoutDial := base.Score
	base.Dial = &model.DialResult{
		OK: true, LatencyMS: 120, HTTPMS: 180, StatusCode: 200,
		DownloadBytes: 256 << 10, ThroughputBPS: 2 << 20,
	}
	ApplyV2(base)
	if base.Score <= withoutDial || base.Quality.Breakdown["throughput"] <= 50 {
		t.Fatalf("real metrics did not improve score: before=%v after=%v breakdown=%+v",
			withoutDial, base.Score, base.Quality.Breakdown)
	}

	unstable := *base
	qualityCopy := *base.Quality
	unstable.Quality = &qualityCopy
	unstable.Quality.Stability7D = .1
	ApplyV2(&unstable, Weights{Stability: 1})
	if unstable.Score >= base.Score {
		t.Fatalf("history weight ignored: stable=%v unstable=%v", base.Score, unstable.Score)
	}
}
