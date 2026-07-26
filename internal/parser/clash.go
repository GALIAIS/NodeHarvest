package parser

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/GALIAIS/NodeHarvest/internal/model"
	"gopkg.in/yaml.v3"
)

// looksLikeClash 粗判是否为 Clash/Mihomo 配置
func looksLikeClash(content string) bool {
	s := content
	if len(s) > 4096 {
		s = s[:4096]
	}
	low := strings.ToLower(s)
	if strings.Contains(low, "proxies:") {
		return true
	}
	if strings.Contains(low, "type: vmess") ||
		strings.Contains(low, "type: vless") ||
		strings.Contains(low, "type: trojan") ||
		strings.Contains(low, "type: ss") ||
		strings.Contains(low, "type: hysteria") {
		return strings.Contains(low, "server:")
	}
	return false
}

// ParseClash 从 Clash YAML 提取节点并还原为 URI
func ParseClash(content, source string) []*model.Node {
	var root map[string]any
	if err := yaml.Unmarshal([]byte(content), &root); err != nil {
		wrapped := "proxies:\n" + content
		if err2 := yaml.Unmarshal([]byte(wrapped), &root); err2 != nil {
			return nil
		}
	}

	rawList, ok := root["proxies"]
	if !ok {
		return nil
	}
	list, ok := rawList.([]any)
	if !ok {
		return nil
	}

	nodes := make([]*model.Node, 0, len(list))
	seen := make(map[string]struct{})
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		uri := clashProxyToURI(m)
		if uri == "" {
			if n2 := clashMapToNode(m, source); n2 != nil {
				key := n2.Key()
				if _, dup := seen[key]; !dup {
					seen[key] = struct{}{}
					nodes = append(nodes, n2)
				}
			}
			continue
		}
		if _, dup := seen[uri]; dup {
			continue
		}
		seen[uri] = struct{}{}
		n, err := ParseURI(uri, source)
		if err != nil || n == nil {
			if n2 := clashMapToNode(m, source); n2 != nil {
				nodes = append(nodes, n2)
			}
			continue
		}
		nodes = append(nodes, n)
	}
	return nodes
}

func clashProxyToURI(m map[string]any) string {
	typ := strings.ToLower(asString(m["type"]))
	name := asString(m["name"])
	server := asString(m["server"])
	port := asInt(m["port"])
	if server == "" || port <= 0 {
		return ""
	}
	frag := url.QueryEscape(name)

	switch typ {
	case "ss", "shadowsocks":
		cipher := asString(m["cipher"])
		password := asString(m["password"])
		if cipher == "" || password == "" {
			return ""
		}
		userinfo := strings.TrimRight(base64.StdEncoding.EncodeToString([]byte(cipher+":"+password)), "=")
		return fmt.Sprintf("ss://%s@%s:%d#%s", userinfo, server, port, frag)
	case "trojan":
		password := asString(m["password"])
		if password == "" {
			return ""
		}
		q := url.Values{}
		if sni := firstStr(m, "sni", "servername"); sni != "" {
			q.Set("sni", sni)
		}
		return fmt.Sprintf("trojan://%s@%s:%d?%s#%s", url.QueryEscape(password), server, port, q.Encode(), frag)
	case "vmess":
		obj := map[string]any{
			"v":    "2",
			"ps":   name,
			"add":  server,
			"port": strconv.Itoa(port),
			"id":   asString(m["uuid"]),
			"aid":  "0",
			"scy":  firstStr(m, "cipher", "security"),
			"net":  firstStr(m, "network", "net"),
			"type": "none",
			"tls":  boolToTLS(m["tls"]),
		}
		if sn := firstStr(m, "servername", "sni"); sn != "" {
			obj["sni"] = sn
		}
		if host := clashHost(m); host != "" {
			obj["host"] = host
		}
		if path := clashPath(m); path != "" {
			obj["path"] = path
		}
		b, err := json.Marshal(obj)
		if err != nil {
			return ""
		}
		return "vmess://" + base64.StdEncoding.EncodeToString(b)
	case "vless":
		uuid := asString(m["uuid"])
		if uuid == "" {
			return ""
		}
		q := url.Values{}
		q.Set("encryption", "none")
		net := firstStr(m, "network", "net")
		if net != "" {
			q.Set("type", net)
		}
		if m["reality-opts"] != nil {
			q.Set("security", "reality")
		} else if asBool(m["tls"]) {
			q.Set("security", "tls")
		}
		if sn := firstStr(m, "servername", "sni"); sn != "" {
			q.Set("sni", sn)
		}
		if flow := asString(m["flow"]); flow != "" {
			q.Set("flow", flow)
		}
		if path := clashPath(m); path != "" {
			q.Set("path", path)
		}
		if host := clashHost(m); host != "" {
			q.Set("host", host)
		}
		if ro, ok := m["reality-opts"].(map[string]any); ok {
			if pbk := asString(ro["public-key"]); pbk != "" {
				q.Set("pbk", pbk)
			}
			if sid := asString(ro["short-id"]); sid != "" {
				q.Set("sid", sid)
			}
		}
		return fmt.Sprintf("vless://%s@%s:%d?%s#%s", uuid, server, port, q.Encode(), frag)
	case "hysteria2", "hy2":
		password := firstStr(m, "password", "auth")
		q := url.Values{}
		if sni := firstStr(m, "sni", "servername"); sni != "" {
			q.Set("sni", sni)
		}
		return fmt.Sprintf("hysteria2://%s@%s:%d?%s#%s", url.QueryEscape(password), server, port, q.Encode(), frag)
	default:
		return ""
	}
}

