package pools

import (
	"strings"

	"github.com/local/node-hunter/internal/model"
	"github.com/local/node-hunter/internal/store"
)

// Named 企业多订阅池
type Named struct {
	Key         string
	Title       string
	Description string
	Filter      store.NodeFilter
}

func Defaults(minScore float64) []Named {
	if minScore <= 0 {
		minScore = 70
	}
	return []Named{
		{
			Key: "global-hq", Title: "Global HQ", Description: "High quality alive nodes (TCP/TLS screen)",
			Filter: store.NodeFilter{AliveOnly: true, HighQuality: true, MinScore: minScore, Limit: 500},
		},
		{
			Key: "verified", Title: "Verified", Description: "Passed real protocol dial (sing-box)",
			Filter: store.NodeFilter{AliveOnly: true, VerifiedOnly: true, Limit: 300},
		},
		{
			Key: "ai-friendly", Title: "AI Friendly", Description: "Nodes with any AI probe pass",
			Filter: store.NodeFilter{AliveOnly: true, MinScore: 60, Limit: 300},
		},
		{
			Key: "low-latency", Title: "Low Latency", Description: "Alive sorted by score (prefer low latency via score)",
			Filter: store.NodeFilter{AliveOnly: true, MinScore: 50, Limit: 200},
		},
	}
}

// Select 应用池过滤；ai-friendly 额外检查 AIAccess
func Select(st *store.Store, pool Named) []*model.Node {
	nodes := st.ListNodes(pool.Filter)
	if pool.Key == "ai-friendly" {
		out := make([]*model.Node, 0, len(nodes))
		for _, n := range nodes {
			if hasAIOK(n) {
				out = append(out, n)
			}
		}
		return out
	}
	return nodes
}

func hasAIOK(n *model.Node) bool {
	for _, r := range n.AIAccess {
		if r != nil && r.OK {
			return true
		}
	}
	return false
}

func Find(key string, minScore float64) (Named, bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, p := range Defaults(minScore) {
		if p.Key == key {
			return p, true
		}
	}
	return Named{}, false
}
