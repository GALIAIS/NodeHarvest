package quality

import (
	"math"

	"github.com/GALIAIS/NodeHarvest/internal/model"
)

// Weights controls the relative importance of measured score components.
type Weights struct {
	Latency    float64
	Success    float64
	Stability  float64
	TLS        float64
	HTTP       float64
	Throughput float64
}

func DefaultWeights() Weights {
	return Weights{Latency: .25, Success: .25, Stability: .15, TLS: .10, HTTP: .15, Throughput: .10}
}

type Breakdown struct {
	Connectivity float64 `json:"connectivity"`
	Latency      float64 `json:"latency"`
	Stability    float64 `json:"stability"`
	TLS          float64 `json:"tls"`
	HTTP         float64 `json:"http"`
	Throughput   float64 `json:"throughput"`
	Protocol     float64 `json:"protocol"`
	Total        float64 `json:"total"`
	Grade        string  `json:"grade"`
}

// ComputeV2 calculates an explainable score from active and historical measurements.
func ComputeV2(n *model.Node, configured ...Weights) Breakdown {
	var b Breakdown
	if n == nil {
		return b
	}
	if !n.Alive && (n.Dial == nil || !n.Dial.OK) {
		b.Grade = "F"
		return b
	}
	weights := DefaultWeights()
	if len(configured) > 0 && weightSum(configured[0]) > 0 {
		weights = configured[0]
	}
	q := n.Quality
	b.Connectivity = 80
	if q != nil {
		b.Connectivity = clamp(q.SuccessRate*100, 0, 100)
	}
	if n.Dial != nil {
		if n.Dial.OK {
			b.Connectivity = math.Max(b.Connectivity, 95)
		} else {
			b.Connectivity = math.Min(b.Connectivity, 20)
		}
	}

	latency := float64(n.LatencyMS())
	if q != nil && q.AvgLatencyMS > 0 {
		latency = float64(q.AvgLatencyMS)
	}
	if n.Dial != nil && n.Dial.LatencyMS > 0 {
		latency = float64(n.Dial.LatencyMS)
	}
	if latency <= 0 {
		latency = 500
	}
	b.Latency = clamp(100/(1+latency/400), 0, 100)

	b.Stability = b.Connectivity
	if q != nil && q.Stability7D > 0 {
		b.Stability = clamp(q.Stability7D*100, 0, 100)
	}

	b.TLS = 60
	if n.TLS {
		b.TLS = 40
		if q != nil && q.TLSOK {
			b.TLS = 100
		}
	} else {
		b.TLS = 80
	}

	b.HTTP = 50
	httpMS := int64(0)
	if q != nil {
		httpMS = q.HTTPMS
	}
	if n.Dial != nil {
		httpMS = n.Dial.HTTPMS
		if n.Dial.StatusCode >= 200 && n.Dial.StatusCode < 400 {
			b.HTTP = 100
		} else if n.Dial.StatusCode > 0 {
			b.HTTP = 10
		}
	}
	if httpMS > 0 {
		b.HTTP *= clamp(1/(1+float64(httpMS)/2000), .25, 1)
	}

	b.Throughput = 50
	throughput := int64(0)
	if q != nil {
		throughput = q.ThroughputBPS
	}
	if n.Dial != nil && n.Dial.ThroughputBPS > 0 {
		throughput = n.Dial.ThroughputBPS
	}
	if throughput > 0 {
		// 64 KiB/s is usable; 4 MiB/s reaches the component ceiling.
		b.Throughput = clamp(20+20*math.Log2(float64(throughput)/(64<<10)), 10, 100)
	}

	b.Protocol = protocolCompleteness(n)
	totalWeight := weightSum(weights)
	b.Total = (b.Latency*weights.Latency +
		b.Connectivity*weights.Success +
		b.Stability*weights.Stability +
		b.TLS*weights.TLS +
		b.HTTP*weights.HTTP +
		b.Throughput*weights.Throughput) / totalWeight
	// Credentials/transport completeness is a bounded multiplier, not a hidden seventh weight.
	b.Total = clamp(b.Total*(.85+.15*b.Protocol/100), 0, 100)
	b.Grade = model.AssignGrade(b.Total)
	return b
}

func ApplyV2(n *model.Node, configured ...Weights) {
	b := ComputeV2(n, configured...)
	n.Score = math.Round(b.Total*10) / 10
	n.Grade = b.Grade
	if n.Quality == nil {
		n.Quality = &model.Quality{ScoreVersion: "v2"}
	}
	n.Quality.ScoreVersion = "v2"
	n.Quality.Score = n.Score
	n.Quality.Breakdown = map[string]float64{
		"success": b.Connectivity, "latency": b.Latency, "stability": b.Stability,
		"tls": b.TLS, "http": b.HTTP, "throughput": b.Throughput, "protocol": b.Protocol,
	}
	if !contains(n.Quality.Notes, "v2 weighted scoring applied") {
		n.Quality.Notes = append(n.Quality.Notes, "v2 weighted scoring applied")
	}
}

func protocolCompleteness(n *model.Node) float64 {
	switch n.Protocol {
	case model.ProtoVLESS, model.ProtoVMess:
		if n.UUID != "" && (n.Network != "" || n.TLS || n.Security != "") {
			return 100
		}
		if n.UUID != "" {
			return 85
		}
	case model.ProtoTrojan, model.ProtoHysteria2:
		if n.Password != "" {
			return 95
		}
	case model.ProtoSS:
		if n.Method != "" && n.Password != "" {
			return 90
		}
	}
	return 70
}

func weightSum(w Weights) float64 {
	return w.Latency + w.Success + w.Stability + w.TLS + w.HTTP + w.Throughput
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
