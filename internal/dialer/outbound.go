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
		tls := map[string]any{
			"enabled":     true,
			"server_name": firstNonEmpty(n.SNI, n.Host, n.Server),
		}
		if n.SkipTLSVerify() {
			tls["insecure"] = true
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

// Supports 是否可尝试真实拨测
func Supports(n *model.Node) bool {
	if n == nil {
		return false
	}
	switch n.Protocol {
	case model.ProtoSS, model.ProtoTrojan, model.ProtoVMess, model.ProtoVLESS, model.ProtoHysteria2:
		return true
	default:
		return false
	}
}
