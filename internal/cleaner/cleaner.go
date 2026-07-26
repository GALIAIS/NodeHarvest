package cleaner

import (
	"net"
	"strings"

	"github.com/GALIAIS/NodeHarvest/internal/config"
	"github.com/GALIAIS/NodeHarvest/internal/model"
)

// Clean 校验 + 协议过滤 + 去重，保留更完整的一条
func Clean(nodes []*model.Node, cfg *config.Config) []*model.Node {
	best := make(map[string]*model.Node, len(nodes))
	priorities := make(map[string]int, len(cfg.Sources))
	for _, source := range cfg.Sources {
		priorities[source.Name] = source.Priority
	}

	for _, n := range nodes {
		if n == nil {
			continue
		}
		normalize(n)

		if cfg.Filter.DropInvalid && !n.IsValid() {
			continue
		}
		if !cfg.ProtocolAllowed(string(n.Protocol)) {
			continue
		}
		// 过滤明显无效主机
		if isBadHost(n.Server) {
			continue
		}

		key := n.Key()
		if cfg.Filter.CollapseSameIPPorts && net.ParseIP(n.Server) != nil {
			endpoint := *n
			endpoint.Port = 0
			key = endpoint.Key()
		}
		if old, ok := best[key]; ok {
			sources := mergeSources(old.Sources, n.Sources)
			if scoreMeta(n) > scoreMeta(old) ||
				(scoreMeta(n) == scoreMeta(old) && priorities[n.Source] > priorities[old.Source]) {
				n.Sources = sources
				best[key] = n
			} else {
				old.Sources = sources
			}
			continue
		}
		best[key] = n
	}

	out := make([]*model.Node, 0, len(best))
	for _, n := range best {
		out = append(out, n)
	}
	return out
}

func normalize(n *model.Node) {
	n.Server = strings.TrimSpace(n.Server)
	n.Name = strings.TrimSpace(n.Name)
	n.UUID = strings.TrimSpace(n.UUID)
	n.Password = strings.TrimSpace(n.Password)
	n.Method = strings.TrimSpace(n.Method)
	n.Network = strings.ToLower(strings.TrimSpace(n.Network))
	n.Security = strings.ToLower(strings.TrimSpace(n.Security))
	n.SNI = strings.TrimSpace(n.SNI)
	n.Host = strings.TrimSpace(n.Host)
	n.Path = strings.TrimSpace(n.Path)

	// hy2 统一
	if n.Protocol == "hy2" {
		n.Protocol = model.ProtoHysteria2
	}
	if n.Name == "" {
		n.Name = n.Address()
	}
	// 清理名称中的控制字符
	n.Name = strings.Map(func(r rune) rune {
		if r < 32 {
			return -1
		}
		return r
	}, n.Name)
	if n.Source != "" {
		n.Sources = mergeSources(n.Sources, []string{n.Source})
	}
	n.Fingerprint = n.Key()
}

func mergeSources(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, source := range append(a, b...) {
		if source != "" && !seen[source] {
			seen[source] = true
			out = append(out, source)
		}
	}
	return out
}

func isBadHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "" || h == "localhost" || h == "0.0.0.0" || h == "::" || h == "example.com" {
		return true
	}
	// 私有/保留地址通常不是公开节点
	if ip := net.ParseIP(h); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return true
		}
	}
	return false
}

// scoreMeta 元数据完整度，用于去重时择优
func scoreMeta(n *model.Node) int {
	s := 0
	if n.Name != "" {
		s++
	}
	if n.TLS {
		s++
	}
	if n.SNI != "" {
		s++
	}
	if n.UUID != "" || n.Password != "" {
		s++
	}
	if n.Network != "" {
		s++
	}
	if n.RawURI != "" {
		s++
	}
	return s
}
