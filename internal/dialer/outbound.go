package dialer

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/GALIAIS/NodeHarvest/internal/model"
)

// BuildOutbound 将 Node 转为 sing-box outbound 对象（map，便于 JSON）
// 支持：ss / trojan / vmess / vless / hysteria2（尽力）；不支持则返回 error
func BuildOutbound(n *model.Node) (map[string]any, error) {
	if n == nil || !n.IsValid() {
		return nil, fmt.Errorf("invalid node")
	}
	extra := n.Extra
	if extra == nil {
		extra = map[string]string{}
	}
	switch n.Protocol {
	case model.ProtoSS:
		if n.Extra["plugin"] != "" {
			return nil, fmt.Errorf("sing-box external ss plugins are not bundled")
		}
		method := n.Method
		if method == "" {
			method = "aes-256-gcm"
		}
		if n.Password == "" {
			return nil, fmt.Errorf("ss missing password")
		}
		return map[string]any{
			"type":        "shadowsocks",
			"tag":         "proxy",
			"server":      n.Server,
			"server_port": n.Port,
			"method":      method,
			"password":    n.Password,
		}, nil

	case model.ProtoTrojan:
		if n.Password == "" {
			return nil, fmt.Errorf("trojan missing password")
		}
		o := map[string]any{
			"type":        "trojan",
			"tag":         "proxy",
			"server":      n.Server,
			"server_port": n.Port,
			"password":    n.Password,
		}
		tls := map[string]any{
			"enabled":     true,
			"server_name": firstNonEmpty(n.SNI, n.Host, n.Server),
		}
		if n.SkipTLSVerify() {
			tls["insecure"] = true
		}
		if n.ALPN != "" {
			tls["alpn"] = splitCSV(n.ALPN)
		}
		if fp := extra["fp"]; fp != "" {
			tls["utls"] = map[string]any{"enabled": true, "fingerprint": fp}
		}
		if strings.EqualFold(n.Security, "reality") || extra["pbk"] != "" {
			tls["utls"] = map[string]any{"enabled": true, "fingerprint": firstNonEmpty(extra["fp"], "chrome")}
			tls["reality"] = map[string]any{
				"enabled": true, "public_key": extra["pbk"], "short_id": extra["sid"],
			}
		}
		o["tls"] = tls
		if tr := buildTransport(n); tr != nil {
			o["transport"] = tr
		}
		return o, nil

	case model.ProtoVMess:
		if n.UUID == "" {
			return nil, fmt.Errorf("vmess missing uuid")
		}
		sec := n.Method
		if sec == "" {
			sec = "auto"
		}
		o := map[string]any{
			"type":        "vmess",
			"tag":         "proxy",
			"server":      n.Server,
			"server_port": n.Port,
			"uuid":        n.UUID,
			"security":    sec,
			"alter_id":    0,
		}
		if aid := extra["aid"]; aid != "" {
			if v, err := strconv.Atoi(aid); err == nil {
				o["alter_id"] = v
			}
		}
		if n.TLS || n.Security == "tls" {
			tls := map[string]any{
				"enabled":     true,
				"server_name": firstNonEmpty(n.SNI, n.Host, n.Server),
			}
			if n.SkipTLSVerify() {
				tls["insecure"] = true
			}
			if fp := extra["fp"]; fp != "" {
				tls["utls"] = map[string]any{"enabled": true, "fingerprint": fp}
			}
			if n.ALPN != "" {
				tls["alpn"] = splitCSV(n.ALPN)
			}
			o["tls"] = tls
		}
		if tr := buildTransport(n); tr != nil {
			o["transport"] = tr
		}
		return o, nil

	case model.ProtoVLESS:
		if n.UUID == "" {
			return nil, fmt.Errorf("vless missing uuid")
		}
		o := map[string]any{
			"type":        "vless",
			"tag":         "proxy",
			"server":      n.Server,
			"server_port": n.Port,
			"uuid":        n.UUID,
		}
		if n.Flow != "" {
			o["flow"] = n.Flow
		}
		sec := strings.ToLower(n.Security)
		if n.TLS || sec == "tls" || sec == "reality" {
			tls := map[string]any{
				"enabled":     true,
				"server_name": firstNonEmpty(n.SNI, n.Host, n.Server),
			}
			if n.SkipTLSVerify() {
				tls["insecure"] = true
			}
			if fp := extra["fp"]; fp != "" {
				tls["utls"] = map[string]any{"enabled": true, "fingerprint": fp}
			} else {
				tls["utls"] = map[string]any{"enabled": true, "fingerprint": "chrome"}
			}
			if sec == "reality" || extra["pbk"] != "" {
				tls["reality"] = map[string]any{
					"enabled":    true,
					"public_key": extra["pbk"],
					"short_id":   extra["sid"],
				}
			}
			if n.ALPN != "" {
				tls["alpn"] = splitCSV(n.ALPN)
			}
			o["tls"] = tls
		}
		if tr := buildTransport(n); tr != nil {
			o["transport"] = tr
		}
		return o, nil

	case model.ProtoHysteria2:
		if n.Password == "" {
			return nil, fmt.Errorf("hysteria2 missing password")
		}
		o := map[string]any{
			"type":        "hysteria2",
			"tag":         "proxy",
			"server":      n.Server,
			"server_port": n.Port,
			"password":    n.Password,
		}
		if ports := singBoxServerPorts(extra["ports"]); len(ports) > 0 {
			delete(o, "server_port")
			o["server_ports"] = ports
		}
		if interval := singBoxHopInterval(extra["hop-interval"]); interval != "" {
			o["hop_interval"] = interval
		}
		tls := map[string]any{
			"enabled":     true,
			"server_name": firstNonEmpty(n.SNI, n.Host, n.Server),
		}
		if n.SkipTLSVerify() {
			tls["insecure"] = true
		}
		if n.ALPN != "" {
			tls["alpn"] = splitCSV(n.ALPN)
		}
		if fp := extra["fp"]; fp != "" {
			tls["utls"] = map[string]any{"enabled": true, "fingerprint": fp}
		}
		o["tls"] = tls
		if obfsType := extra["obfs"]; obfsType != "" {
			o["obfs"] = map[string]any{"type": obfsType, "password": extra["obfs-password"]}
		}
		return o, nil

	case model.ProtoTUIC:
		if n.UUID == "" || n.Password == "" {
			return nil, fmt.Errorf("tuic missing uuid/password")
		}
		o := map[string]any{
			"type":               "tuic",
			"tag":                "proxy",
			"server":             n.Server,
			"server_port":        n.Port,
			"uuid":               n.UUID,
			"password":           n.Password,
			"congestion_control": firstNonEmpty(extra["congestion_control"], "cubic"),
			"udp_relay_mode":     firstNonEmpty(extra["udp_relay_mode"], "native"),
		}
		tls := map[string]any{
			"enabled":     true,
			"server_name": firstNonEmpty(n.SNI, n.Host, n.Server),
		}
		if n.SkipTLSVerify() {
			tls["insecure"] = true
		}
		if n.ALPN != "" {
			tls["alpn"] = splitCSV(n.ALPN)
		}
		if fp := extra["fp"]; fp != "" {
			tls["utls"] = map[string]any{"enabled": true, "fingerprint": fp}
		}
		o["tls"] = tls
		return o, nil

	default:
		return nil, fmt.Errorf("unsupported protocol %s", n.Protocol)
	}
}

