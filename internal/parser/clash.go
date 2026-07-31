package parser

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"sort"
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
		strings.Contains(low, "type: hysteria") ||
		strings.Contains(low, "type: tuic") {
		return strings.Contains(low, "server:")
	}
	return false
}

// ParseClash 从 Clash/Mihomo YAML 提取节点。直接映射字段，避免经 URI
// 中转时丢失 WebSocket、gRPC、Reality 和插件参数。
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
		n := clashMapToNode(m, source)
		if n == nil {
			continue
		}
		key := n.Key()
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
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
		q := url.Values{}
		if plugin := asString(m["plugin"]); plugin != "" {
			spec := plugin
			if opts := asMap(m["plugin-opts"]); opts != nil {
				keys := make([]string, 0, len(opts))
				for key := range opts {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				for _, key := range keys {
					if enabled, ok := opts[key].(bool); ok && enabled {
						spec += ";" + key
					} else {
						spec += ";" + key + "=" + asString(opts[key])
					}
				}
			}
			q.Set("plugin", spec)
		}
		query := ""
		if len(q) > 0 {
			query = "?" + q.Encode()
		}
		return fmt.Sprintf("ss://%s@%s%s#%s", userinfo, uriHostPort(server, strconv.Itoa(port)), query, frag)
	case "ssr":
		cipher, password := asString(m["cipher"]), asString(m["password"])
		protocol, obfs := asString(m["protocol"]), asString(m["obfs"])
		if cipher == "" || password == "" || protocol == "" || obfs == "" {
			return ""
		}
		params := url.Values{}
		if name != "" {
			params.Set("remarks", rawB64(name))
		}
		if value := asString(m["protocol-param"]); value != "" {
			params.Set("protoparam", rawB64(value))
		}
		if value := asString(m["obfs-param"]); value != "" {
			params.Set("obfsparam", rawB64(value))
		}
		body := fmt.Sprintf("%s:%s:%s:%s:%s/?%s", uriHostPort(server, strconv.Itoa(port)),
			protocol, cipher, obfs, rawB64(password), params.Encode())
		return "ssr://" + rawB64(body)
	case "trojan":
		password := asString(m["password"])
		if password == "" {
			return ""
		}
		q := url.Values{}
		if sni := firstStr(m, "sni", "servername"); sni != "" {
			q.Set("sni", sni)
		}
		if path := clashPath(m); path != "" {
			q.Set("path", path)
		}
		if host := clashHost(m); host != "" {
			q.Set("host", host)
		}
		applyClashURIOptions(q, m)
		if asBool(m["skip-cert-verify"]) {
			q.Set("allowInsecure", "1")
		}
		if ro := asMap(m["reality-opts"]); ro != nil {
			q.Set("security", "reality")
			q.Set("pbk", firstStr(ro, "public-key", "public_key"))
			q.Set("sid", firstStr(ro, "short-id", "short_id"))
		}
		return fmt.Sprintf("trojan://%s@%s?%s#%s", url.QueryEscape(password),
			uriHostPort(server, strconv.Itoa(port)), q.Encode(), frag)
	case "vmess":
		obj := map[string]any{
			"v":    "2",
			"ps":   name,
			"add":  server,
			"port": strconv.Itoa(port),
			"id":   asString(m["uuid"]),
			"aid":  firstStr(m, "alterId", "alter-id"),
			"scy":  firstStr(m, "cipher", "security"),
			"net":  firstNonEmpty(firstStr(m, "network", "net"), "tcp"),
			"type": "none",
			"tls":  boolToTLS(m["tls"]),
		}
		if obj["aid"] == "" {
			obj["aid"] = "0"
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
		if alpn := asCSV(m["alpn"]); alpn != "" {
			obj["alpn"] = alpn
		}
		if fp := asString(m["client-fingerprint"]); fp != "" {
			obj["fp"] = fp
		}
		if asBool(m["skip-cert-verify"]) {
			obj["allowInsecure"] = true
		}
		if opts := asMap(m["grpc-opts"]); opts != nil {
			obj["path"] = firstStr(opts, "grpc-service-name", "serviceName")
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
		q.Set("encryption", firstNonEmpty(asString(m["encryption"]), "none"))
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
		applyClashURIOptions(q, m)
		if value := asString(m["packet-encoding"]); value != "" {
			q.Set("packetEncoding", value)
		}
		if asBool(m["skip-cert-verify"]) {
			q.Set("allowInsecure", "1")
		}
		if ro, ok := m["reality-opts"].(map[string]any); ok {
			if pbk := asString(ro["public-key"]); pbk != "" {
				q.Set("pbk", pbk)
			}
			if sid := asString(ro["short-id"]); sid != "" {
				q.Set("sid", sid)
			}
		}
		return fmt.Sprintf("vless://%s@%s?%s#%s", uuid,
			uriHostPort(server, strconv.Itoa(port)), q.Encode(), frag)
	case "hysteria2", "hy2":
		password := firstStr(m, "password", "auth")
		q := url.Values{}
		if sni := firstStr(m, "sni", "servername"); sni != "" {
			q.Set("sni", sni)
		}
		if value := asString(m["obfs"]); value != "" {
			q.Set("obfs", value)
		}
		if value := asString(m["obfs-password"]); value != "" {
			q.Set("obfs-password", value)
		}
		if value := asCSV(m["alpn"]); value != "" {
			q.Set("alpn", value)
		}
		if value := asString(m["fingerprint"]); value != "" {
			q.Set("pinSHA256", value)
		}
		if value := asString(m["ech"]); value != "" {
			q.Set("ech", value)
		}
		if asBool(m["skip-cert-verify"]) {
			q.Set("insecure", "1")
		}
		portSpec := firstStr(m, "ports")
		if portSpec == "" {
			portSpec = strconv.Itoa(port)
		}
		return fmt.Sprintf("hysteria2://%s@%s?%s#%s", url.QueryEscape(password),
			uriHostPort(server, portSpec), q.Encode(), frag)
	case "tuic":
		uuid, password := asString(m["uuid"]), asString(m["password"])
		if uuid == "" || password == "" {
			return ""
		}
		q := url.Values{}
		if value := firstStr(m, "sni", "servername"); value != "" {
			q.Set("sni", value)
		}
		if value := asCSV(m["alpn"]); value != "" {
			q.Set("alpn", value)
		}
		if value := firstStr(m, "congestion-controller", "congestion_control"); value != "" {
			q.Set("congestion_control", value)
		}
		if value := firstStr(m, "udp-relay-mode", "udp_relay_mode"); value != "" {
			q.Set("udp_relay_mode", value)
		}
		if asBool(m["skip-cert-verify"]) {
			q.Set("allowInsecure", "1")
		}
		return fmt.Sprintf("tuic://%s:%s@%s?%s#%s",
			url.QueryEscape(uuid), url.QueryEscape(password),
			uriHostPort(server, strconv.Itoa(port)), q.Encode(), frag)
	default:
		return ""
	}
}

func applyClashURIOptions(q url.Values, m map[string]any) {
	if fp := asString(m["client-fingerprint"]); fp != "" {
		q.Set("fp", fp)
	}
	if alpn := asCSV(m["alpn"]); alpn != "" {
		q.Set("alpn", alpn)
	}
	network := firstStr(m, "network", "net")
	if network != "" {
		q.Set("type", network)
	}
	switch strings.ToLower(network) {
	case "grpc":
		if opts := asMap(m["grpc-opts"]); opts != nil {
			q.Set("serviceName", firstStr(opts, "grpc-service-name", "serviceName"))
			if mode := firstStr(opts, "grpc-mode", "mode"); mode != "" {
				q.Set("mode", mode)
			}
		}
	case "ws", "websocket":
		if opts := asMap(m["ws-opts"]); opts != nil {
			if value := asString(opts["max-early-data"]); value != "" {
				q.Set("ed", value)
			}
			if value := asString(opts["early-data-header-name"]); value != "" {
				q.Set("eh", value)
			}
		}
	case "xhttp", "splithttp":
		if opts := asMap(m["xhttp-opts"]); opts != nil {
			if mode := asString(opts["mode"]); mode != "" {
				q.Set("mode", mode)
			}
		}
	}
}

func rawB64(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func uriHostPort(server, port string) string {
	return net.JoinHostPort(strings.Trim(server, "[]"), port)
}

func clashMapToNode(m map[string]any, source string) *model.Node {
	typ := strings.ToLower(asString(m["type"]))
	server := asString(m["server"])
	port := asInt(m["port"])
	if server == "" || port <= 0 {
		return nil
	}
	extra := map[string]string{}
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
		Flow:     asString(m["flow"]),
		ALPN:     asCSV(m["alpn"]),
		Extra:    extra,
	}
	if n.Network == "" {
		n.Network = "tcp"
	}
	if asBool(m["skip-cert-verify"]) {
		extra["skip-cert-verify"] = "true"
	}
	if fp := asString(m["client-fingerprint"]); fp != "" {
		extra["fp"] = fp
	}
	if value := asString(m["fingerprint"]); value != "" {
		extra["fingerprint"] = value
	}
	if value := asString(m["name-cert-verify"]); value != "" {
		extra["name-cert-verify"] = value
	}
	if value := asString(m["packet-encoding"]); value != "" {
		extra["packet-encoding"] = value
	}
	if reality := asMap(m["reality-opts"]); reality != nil {
		n.Security = "reality"
		n.TLS = true
		extra["pbk"] = firstStr(reality, "public-key", "public_key")
		extra["sid"] = firstStr(reality, "short-id", "short_id")
		if raw, err := json.Marshal(reality); err == nil {
			extra["reality-opts"] = string(raw)
		}
	} else if n.TLS {
		n.Security = "tls"
	}
	for _, key := range []string{"ech-opts", "shadow-tls-opts", "restls-opts", "jls-opts", "tlsmirror-opts"} {
		if opts := asMap(m[key]); opts != nil {
			if raw, err := json.Marshal(opts); err == nil {
				extra[key] = string(raw)
			}
		}
	}
	applyClashTransport(n, m)
	switch typ {
	case "ss", "shadowsocks":
		n.Protocol = model.ProtoSS
		if plugin := asString(m["plugin"]); plugin != "" {
			extra["plugin"] = plugin
			if opts := asMap(m["plugin-opts"]); opts != nil {
				if raw, err := json.Marshal(opts); err == nil {
					extra["plugin-opts"] = string(raw)
				}
			}
		}
	case "ssr":
		n.Protocol = model.ProtoSSR
		extra["protocol"] = asString(m["protocol"])
		extra["obfs"] = asString(m["obfs"])
		extra["protocol-param"] = asString(m["protocol-param"])
		extra["obfs-param"] = asString(m["obfs-param"])
	case "trojan":
		n.Protocol = model.ProtoTrojan
		n.TLS = true
	case "vmess":
		n.Protocol = model.ProtoVMess
	case "vless":
		n.Protocol = model.ProtoVLESS
		extra["encryption"] = asString(m["encryption"])
	case "hysteria2", "hy2":
		n.Protocol = model.ProtoHysteria2
		n.TLS = true
		extra["obfs"] = asString(m["obfs"])
		extra["obfs-password"] = asString(m["obfs-password"])
		for _, key := range []string{
			"ports", "hop-interval", "up", "down", "bbr-profile",
			"obfs-min-packet-size", "obfs-max-packet-size",
		} {
			extra[key] = asString(m[key])
		}
		if opts := asMap(m["realm-opts"]); opts != nil {
			if raw, err := json.Marshal(opts); err == nil {
				extra["realm-opts"] = string(raw)
			}
		}
	case "tuic":
		n.Protocol = model.ProtoTUIC
		n.TLS = true
		extra["token"] = asString(m["token"])
		extra["congestion_control"] = firstStr(m, "congestion-controller", "congestion_control")
		extra["udp_relay_mode"] = firstStr(m, "udp-relay-mode", "udp_relay_mode")
		for _, key := range []string{
			"ip", "heartbeat-interval", "disable-sni", "reduce-rtt", "request-timeout",
			"max-udp-relay-packet-size", "fast-open", "max-open-streams", "bbr-profile",
		} {
			extra[key] = asString(m[key])
		}
	default:
		return nil
	}
	if n.Name == "" {
		n.Name = n.Address()
	}
	n.RawURI = clashProxyToURI(m)
	n.Fingerprint = n.Key()
	return n
}

func applyClashTransport(n *model.Node, m map[string]any) {
	network := strings.ToLower(n.Network)
	optionsKey := map[string]string{
		"ws": "ws-opts", "websocket": "ws-opts", "grpc": "grpc-opts",
		"http": "http-opts", "h2": "h2-opts", "httpupgrade": "http-upgrade-opts",
		"xhttp": "xhttp-opts", "splithttp": "xhttp-opts",
	}[network]
	if opts := asMap(m[optionsKey]); opts != nil {
		if raw, err := json.Marshal(opts); err == nil {
			n.Extra["clash-transport-opts"] = string(raw)
		}
	}
	switch network {
	case "ws", "websocket":
		if opts := asMap(m["ws-opts"]); opts != nil {
			n.Path = asString(opts["path"])
			if headers := asMap(opts["headers"]); headers != nil {
				n.Host = firstListValue(firstAny(headers, "Host", "host"))
			}
			if value := asString(opts["max-early-data"]); value != "" {
				n.Extra["max-early-data"] = value
			}
			if value := asString(opts["early-data-header-name"]); value != "" {
				n.Extra["early-data-header-name"] = value
			}
		}
	case "grpc":
		if opts := asMap(m["grpc-opts"]); opts != nil {
			n.Extra["serviceName"] = firstStr(opts, "grpc-service-name", "serviceName")
			n.Extra["mode"] = firstStr(opts, "grpc-mode", "mode")
		}
	case "http":
		if opts := asMap(m["http-opts"]); opts != nil {
			n.Path = firstListValue(opts["path"])
			if headers := asMap(opts["headers"]); headers != nil {
				n.Host = firstListValue(firstAny(headers, "Host", "host"))
			}
		}
	case "h2":
		if opts := asMap(m["h2-opts"]); opts != nil {
			n.Path = firstListValue(opts["path"])
			n.Host = firstListValue(opts["host"])
		}
	case "httpupgrade":
		if opts := asMap(m["http-upgrade-opts"]); opts != nil {
			n.Path = asString(opts["path"])
			n.Host = asString(opts["host"])
		}
	case "xhttp", "splithttp":
		n.Network = "xhttp"
		if opts := asMap(m["xhttp-opts"]); opts != nil {
			n.Path = asString(opts["path"])
			n.Extra["mode"] = asString(opts["mode"])
			if headers := asMap(opts["headers"]); headers != nil {
				n.Host = firstListValue(firstAny(headers, "Host", "host"))
			}
		}
	}
	if n.Path == "" {
		n.Path = clashPath(m)
	}
	if n.Host == "" {
		n.Host = clashHost(m)
	}
}

func clashHost(m map[string]any) string {
	if h := asString(m["host"]); h != "" {
		return h
	}
	for _, key := range []string{"ws-opts", "xhttp-opts"} {
		opts := asMap(m[key])
		if opts == nil {
			continue
		}
		if headers, ok := opts["headers"].(map[string]any); ok {
			if h := firstListValue(firstAny(headers, "Host", "host")); h != "" {
				return h
			}
		}
	}
	for _, key := range []string{"http-upgrade-opts", "h2-opts"} {
		if opts := asMap(m[key]); opts != nil {
			if h := firstListValue(opts["host"]); h != "" {
				return h
			}
		}
	}
	if opts := asMap(m["http-opts"]); opts != nil {
		if headers := asMap(opts["headers"]); headers != nil {
			return firstListValue(firstAny(headers, "Host", "host"))
		}
	}
	return ""
}

func clashPath(m map[string]any) string {
	if p := asString(m["path"]); p != "" {
		return p
	}
	for _, key := range []string{"ws-opts", "xhttp-opts", "http-upgrade-opts"} {
		if opts := asMap(m[key]); opts != nil {
			if path := asString(opts["path"]); path != "" {
				return path
			}
		}
	}
	for _, key := range []string{"h2-opts", "http-opts"} {
		if opts := asMap(m[key]); opts != nil {
			if path := firstListValue(opts["path"]); path != "" {
				return path
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

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func asCSV(v any) string {
	switch values := v.(type) {
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if item := asString(value); item != "" {
				out = append(out, item)
			}
		}
		return strings.Join(out, ",")
	case []string:
		return strings.Join(values, ",")
	default:
		return asString(v)
	}
}

func firstListValue(v any) string {
	switch values := v.(type) {
	case []any:
		if len(values) > 0 {
			return asString(values[0])
		}
	case []string:
		if len(values) > 0 {
			return values[0]
		}
	default:
		return asString(v)
	}
	return ""
}

func firstAny(m map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := m[key]; ok {
			return value
		}
	}
	return nil
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
