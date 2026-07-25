package parser

import (
	"encoding/base64"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/local/node-hunter/internal/model"
)

var (
	// 匹配常见协议 URI
	uriRe = regexp.MustCompile(`(?i)((?:vmess|vless|trojan|ss|ssr|hysteria2|hy2|tuic)://[^\s<>"']+)`)
	// 行内 base64 粗判
	b64Re = regexp.MustCompile(`^[A-Za-z0-9+/=\s\r\n]+$`)
)

// ParseContent 从订阅正文或混合文本中提取节点
func ParseContent(content, source string) []*model.Node {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}

	// Clash / Mihomo YAML 订阅
	if looksLikeClash(content) {
		if nodes := ParseClash(content, source); len(nodes) > 0 {
			return nodes
		}
	}

	// 尝试整体 base64 解码（标准订阅）
	if decoded, ok := tryDecodeBase64(content); ok {
		content = decoded
		if looksLikeClash(content) {
			if nodes := ParseClash(content, source); len(nodes) > 0 {
				return nodes
			}
		}
	}

	// 统一换行，并再扫一遍可能嵌套的 base64 行
	lines := splitLines(content)
	var expanded []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		if !looksLikeURI(line) {
			if decoded, ok := tryDecodeBase64(line); ok && looksLikeURI(decoded) {
				expanded = append(expanded, splitLines(decoded)...)
				continue
			}
		}
		expanded = append(expanded, line)
	}

	// 从全文再捞一次 URI（应对 HTML / markdown）
	for _, m := range uriRe.FindAllString(content, -1) {
		expanded = append(expanded, m)
	}

	seenURI := make(map[string]struct{})
	nodes := make([]*model.Node, 0, len(expanded))
	for _, line := range expanded {
		line = strings.TrimSpace(line)
		line = strings.Trim(line, "`\"'")
		if line == "" {
			continue
		}
		// 清理尾部标点
		line = strings.TrimRightFunc(line, func(r rune) bool {
			return r == ',' || r == ';' || r == ')' || r == ']' || unicode.IsSpace(r)
		})
		if _, ok := seenURI[line]; ok {
			continue
		}
		seenURI[line] = struct{}{}

		n, err := ParseURI(line, source)
		if err != nil || n == nil {
			continue
		}
		nodes = append(nodes, n)
	}
	return nodes
}

// ParseURI 解析单条节点 URI
func ParseURI(raw, source string) (*model.Node, error) {
	raw = strings.TrimSpace(raw)
	lower := strings.ToLower(raw)

	switch {
	case strings.HasPrefix(lower, "vmess://"):
		return parseVMess(raw, source)
	case strings.HasPrefix(lower, "vless://"):
		return parseVLESS(raw, source)
	case strings.HasPrefix(lower, "trojan://"):
		return parseTrojan(raw, source)
	case strings.HasPrefix(lower, "ss://"):
		return parseSS(raw, source)
	case strings.HasPrefix(lower, "ssr://"):
		return parseSSR(raw, source)
	case strings.HasPrefix(lower, "hysteria2://"), strings.HasPrefix(lower, "hy2://"):
		return parseHysteria2(raw, source)
	case strings.HasPrefix(lower, "tuic://"):
		return parseTUIC(raw, source)
	default:
		return nil, errUnsupported
	}
}

var errUnsupported = &parseError{msg: "unsupported scheme"}

type parseError struct{ msg string }

func (e *parseError) Error() string { return e.msg }

func looksLikeURI(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	prefixes := []string{"vmess://", "vless://", "trojan://", "ss://", "ssr://", "hysteria2://", "hy2://", "tuic://"}
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Split(s, "\n")
}

func tryDecodeBase64(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" || looksLikeURI(s) {
		return "", false
	}
	// 去掉空白
	compact := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
	if len(compact) < 16 || !b64Re.MatchString(compact) {
		return "", false
	}
	// 补 padding
	if m := len(compact) % 4; m != 0 {
		compact += strings.Repeat("=", 4-m)
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(compact); err == nil {
			out := string(b)
			// 解码结果应是可打印文本
			if isMostlyPrintable(out) {
				return out, true
			}
		}
	}
	return "", false
}

func isMostlyPrintable(s string) bool {
	if s == "" {
		return false
	}
	bad := 0
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		if r < 32 || r == 127 {
			bad++
		}
	}
	return bad*10 < len(s) // 允许少量噪声
}

func decodeMaybeBase64(s string) string {
	if d, ok := tryDecodeBase64(s); ok {
		return d
	}
	return s
}

func parseNameFromFragment(u *url.URL, fallback string) string {
	if u == nil {
		return fallback
	}
	if u.Fragment != "" {
		if name, err := url.QueryUnescape(u.Fragment); err == nil && name != "" {
			return name
		}
		return u.Fragment
	}
	return fallback
}

func atoiDefault(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func queryGet(q url.Values, keys ...string) string {
	for _, k := range keys {
		if v := q.Get(k); v != "" {
			return v
		}
	}
	return ""
}

func truthy(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "1" || s == "true" || s == "yes" || s == "tls" || s == "reality"
}
