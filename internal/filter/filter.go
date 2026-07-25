package filter

import (
	"sort"
	"strings"

	"github.com/local/node-hunter/internal/config"
	"github.com/local/node-hunter/internal/model"
)

// Apply 按延迟/存活/排序/数量筛选高质量节点
func Apply(nodes []*model.Node, cfg *config.Config) []*model.Node {
	out := make([]*model.Node, 0, len(nodes))
	maxLat := cfg.Filter.MaxLatencyMS
	if maxLat <= 0 {
		maxLat = 2500
	}

	minScore := cfg.Filter.MinScore
	if minScore < 0 {
		minScore = 0
	}
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if cfg.Filter.MinSuccess && !n.Alive {
			continue
		}
		if n.Alive && n.LatencyMS() > int64(maxLat) {
			continue
		}
		if minScore > 0 && n.Score > 0 && n.Score < minScore {
			// 已评分但低于阈值则丢弃；Score==0 可能未测，留给上层 AliveOnly 处理
			continue
		}
		out = append(out, n)
	}

	sortNodes(out, cfg)

	if cfg.Filter.MaxNodes > 0 && len(out) > cfg.Filter.MaxNodes {
		out = out[:cfg.Filter.MaxNodes]
	}
	return out
}

func sortNodes(nodes []*model.Node, cfg *config.Config) {
	by := strings.ToLower(cfg.Filter.SortBy)
	preferTLS := cfg.Filter.PreferTLS

	sort.SliceStable(nodes, func(i, j int) bool {
		a, b := nodes[i], nodes[j]

		if preferTLS && a.TLS != b.TLS {
			return a.TLS
		}

		switch by {
		case "score":
			if a.Score != b.Score {
				return a.Score > b.Score
			}
			return a.Latency < b.Latency
		case "protocol":
			if a.Protocol != b.Protocol {
				return a.Protocol < b.Protocol
			}
			return a.Latency < b.Latency
		case "name":
			if a.Name != b.Name {
				return a.Name < b.Name
			}
			return a.Latency < b.Latency
		default: // latency
			// 存活优先
			if a.Alive != b.Alive {
				return a.Alive
			}
			if a.Latency != b.Latency {
				return a.Latency < b.Latency
			}
			return a.Score > b.Score
		}
	})
}

// StatsByProtocol 按协议计数
func StatsByProtocol(nodes []*model.Node) map[string]int {
	m := make(map[string]int)
	for _, n := range nodes {
		m[string(n.Protocol)]++
	}
	return m
}
