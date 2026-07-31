package exporter

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GALIAIS/NodeHarvest/internal/config"
	"github.com/GALIAIS/NodeHarvest/internal/model"
	"github.com/GALIAIS/NodeHarvest/internal/timex"
	"gopkg.in/yaml.v3"
)

// Export 按配置写出多种格式，返回写入的文件路径
func Export(nodes []*model.Node, cfg *config.Config) ([]string, error) {
	dir := cfg.Export.Dir
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	// #nosec G302 -- dir is a directory and 0700 is the restrictive directory mode.
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	ts := timex.FormatFileTS(time.Now())
	prefix := cfg.Export.FilenamePrefix
	var written []string

	for _, format := range cfg.Export.Formats {
		format = strings.ToLower(strings.TrimSpace(format))
		var path string
		var err error
		switch format {
		case "raw", "uri", "txt":
			path = filepath.Join(dir, fmt.Sprintf("%s-%s.txt", prefix, ts))
			err = writeRaw(path, nodes)
		case "base64", "sub", "subscription":
			path = filepath.Join(dir, fmt.Sprintf("%s-%s.base64.txt", prefix, ts))
			err = writeBase64(path, nodes)
		case "clash", "yaml", "yml":
			path = filepath.Join(dir, fmt.Sprintf("%s-%s.clash.yaml", prefix, ts))
			err = writeClash(path, nodes)
		case "json":
			path = filepath.Join(dir, fmt.Sprintf("%s-%s.json", prefix, ts))
			err = writeJSON(path, nodes)
		default:
			continue
		}
		if err != nil {
			return written, fmt.Errorf("export %s: %w", format, err)
		}
		written = append(written, path)
	}

	// 始终写一份 latest 副本（跨平台用复制，不用 symlink）
	if len(written) > 0 {
		stable := []struct {
			path  string
			write func(string, []*model.Node) error
		}{
			{filepath.Join(dir, prefix+"-latest.txt"), writeRaw},
			{filepath.Join(dir, prefix+"-latest.base64.txt"), writeBase64},
			{filepath.Join(dir, prefix+"-latest.json"), writeJSON},
			{filepath.Join(dir, prefix+"-latest.clash.yaml"), writeClash},
			{filepath.Join(dir, "sub.txt"), writeRaw},
			{filepath.Join(dir, "sub.base64"), writeBase64},
			{filepath.Join(dir, "clash.yaml"), writeClash},
		}
		for _, output := range stable {
			if err := output.write(output.path, nodes); err != nil {
				return written, fmt.Errorf("export stable %s: %w", output.path, err)
			}
		}
	}
	cleanupOldRuns(dir, prefix, cfg.Export.KeepRuns)
	return written, nil
}

