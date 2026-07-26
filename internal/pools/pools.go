package pools

import (
	"strings"

	"github.com/GALIAIS/NodeHarvest/internal/config"
	"github.com/GALIAIS/NodeHarvest/internal/model"
	"github.com/GALIAIS/NodeHarvest/internal/store"
)

type Named struct {
	Key             string
	Title           string
	Description     string
	MinScore        float64
	MaxLatencyMS    int64
	Countries       map[string]bool
	Protocols       map[string]bool
	RequireAI       bool
	RequireVerified bool
	RefreshSec      int
	MaxNodes        int
}

func Configured(cfg *config.Config) []Named {
	if cfg == nil {
		cfg = config.Default()
	}
	out := make([]Named, 0, len(cfg.Pools))
	for _, source := range cfg.Pools {
		minScore := source.MinScore
		if minScore <= 0 {
			minScore = cfg.Filter.MinScore
		}
		if minScore <= 0 {
			minScore = 70
		}
		refresh := source.RefreshSec
		if refresh <= 0 {
			refresh = max(cfg.Publish.CacheSec, 60)
		}
		limit := source.MaxNodes
		if limit <= 0 {
			limit = cfg.Publish.MaxNodes
		}
		if limit <= 0 {
			limit = 500
		}
		out = append(out, Named{
			Key: strings.ToLower(strings.TrimSpace(source.Key)), Title: source.Name,
			Description: description(source), MinScore: minScore,
			MaxLatencyMS: int64(source.MaxLatencyMS), Countries: normalizedSet(source.Countries),
			Protocols: normalizedSet(source.Protocols), RequireAI: source.RequireAI,
			RequireVerified: source.RequireVerified, RefreshSec: refresh, MaxNodes: limit,
		})
	}
	return out
}

func Select(st *store.Store, pool Named) []*model.Node {
	if st == nil {
		return nil
	}
	candidates := st.ListNodes(store.NodeFilter{
		AliveOnly: true, MinScore: pool.MinScore, Limit: max(pool.MaxNodes*20, 5000),
	})
	out := make([]*model.Node, 0, min(len(candidates), pool.MaxNodes))
	for _, node := range candidates {
		if pool.RequireVerified && !node.Verified {
			continue
		}
		if pool.RequireAI && !hasAIOK(node) {
			continue
		}
		if pool.MaxLatencyMS > 0 && node.LatencyMS() > pool.MaxLatencyMS {
			continue
		}
		country := strings.ToUpper(first(node.Country, "XX"))
		protocol := normalizeProtocol(string(node.Protocol))
		if len(pool.Countries) > 0 && !pool.Countries[country] {
			continue
		}
		if len(pool.Protocols) > 0 && !pool.Protocols[protocol] {
			continue
		}
		out = append(out, node)
		if len(out) >= pool.MaxNodes {
			break
		}
	}
	return out
}

func Find(cfg *config.Config, key string) (Named, bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, pool := range Configured(cfg) {
		if pool.Key == key {
			return pool, true
		}
	}
	return Named{}, false
}

func hasAIOK(n *model.Node) bool {
	for _, result := range n.AIAccess {
		if result != nil && result.OK {
			return true
		}
	}
	return false
}

func normalizedSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.ToUpper(strings.TrimSpace(value))
		if len(value) > 2 {
			value = normalizeProtocol(value)
		}
		if value != "" {
			out[value] = true
		}
	}
	return out
}

func normalizeProtocol(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "hy2" {
		return "hysteria2"
	}
	return value
}

func first(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func description(pool config.PoolConfig) string {
	switch {
	case pool.RequireVerified:
		return "Passed real protocol validation"
	case pool.RequireAI:
		return "Passed at least one configured AI reachability probe"
	case pool.MaxLatencyMS > 0:
		return "Bounded latency and score selection"
	default:
		return "High-quality alive nodes"
	}
}