func clashMapToNode(m map[string]any, source string) *model.Node {
	typ := strings.ToLower(asString(m["type"]))
	server := asString(m["server"])
	port := asInt(m["port"])
	if server == "" || port <= 0 {
		return nil
	}
	n := &model.Node{
		Name:     asString(m["name"]),
		Server:   server,
		Port:     port,
		Source:   source,
		TLS:      asBool(m["tls"]),
		SNI:      firstStr(m, "sni", "servername"),
		UUID:     asString(m["uuid"]),
		Password: firstStr(m, "password", "auth"),
		Method:   firstStr(m, "cipher", "security"),
		Network:  firstStr(m, "network", "net"),
	}
	switch typ {
	case "ss", "shadowsocks":
		n.Protocol = model.ProtoSS
	case "trojan":
		n.Protocol = model.ProtoTrojan
		n.TLS = true
	case "vmess":
		n.Protocol = model.ProtoVMess
	case "vless":
		n.Protocol = model.ProtoVLESS
	case "hysteria2", "hy2":
		n.Protocol = model.ProtoHysteria2
		n.TLS = true
	default:
		return nil
	}
	if n.Name == "" {
		n.Name = n.Address()
	}
	n.Fingerprint = n.Key()
	return n
}

func clashHost(m map[string]any) string {
	if h := asString(m["host"]); h != "" {
		return h
	}
	if opts, ok := m["ws-opts"].(map[string]any); ok {
		if headers, ok := opts["headers"].(map[string]any); ok {
			if h := asString(headers["Host"]); h != "" {
				return h
			}
			if h := asString(headers["host"]); h != "" {
				return h
			}
		}
	}
	return ""
}

func clashPath(m map[string]any) string {
	if p := asString(m["path"]); p != "" {
		return p
	}
	if opts, ok := m["ws-opts"].(map[string]any); ok {
		return asString(opts["path"])
	}
	if opts, ok := m["h2-opts"].(map[string]any); ok {
		switch v := opts["path"].(type) {
		case string:
			return v
		case []any:
			if len(v) > 0 {
				return asString(v[0])
			}
		}
	}
	return ""
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func asInt(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	default:
		return 0
	}
}

func asBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return truthy(t)
	case int:
		return t != 0
	default:
		return false
	}
}

func firstStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := asString(m[k]); s != "" {
			return s
		}
	}
	return ""
}

func boolToTLS(v any) string {
	if asBool(v) {
		return "tls"
	}
	return ""
}
