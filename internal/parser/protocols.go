package parser

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/GALIAIS/NodeHarvest/internal/model"
)

// ---------- VMess ----------

type vmessJSON struct {
	V    any    `json:"v"`
	PS   string `json:"ps"`
	Add  string `json:"add"`
	Port any    `json:"port"`
	ID   string `json:"id"`
	Aid  any    `json:"aid"`
	Scy  string `json:"scy"`
	Net  string `json:"net"`
	Type string `json:"type"`
	Host string `json:"host"`
	Path string `json:"path"`
	TLS  string `json:"tls"`
	SNI  string `json:"sni"`
	Alpn string `json:"alpn"`
	// Providers use both spellings and encode them as either booleans or strings.
	AllowInsecure any `json:"allowInsecure"`
	Insecure      any `json:"insecure"`
}

func parseVMess(raw, source string) (*model.Node, error) {
	payload := strings.TrimPrefix(raw, "vmess://")
	payload = strings.TrimPrefix(payload, "VMESS://")
	// 部分链接带 fragment
	if i := strings.Index(payload, "#"); i >= 0 {
		payload = payload[:i]
	}
	payload = strings.TrimSpace(payload)
	if m := len(payload) % 4; m != 0 {
		payload += strings.Repeat("=", 4-m)
	}
	var data []byte
	var err error
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.RawURLEncoding} {
		data, err = enc.DecodeString(payload)
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("vmess base64: %w", err)
	}

	var j vmessJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, fmt.Errorf("vmess json: %w", err)
	}
	port := anyToInt(j.Port)
	if j.Add == "" || port <= 0 {
		return nil, fmt.Errorf("vmess missing server/port")
	}
	name := j.PS
	if name == "" {
		name = fmt.Sprintf("vmess-%s-%d", j.Add, port)
	}
	extra := map[string]string{"type": j.Type, "aid": fmt.Sprint(j.Aid)}
	if j.AllowInsecure != nil {
		extra["allowInsecure"] = fmt.Sprint(j.AllowInsecure)
	}
	if j.Insecure != nil {
		extra["insecure"] = fmt.Sprint(j.Insecure)
	}
	n := &model.Node{
		Protocol: model.ProtoVMess,
		Name:     name,
		Server:   j.Add,
		Port:     port,
		UUID:     j.ID,
		Method:   j.Scy,
		Network:  firstNonEmpty(j.Net, "tcp"),
		TLS:      truthy(j.TLS),
		SNI:      firstNonEmpty(j.SNI, j.Host),
		Path:     j.Path,
		Host:     j.Host,
		ALPN:     j.Alpn,
		RawURI:   raw,
		Source:   source,
		Extra:    extra,
	}
	n.Fingerprint = n.Key()
	return n, nil
}

// ---------- VLESS ----------

func parseVLESS(raw, source string) (*model.Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	host, portStr, err := splitHostPort(u.Host, "443")
	if err != nil {
		return nil, err
	}
	port := atoiDefault(portStr, 443)
	q := u.Query()
	sec := queryGet(q, "security")
	n := &model.Node{
		Protocol: model.ProtoVLESS,
		Name:     parseNameFromFragment(u, fmt.Sprintf("vless-%s-%d", host, port)),
		Server:   host,
		Port:     port,
		UUID:     u.User.Username(),
		Network:  firstNonEmpty(queryGet(q, "type", "net"), "tcp"),
		TLS:      truthy(sec) || sec == "tls" || sec == "reality",
		SNI:      queryGet(q, "sni", "peer"),
		Path:     queryGet(q, "path"),
		Host:     queryGet(q, "host"),
		Flow:     queryGet(q, "flow"),
		Security: sec,
		ALPN:     queryGet(q, "alpn"),
		RawURI:   raw,
		Source:   source,
		Extra: map[string]string{
			"fp":          queryGet(q, "fp"),
			"pbk":         queryGet(q, "pbk"),
			"sid":         queryGet(q, "sid"),
			"spx":         queryGet(q, "spx"),
			"serviceName": queryGet(q, "serviceName"),
			"headerType":  queryGet(q, "headerType"),
			"mode":        queryGet(q, "mode"),
			"insecure":    queryGet(q, "insecure", "allowInsecure", "skip-cert-verify"),
		},
	}
	n.Fingerprint = n.Key()
	return n, nil
}