func cleanupOldRuns(dir, prefix string, keep int) {
	if keep <= 0 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	runs := make(map[string][]string)
	for _, entry := range entries {
		name := entry.Name()
		stem := strings.TrimPrefix(name, prefix+"-")
		if entry.IsDir() || stem == name || len(stem) < 15 {
			continue
		}
		run := stem[:15]
		if _, err := time.Parse("20060102-150405", run); err != nil {
			continue
		}
		runs[run] = append(runs[run], name)
	}
	keys := make([]string, 0, len(runs))
	for run := range runs {
		keys = append(keys, run)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(keys)))
	if len(keys) <= keep {
		return
	}
	for _, run := range keys[keep:] {
		for _, name := range runs[run] {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}

// RenderRaw 内存渲染 URI 列表
func RenderRaw(nodes []*model.Node) string {
	var b strings.Builder
	usedNames := reservedProxyNames(len(nodes))
	nextSuffix := make(map[string]int)
	for i, n := range nodes {
		if n == nil || n.RawURI == "" {
			continue
		}
		name := claimUniqueName(n.Name, i, usedNames, nextSuffix)
		b.WriteString(rawURIWithName(n.RawURI, n.Protocol, name))
		b.WriteByte('\n')
	}
	return b.String()
}

// RenderBase64 内存渲染 base64 订阅
func RenderBase64(nodes []*model.Node) string {
	return base64.StdEncoding.EncodeToString([]byte(RenderRaw(nodes)))
}

// RenderClash 内存渲染 Clash proxies YAML
func RenderClash(nodes []*model.Node) string {
	return buildClash(nodes)
}

func writeRaw(path string, nodes []*model.Node) error {
	return writePrivateFile(path, []byte(RenderRaw(nodes)))
}

func writeBase64(path string, nodes []*model.Node) error {
	return writePrivateFile(path, []byte(RenderBase64(nodes)))
}

func writeJSON(path string, nodes []*model.Node) error {
	type outNode struct {
		Protocol  string  `json:"protocol"`
		Name      string  `json:"name"`
		Server    string  `json:"server"`
		Port      int     `json:"port"`
		LatencyMS int64   `json:"latency_ms"`
		Score     float64 `json:"score"`
		Grade     string  `json:"grade,omitempty"`
		Country   string  `json:"country,omitempty"`
		City      string  `json:"city,omitempty"`
		TLS       bool    `json:"tls"`
		Source    string  `json:"source"`
		URI       string  `json:"uri"`
	}
	list := make([]outNode, 0, len(nodes))
	for _, n := range nodes {
		list = append(list, outNode{
			Protocol:  string(n.Protocol),
			Name:      n.Name,
			Server:    n.Server,
			Port:      n.Port,
			LatencyMS: n.LatencyMS(),
			Score:     n.Score,
			Grade:     n.Grade,
			Country:   n.Country,
			City:      n.City,
			TLS:       n.TLS,
			Source:    n.Source,
			URI:       n.RawURI,
		})
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return writePrivateFile(path, data)
}

// writeClash 导出可直接导入 Clash Meta/Mihomo 的完整配置
func writeClash(path string, nodes []*model.Node) error {
	return writePrivateFile(path, []byte(buildClash(nodes)))
}

func writePrivateFile(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func buildClash(nodes []*model.Node) string {
	usedNames := reservedProxyNames(len(nodes))
	nextSuffix := make(map[string]int)
	proxies := make([]map[string]any, 0, len(nodes))
	proxyNames := make([]string, 0, len(nodes))
	for i, n := range nodes {
		if n == nil {
			continue
		}
		name := claimUniqueName(n.Name, i, usedNames, nextSuffix)
		proxy, err := BuildClashProxy(n, name)
		if err == nil {
			proxies = append(proxies, proxy)
			proxyNames = append(proxyNames, name)
		}
	}
	root := map[string]any{
		"mixed-port": 7890,
		"allow-lan":  false,
		"mode":       "rule",
		"log-level":  "info",
		"proxies":    proxies,
	}
	if len(proxyNames) > 0 {
		groupName := claimUniqueName("NodeHarvest", len(nodes), usedNames, nextSuffix)
		root["proxy-groups"] = []any{map[string]any{
			"name": groupName, "type": "select", "proxies": proxyNames,
		}}
		root["rules"] = []string{"MATCH," + groupName}
	}
	raw, err := yaml.Marshal(root)
	if err != nil {
		return "proxies: []\n"
	}
	return "# Generated by nodeharvest\n" + string(raw)
}

func claimUniqueName(name string, idx int, used map[string]struct{}, next map[string]int) string {
	base := sanitizeYAMLName(name, idx)
	candidate := base
	if _, exists := used[candidate]; exists {
		suffix := max(next[base], 2)
		for {
			candidate = fmt.Sprintf("%s #%d", base, suffix)
			suffix++
			if _, collision := used[candidate]; !collision {
				break
			}
		}
		next[base] = suffix
	}
	used[candidate] = struct{}{}
	return candidate
}

func reservedProxyNames(capacity int) map[string]struct{} {
	used := make(map[string]struct{}, capacity+6)
	for _, name := range []string{"DIRECT", "REJECT", "REJECT-DROP", "PASS", "COMPATIBLE", "GLOBAL"} {
		used[name] = struct{}{}
	}
	return used
}

func rawURIWithName(raw string, protocol model.Protocol, name string) string {
	switch protocol {
	case model.ProtoVMess:
		_, encoded, found := strings.Cut(raw, "://")
		if !found {
			return raw
		}
		payload, ok := decodeBase64Payload(encoded)
		if !ok {
			return raw
		}
		var config map[string]any
		if json.Unmarshal(payload, &config) != nil {
			return raw
		}
		config["ps"] = name
		payload, err := json.Marshal(config)
		if err != nil {
			return raw
		}
		return "vmess://" + base64.StdEncoding.EncodeToString(payload)
	case model.ProtoSSR:
		_, encoded, found := strings.Cut(raw, "://")
		if !found {
			return raw
		}
		payload, ok := decodeBase64Payload(encoded)
		if !ok {
			return raw
		}
		base, query, _ := strings.Cut(string(payload), "/?")
		values, err := url.ParseQuery(query)
		if err != nil {
			return raw
		}
		values.Set("remarks", base64.RawURLEncoding.EncodeToString([]byte(name)))
		payload = []byte(strings.TrimSuffix(base, "/") + "/?" + values.Encode())
		return "ssr://" + base64.RawURLEncoding.EncodeToString(payload)
	default:
		if fragment := strings.LastIndex(raw, "#"); fragment >= 0 {
			raw = raw[:fragment]
		}
		return raw + "#" + url.PathEscape(name)
	}
}

func decodeBase64Payload(value string) ([]byte, bool) {
	value = strings.TrimSpace(value)
	for _, encoding := range []*base64.Encoding{
		base64.RawStdEncoding, base64.StdEncoding, base64.RawURLEncoding, base64.URLEncoding,
	} {
		if decoded, err := encoding.DecodeString(value); err == nil {
			return decoded, true
		}
	}
	return nil, false
}

// BuildClashProxy converts one node to the exact proxy object consumed by
// Clash Meta/Mihomo. The dialer reuses it so exported and tested configs match.
func BuildClashProxy(n *model.Node, name string) (map[string]any, error) {
	if n == nil || !n.IsValid() {
		return nil, fmt.Errorf("invalid node")
	}
	if strings.TrimSpace(name) == "" {
		name = n.Name
	}
	p := map[string]any{
		"name": name, "server": n.Server, "port": n.Port, "udp": true,
	}
	switch n.Protocol {
	case model.ProtoVMess:
		if n.UUID == "" {
			return nil, fmt.Errorf("vmess missing uuid")
		}
		p["type"], p["uuid"], p["cipher"] = "vmess", n.UUID, first(n.Method, "auto")
		p["alterId"] = intValue(n.Extra["aid"])
		applyTLS(p, n, "servername")
		applyTransport(p, n)
	case model.ProtoVLESS:
		if n.UUID == "" {
			return nil, fmt.Errorf("vless missing uuid")
		}
		if (strings.EqualFold(n.Security, "reality") || n.Extra["sid"] != "") && n.Extra["pbk"] == "" {
			return nil, fmt.Errorf("vless reality missing public key")
		}
		p["type"], p["uuid"] = "vless", n.UUID
		if value := n.Extra["encryption"]; value != "" {
			p["encryption"] = value
		}
		if n.Flow != "" {
			p["flow"] = n.Flow
		}
		if value := n.Extra["packet-encoding"]; value != "" {
			p["packet-encoding"] = value
		}
		applyTLS(p, n, "servername")
		applyReality(p, n)
		applyTransport(p, n)
	case model.ProtoTrojan:
		if n.Password == "" {
			return nil, fmt.Errorf("trojan missing password")
		}
		if (strings.EqualFold(n.Security, "reality") || n.Extra["sid"] != "") && n.Extra["pbk"] == "" {
			return nil, fmt.Errorf("trojan reality missing public key")
		}
		p["type"], p["password"] = "trojan", n.Password
		applyTLS(p, n, "sni")
		applyReality(p, n)
		applyTransport(p, n)
	case model.ProtoSS:
		if n.Method == "" || n.Password == "" {
			return nil, fmt.Errorf("ss missing cipher/password")
		}
		p["type"], p["cipher"], p["password"] = "ss", n.Method, n.Password
		if plugin := n.Extra["plugin"]; plugin != "" {
			p["plugin"] = plugin
			if raw := n.Extra["plugin-opts"]; raw != "" {
				var opts map[string]any
				if yaml.Unmarshal([]byte(raw), &opts) == nil && len(opts) > 0 {
					p["plugin-opts"] = opts
				}
			}
		}
	case model.ProtoSSR:
		if n.Method == "" || n.Password == "" || n.Extra["protocol"] == "" || n.Extra["obfs"] == "" {
			return nil, fmt.Errorf("ssr missing protocol fields")
		}
		p["type"], p["cipher"], p["password"] = "ssr", n.Method, n.Password
		p["protocol"], p["obfs"] = n.Extra["protocol"], n.Extra["obfs"]
		if value := n.Extra["protocol-param"]; value != "" {
			p["protocol-param"] = value
		}
		if value := n.Extra["obfs-param"]; value != "" {
			p["obfs-param"] = value
		}
	case model.ProtoHysteria2:
		if n.Password == "" {
			return nil, fmt.Errorf("hysteria2 missing password")
		}
		p["type"], p["password"] = "hysteria2", n.Password
		applyTLS(p, n, "sni")
		for _, key := range []string{"ports", "hop-interval", "up", "down", "bbr-profile"} {
			if value := n.Extra[key]; value != "" {
				p[key] = value
			}
		}
		if value := n.Extra["obfs"]; value != "" {
			p["obfs"] = value
		}
		if value := n.Extra["obfs-password"]; value != "" {
			p["obfs-password"] = value
		}
		applyIntegerExtras(p, n.Extra, "obfs-min-packet-size", "obfs-max-packet-size")
		if raw := n.Extra["realm-opts"]; raw != "" {
			var opts map[string]any
			if yaml.Unmarshal([]byte(raw), &opts) == nil && len(opts) > 0 {
				p["realm-opts"] = opts
			}
		}
	case model.ProtoTUIC:
		p["type"] = "tuic"
		if token := n.Extra["token"]; token != "" {
			p["token"] = token
		} else {
			if n.UUID == "" || n.Password == "" {
				return nil, fmt.Errorf("tuic missing token or uuid/password")
			}
			p["uuid"], p["password"] = n.UUID, n.Password
		}
		applyTLS(p, n, "sni")
		for _, key := range []string{"ip", "bbr-profile"} {
			if value := n.Extra[key]; value != "" {
				p[key] = value
			}
		}
		if value := n.Extra["congestion_control"]; value != "" {
			p["congestion-controller"] = value
		}
		if value := n.Extra["udp_relay_mode"]; value != "" {
			p["udp-relay-mode"] = value
		}
		applyIntegerExtras(p, n.Extra,
			"heartbeat-interval", "request-timeout", "max-udp-relay-packet-size", "max-open-streams")
		applyBooleanExtras(p, n.Extra, "disable-sni", "reduce-rtt", "fast-open")
	default:
		return nil, fmt.Errorf("unsupported protocol %s", n.Protocol)
	}
	return p, nil
}

func applyTLS(p map[string]any, n *model.Node, serverNameKey string) {
	security := strings.ToLower(strings.TrimSpace(n.Security))
	if (n.Protocol == model.ProtoVMess || n.Protocol == model.ProtoVLESS) &&
		(n.TLS || security == "tls" || security == "reality") {
		p["tls"] = true
	}
	if n.SNI != "" {
		p[serverNameKey] = n.SNI
	}
	if n.SkipTLSVerify() {
		p["skip-cert-verify"] = true
	}
	if n.ALPN != "" {
		p["alpn"] = splitCSV(n.ALPN)
	}
	if fp := n.Extra["fp"]; fp != "" &&
		(n.Protocol == model.ProtoVMess || n.Protocol == model.ProtoVLESS || n.Protocol == model.ProtoTrojan) {
		p["client-fingerprint"] = fp
	}
	if value := n.Extra["fingerprint"]; value != "" {
		p["fingerprint"] = value
	}
	if value := n.Extra["name-cert-verify"]; value != "" {
		p["name-cert-verify"] = value
	}
	for _, key := range []string{"ech-opts", "shadow-tls-opts", "restls-opts", "jls-opts", "tlsmirror-opts"} {
		raw := n.Extra[key]
		if raw == "" {
			continue
		}
		var opts map[string]any
		if yaml.Unmarshal([]byte(raw), &opts) == nil && len(opts) > 0 {
			p[key] = opts
		}
	}
	if raw := n.Extra["ech"]; raw != "" {
		if _, preserved := p["ech-opts"]; !preserved {
			p["ech-opts"] = map[string]any{"enable": true, "config": raw}
		}
	}
}

func applyReality(p map[string]any, n *model.Node) {
	if strings.EqualFold(n.Security, "reality") || n.Extra["pbk"] != "" {
		opts := map[string]any{}
		if raw := n.Extra["reality-opts"]; raw != "" {
			_ = yaml.Unmarshal([]byte(raw), &opts)
		}
		if value := n.Extra["pbk"]; value != "" {
			opts["public-key"] = value
		}
		if value := n.Extra["sid"]; value != "" {
			opts["short-id"] = value
		}
		p["reality-opts"] = opts
	}
}

func applyTransport(p map[string]any, n *model.Node) {
	network := strings.ToLower(strings.TrimSpace(n.Network))
	if network == "" {
		network = "tcp"
	}
	if network == "websocket" {
		network = "ws"
	}
	if network == "tcp" && strings.EqualFold(n.Extra["headerType"], "http") {
		network = "http"
	}
	p["network"] = network
	switch network {
	case "ws":
		opts := map[string]any{"path": first(n.Path, "/")}
		if n.Host != "" {
			opts["headers"] = map[string]any{"Host": n.Host}
		}
		if value := intValue(first(n.Extra["max-early-data"], n.Extra["ed"])); value > 0 {
			opts["max-early-data"] = value
		}
		if value := first(n.Extra["early-data-header-name"], n.Extra["eh"]); value != "" {
			opts["early-data-header-name"] = value
		}
		mergeOriginalTransport(opts, n)
		p["ws-opts"] = opts
	case "grpc":
		service := first(n.Extra["serviceName"], strings.TrimPrefix(n.Path, "/"))
		opts := map[string]any{"grpc-service-name": service}
		if mode := n.Extra["mode"]; mode != "" {
			opts["grpc-mode"] = mode
		}
		mergeOriginalTransport(opts, n)
		p["grpc-opts"] = opts
	case "http":
		opts := map[string]any{"path": []string{first(n.Path, "/")}}
		if n.Host != "" {
			opts["headers"] = map[string]any{"Host": []string{n.Host}}
		}
		mergeOriginalTransport(opts, n)
		p["http-opts"] = opts
	case "h2":
		opts := map[string]any{"path": first(n.Path, "/")}
		if n.Host != "" {
			opts["host"] = []string{n.Host}
		}
		mergeOriginalTransport(opts, n)
		p["h2-opts"] = opts
	case "httpupgrade":
		opts := map[string]any{"path": first(n.Path, "/")}
		if n.Host != "" {
			opts["host"] = n.Host
		}
		mergeOriginalTransport(opts, n)
		p["http-upgrade-opts"] = opts
	case "xhttp", "splithttp":
		p["network"] = "xhttp"
		opts := map[string]any{"path": first(n.Path, "/")}
		if n.Host != "" {
			opts["headers"] = map[string]any{"Host": n.Host}
		}
		if mode := n.Extra["mode"]; mode != "" {
			opts["mode"] = mode
		}
		mergeOriginalTransport(opts, n)
		p["xhttp-opts"] = opts
	}
}

func mergeOriginalTransport(target map[string]any, n *model.Node) {
	raw := n.Extra["clash-transport-opts"]
	if raw == "" {
		return
	}
	var original map[string]any
	if yaml.Unmarshal([]byte(raw), &original) == nil {
		mergeMissing(target, original)
	}
}

func mergeMissing(target, original map[string]any) {
	for key, value := range original {
		current, exists := target[key]
		if !exists {
			target[key] = value
			continue
		}
		currentMap, currentOK := current.(map[string]any)
		originalMap, originalOK := value.(map[string]any)
		if currentOK && originalOK {
			mergeMissing(currentMap, originalMap)
		}
	}
}

func splitCSV(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func intValue(value string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(value))
	return n
}

func applyIntegerExtras(target map[string]any, extra map[string]string, keys ...string) {
	for _, key := range keys {
		if value := strings.TrimSpace(extra[key]); value != "" {
			if parsed, err := strconv.Atoi(value); err == nil {
				target[key] = parsed
			}
		}
	}
}

func applyBooleanExtras(target map[string]any, extra map[string]string, keys ...string) {
	for _, key := range keys {
		switch strings.ToLower(strings.TrimSpace(extra[key])) {
		case "1", "true", "yes", "on":
			target[key] = true
		case "0", "false", "no", "off":
			target[key] = false
		}
	}
}

func sanitizeYAMLName(name string, idx int) string {
	name = strings.ReplaceAll(name, "\"", "'")
	name = strings.ReplaceAll(name, "\n", " ")
	if strings.TrimSpace(name) == "" {
		return fmt.Sprintf("node-%d", idx+1)
	}
	return name
}

func first(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