func buildTransport(n *model.Node) map[string]any {
	netw := strings.ToLower(n.Network)
	if netw == "" || netw == "tcp" {
		// headerType http 伪装
		if n.Extra != nil && (n.Extra["headerType"] == "http" || n.Extra["type"] == "http") {
			return map[string]any{
				"type": "http",
				"host": nonEmptySlice(n.Host),
				"path": firstNonEmpty(n.Path, "/"),
			}
		}
		return nil
	}
	switch netw {
	case "ws", "websocket":
		tr := map[string]any{
			"type": "ws",
			"path": firstNonEmpty(n.Path, "/"),
		}
		if n.Host != "" {
			tr["headers"] = map[string]any{"Host": n.Host}
		}
		return tr
	case "grpc":
		svc := ""
		if n.Extra != nil {
			svc = n.Extra["serviceName"]
		}
		if svc == "" {
			svc = strings.TrimPrefix(n.Path, "/")
		}
		return map[string]any{
			"type":         "grpc",
			"service_name": firstNonEmpty(svc, "GunService"),
		}
	case "http", "h2":
		return map[string]any{
			"type": "http",
			"host": nonEmptySlice(n.Host),
			"path": firstNonEmpty(n.Path, "/"),
		}
	case "httpupgrade":
		tr := map[string]any{
			"type": "httpupgrade",
			"path": firstNonEmpty(n.Path, "/"),
		}
		if n.Host != "" {
			tr["host"] = n.Host
		}
		return tr
	default:
		// xhttp 等较新传输：sing-box 版本差异大，跳过 transport 让核心尝试 tcp
		return nil
	}
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func nonEmptySlice(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return []string{s}
}

func singBoxServerPorts(value string) []string {
	var ports []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if left, right, found := strings.Cut(item, "-"); found {
			if _, err := strconv.Atoi(strings.TrimSpace(left)); err != nil {
				continue
			}
			if _, err := strconv.Atoi(strings.TrimSpace(right)); err != nil {
				continue
			}
			item = strings.TrimSpace(left) + ":" + strings.TrimSpace(right)
		} else if _, err := strconv.Atoi(item); err != nil {
			continue
		}
		ports = append(ports, item)
	}
	return ports
}

func singBoxHopInterval(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "-") {
		return ""
	}
	if _, err := strconv.Atoi(value); err == nil {
		return value + "s"
	}
	return value
}

// Supports 是否有至少一个内置引擎可尝试真实拨测。
func Supports(n *model.Node) bool {
	return SupportsEngine(n, "mihomo")
}

// SupportsEngine reports protocol support for a specific configured engine.
// "both" is Mihomo-authoritative and therefore accepts every Mihomo protocol.
func SupportsEngine(n *model.Node, engine string) bool {
	if n == nil {
		return false
	}
	engine = strings.ToLower(strings.TrimSpace(engine))
	network := strings.ToLower(strings.TrimSpace(n.Network))
	if (network == "xhttp" || network == "splithttp") && engine != "mihomo" && engine != "both" {
		return false
	}
	switch n.Protocol {
	case model.ProtoSS:
		return n.Extra["plugin"] == "" || engine == "mihomo" || engine == "both"
	case model.ProtoTrojan, model.ProtoVMess, model.ProtoVLESS:
		return true
	case model.ProtoHysteria2:
		return engine != "xray"
	case model.ProtoTUIC:
		return engine != "xray" && (n.Extra["token"] == "" || engine == "mihomo" || engine == "both")
	case model.ProtoSSR:
		return engine == "mihomo" || engine == "both"
	}
	return false
}