// ---------- Trojan ----------

func parseTrojan(raw, source string) (*model.Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	host, portStr, err := splitHostPort(u.Host, "443")
	if err != nil {
		return nil, err
	}
	port := atoiDefault(portStr, 443)
	pass, _ := u.User.Password()
	if pass == "" {
		pass = u.User.Username()
	}
	q := u.Query()
	n := &model.Node{
		Protocol: model.ProtoTrojan,
		Name:     parseNameFromFragment(u, fmt.Sprintf("trojan-%s-%d", host, port)),
		Server:   host,
		Port:     port,
		Password: pass,
		Network:  firstNonEmpty(queryGet(q, "type"), "tcp"),
		TLS:      true,
		SNI:      queryGet(q, "sni", "peer"),
		Path:     queryGet(q, "path"),
		Host:     queryGet(q, "host"),
		ALPN:     queryGet(q, "alpn"),
		RawURI:   raw,
		Source:   source,
		Extra: map[string]string{
			"insecure": queryGet(q, "insecure", "allowInsecure", "skip-cert-verify"),
		},
	}
	n.Fingerprint = n.Key()
	return n, nil
}

// ---------- Shadowsocks ----------

func parseSS(raw, source string) (*model.Node, error) {
	// ss://BASE64(method:password)@host:port#name
	// ss://BASE64(method:password@host:port)#name
	body := strings.TrimPrefix(raw, "ss://")
	body = strings.TrimPrefix(body, "SS://")
	name := ""
	if i := strings.Index(body, "#"); i >= 0 {
		name, _ = url.QueryUnescape(body[i+1:])
		body = body[:i]
	}
	// 去掉 plugin 查询
	if i := strings.Index(body, "?"); i >= 0 {
		body = body[:i]
	}

	var method, password, host string
	var port int

	if strings.Contains(body, "@") {
		// userinfo@host:port 形式，userinfo 可能是 base64
		parts := strings.SplitN(body, "@", 2)
		userinfo := parts[0]
		hp := parts[1]
		if decoded, ok := tryDecodeBase64(userinfo); ok {
			userinfo = decoded
		}
		// method:password
		if idx := strings.Index(userinfo, ":"); idx >= 0 {
			method = userinfo[:idx]
			password = userinfo[idx+1:]
		} else {
			return nil, fmt.Errorf("ss bad userinfo")
		}
		h, p, err := splitHostPort(hp, "8388")
		if err != nil {
			return nil, err
		}
		host, port = h, atoiDefault(p, 8388)
	} else {
		decoded, ok := tryDecodeBase64(body)
		if !ok {
			return nil, fmt.Errorf("ss decode failed")
		}
		// method:password@host:port
		at := strings.LastIndex(decoded, "@")
		if at < 0 {
			return nil, fmt.Errorf("ss missing @")
		}
		userinfo := decoded[:at]
		hp := decoded[at+1:]
		if idx := strings.Index(userinfo, ":"); idx >= 0 {
			method = userinfo[:idx]
			password = userinfo[idx+1:]
		}
		h, p, err := splitHostPort(hp, "8388")
		if err != nil {
			return nil, err
		}
		host, port = h, atoiDefault(p, 8388)
	}

	if name == "" {
		name = fmt.Sprintf("ss-%s-%d", host, port)
	}
	n := &model.Node{
		Protocol: model.ProtoSS,
		Name:     name,
		Server:   host,
		Port:     port,
		Method:   method,
		Password: password,
		RawURI:   raw,
		Source:   source,
	}
	n.Fingerprint = n.Key()
	return n, nil
}

// ---------- SSR ----------

