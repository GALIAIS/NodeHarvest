package quality

import (
	"math"
	"time"

	"github.com/local/node-hunter/internal/model"
)

// ScoreV2 可解释多因子评分（企业级 v2）
// 在既有 Quality 上叠加稳定性/协议完整度启发。
type Breakdown struct {
	Connectivity float64 `json:"connectivity"` // 0-100
	Latency      float64 `json:"latency"`
	Jitter       float64 `json:"jitter"`
	TLS          float64 `json:"tls"`
	Protocol     float64 `json:"protocol"`
	Total        float64 `json:"total"`
	Grade        string  `json:"grade"`
}

// ComputeV2 根据已有 quality 轮次结果计算 v2 分
func ComputeV2(n *model.Node) Breakdown {
	var b Breakdown
	if n == nil {
		return b
	}
	if !n.Alive {
		b.Total = 0
		b.Grade = "F"
		return b
	}
	q := n.Quality
	// connectivity
	if q != nil {
		b.Connectivity = clamp(q.SuccessRate*100, 0, 100)
	} else {
		b.Connectivity = 80
	}
	// latency: 50ms→100, 2000ms→20
	lat := float64(n.LatencyMS())
	if q != nil && q.AvgLatencyMS > 0 {
		lat = float64(q.AvgLatencyMS)
	}
	if lat <= 0 {
		lat = 500
	}
	b.Latency = clamp(100.0*(1.0/(1.0+lat/400.0)), 0, 100)
	// jitter
	if q != nil && q.JitterMS > 0 {
		b.Jitter = clamp(100.0*(1.0/(1.0+float64(q.JitterMS)/80.0)), 0, 100)
	} else {
		b.Jitter = 70
	}
	// tls
	if q != nil && q.TLSOK {
		b.TLS = 100
	} else if n.TLS {
		b.TLS = 40
	} else {
		b.TLS = 60
	}
	// protocol completeness
	b.Protocol = 50
	switch n.Protocol {
	case model.ProtoVLESS, model.ProtoVMess, model.ProtoTrojan, model.ProtoHysteria2:
		b.Protocol = 80
		if n.UUID != "" || n.Password != "" {
			b.Protocol = 90
		}
		if n.TLS || n.Security == "reality" || n.Security == "tls" {
			b.Protocol = 95
		}
	case model.ProtoSS:
		b.Protocol = 75
		if n.Method != "" && n.Password != "" {
			b.Protocol = 85
		}
	}
	// weights
	b.Total = clamp(
		b.Connectivity*0.30+
			b.Latency*0.30+
			b.Jitter*0.10+
			b.TLS*0.15+
			b.Protocol*0.15,
		0, 100,
	)
	b.Grade = model.AssignGrade(b.Total)
	return b
}

// ApplyV2 写回 node.Score/Grade，并在 notes 中记录
func ApplyV2(n *model.Node) {
	b := ComputeV2(n)
	n.Score = math.Round(b.Total*10) / 10
	n.Grade = b.Grade
	if n.Quality != nil {
		n.Quality.Score = n.Score
		n.Quality.Notes = append(n.Quality.Notes,
			"v2 scoring applied",
		)
	}
	_ = time.Now()
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