func parseSSR(raw, source string) (*model.Node, error) {
	payload := strings.TrimPrefix(raw, "ssr://")
	payload = strings.TrimPrefix(payload, "SSR://")
	decoded, ok := tryDecodeBase64(payload)
	if !ok {
		return nil, fmt.Errorf("ssr decode failed")
	}
	// host:port:protocol:method:obfs:base64pass/?params
	main := decoded
	params := ""
	if i := strings.Index(decoded, "/?"); i >= 0 {
		main = decoded[:i]
		params = decoded[i+2:]
	} else if i := strings.Index(decoded, "?"); i >= 0 {
		main = decoded[:i]
		params = decoded[i+1:]
	}
	parts := strings.Split(main, ":")
	if len(parts) < 6 {
		return nil, fmt.Errorf("ssr format")
	}
	host := parts[0]
	port := atoiDefault(parts[1], 0)
	method := parts[3]
	passB64 := parts[5]
	password := decodeMaybeBase64(passB64)

	name := fmt.Sprintf("ssr-%s-%d", host, port)
	if params != "" {
		q, _ := url.ParseQuery(params)
		if r := q.Get("remarks"); r != "" {
			if d, ok := tryDecodeBase64(r); ok {
				name = d
			} else {
				name = r
			}
		}
	}
	n := &model.Node{
		Protocol: model.ProtoSSR,
		Name:     name,
		Server:   host,
		Port:     port,
		Method:   method,
		Password: password,
		RawURI:   raw,
		Source:   source,
		Extra: map[string]string{
			"protocol": parts[2],
			"obfs":     parts[4],
		},
	}
	n.Fingerprint = n.Key()
	return n, nil
}

// ---------- Hysteria2 ----------

func parseHysteria2(raw, source string) (*model.Node, error) {
	// hysteria2://password@host:port?params#name
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	host, portStr, err := splitHostPort(u.Host, "443")
	if err != nil {
		return nil, err
	}
	port := atoiDefault(portStr, 443)
	pass, _ := u.User.Password()
	if pass == "" {
		pass = u.User.Username()
	}
	q := u.Query()
	n := &model.Node{
		Protocol: model.ProtoHysteria2,
		Name:     parseNameFromFragment(u, fmt.Sprintf("hy2-%s-%d", host, port)),
		Server:   host,
		Port:     port,
		Password: pass,
		TLS:      true,
		SNI:      queryGet(q, "sni", "peer"),
		ALPN:     queryGet(q, "alpn"),
		RawURI:   raw,
		Source:   source,
		Extra: map[string]string{
			"obfs":          queryGet(q, "obfs"),
			"obfs-password": queryGet(q, "obfs-password", "obfsPassword"),
			"insecure":      queryGet(q, "insecure", "allowInsecure", "skip-cert-verify"),
		},
	}
	n.Fingerprint = n.Key()
	return n, nil
}

// ---------- TUIC ----------

func parseTUIC(raw, source string) (*model.Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	host, portStr, err := splitHostPort(u.Host, "443")
	if err != nil {
		return nil, err
	}
	port := atoiDefault(portStr, 443)
	uuid := u.User.Username()
	pass, _ := u.User.Password()
	q := u.Query()
	n := &model.Node{
		Protocol: model.ProtoTUIC,
		Name:     parseNameFromFragment(u, fmt.Sprintf("tuic-%s-%d", host, port)),
		Server:   host,
		Port:     port,
		UUID:     uuid,
		Password: pass,
		TLS:      true,
		SNI:      queryGet(q, "sni", "peer"),
		ALPN:     queryGet(q, "alpn"),
		RawURI:   raw,
		Source:   source,
		Extra: map[string]string{
			"congestion_control": queryGet(q, "congestion_control", "congestion-control"),
			"udp_relay_mode":     queryGet(q, "udp_relay_mode", "udp-relay-mode"),
			"insecure":           queryGet(q, "insecure", "allowInsecure", "skip-cert-verify"),
		},
	}
	n.Fingerprint = n.Key()
	return n, nil
}

// ---------- helpers ----------

func splitHostPort(hostport, defaultPort string) (string, string, error) {
	hostport = strings.TrimSpace(hostport)
	if hostport == "" {
		return "", "", fmt.Errorf("empty host")
	}
	// IPv6 in brackets
	if strings.HasPrefix(hostport, "[") {
		h, p, err := net.SplitHostPort(hostport)
		if err != nil {
			return "", "", err
		}
		return h, p, nil
	}
	if strings.Count(hostport, ":") == 1 {
		h, p, err := net.SplitHostPort(hostport)
		if err != nil {
			return "", "", err
		}
		return h, p, nil
	}
	// no port
	return hostport, defaultPort, nil
}

func anyToInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	case json.Number:
		n, _ := t.Int64()
		return int(n)
	default:
		return 0
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
