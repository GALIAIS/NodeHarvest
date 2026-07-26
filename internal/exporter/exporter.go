package exporter

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GALIAIS/NodeHarvest/internal/config"
	"github.com/GALIAIS/NodeHarvest/internal/model"
	"github.com/GALIAIS/NodeHarvest/internal/timex"
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
	for _, n := range nodes {
		if n.RawURI == "" {
			continue
		}
		b.WriteString(n.RawURI)
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
	var b strings.Builder
	for _, n := range nodes {
		uri := n.RawURI
		if uri == "" {
			continue
		}
		b.WriteString(uri)
		b.WriteByte('\n')
	}
	return writePrivateFile(path, []byte(b.String()))
}

func writeBase64(path string, nodes []*model.Node) error {
	var b strings.Builder
	for _, n := range nodes {
		if n.RawURI == "" {
			continue
		}
		b.WriteString(n.RawURI)
		b.WriteByte('\n')
	}
	enc := base64.StdEncoding.EncodeToString([]byte(b.String()))
	return writePrivateFile(path, []byte(enc))
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

// writeClash 导出 Clash Meta 可用的 proxies 列表片段
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
	var b strings.Builder
	b.WriteString("# Generated by nodeharvest\n")
	b.WriteString("# Paste under `proxies:` in your Clash config\n")
	b.WriteString("proxies:\n")
	for i, n := range nodes {
		name := sanitizeYAMLName(n.Name, i)
		switch n.Protocol {
		case model.ProtoVMess:
			b.WriteString(fmt.Sprintf("  - name: %s\n", yamlQuote(name)))
			b.WriteString("    type: vmess\n")
			b.WriteString(fmt.Sprintf("    server: %s\n", yamlQuote(n.Server)))
			b.WriteString(fmt.Sprintf("    port: %d\n", n.Port))
			b.WriteString(fmt.Sprintf("    uuid: %s\n", yamlQuote(n.UUID)))
			b.WriteString("    alterId: 0\n")
			b.WriteString(fmt.Sprintf("    cipher: %s\n", yamlQuote(first(n.Method, "auto"))))
			b.WriteString(fmt.Sprintf("    network: %s\n", yamlQuote(first(n.Network, "tcp"))))
			if n.TLS {
				b.WriteString("    tls: true\n")
				if n.SkipTLSVerify() {
					b.WriteString("    skip-cert-verify: true\n")
				}
			}
			if n.SNI != "" {
				b.WriteString(fmt.Sprintf("    servername: %s\n", yamlQuote(n.SNI)))
			}
			if n.Path != "" || n.Host != "" {
				b.WriteString("    ws-opts:\n")
				if n.Path != "" {
					b.WriteString(fmt.Sprintf("      path: %s\n", yamlQuote(n.Path)))
				}
				if n.Host != "" {
					b.WriteString("      headers:\n")
					b.WriteString(fmt.Sprintf("        Host: %s\n", yamlQuote(n.Host)))
				}
			}
		case model.ProtoVLESS:
			b.WriteString(fmt.Sprintf("  - name: %s\n", yamlQuote(name)))
			b.WriteString("    type: vless\n")
			b.WriteString(fmt.Sprintf("    server: %s\n", yamlQuote(n.Server)))
			b.WriteString(fmt.Sprintf("    port: %d\n", n.Port))
			b.WriteString(fmt.Sprintf("    uuid: %s\n", yamlQuote(n.UUID)))
			b.WriteString(fmt.Sprintf("    network: %s\n", yamlQuote(first(n.Network, "tcp"))))
			if n.TLS || n.Security == "tls" || n.Security == "reality" {
				b.WriteString("    tls: true\n")
				if n.SkipTLSVerify() {
					b.WriteString("    skip-cert-verify: true\n")
				}
			}
			if n.SNI != "" {
				b.WriteString(fmt.Sprintf("    servername: %s\n", yamlQuote(n.SNI)))
			}
			if n.Flow != "" {
				b.WriteString(fmt.Sprintf("    flow: %s\n", yamlQuote(n.Flow)))
			}
			if n.Security == "reality" {
				b.WriteString("    reality-opts:\n")
				if n.Extra != nil {
					if v := n.Extra["pbk"]; v != "" {
						b.WriteString(fmt.Sprintf("      public-key: %s\n", yamlQuote(v)))
					}
					if v := n.Extra["sid"]; v != "" {
						b.WriteString(fmt.Sprintf("      short-id: %s\n", yamlQuote(v)))
					}
				}
			}
		case model.ProtoTrojan:
			b.WriteString(fmt.Sprintf("  - name: %s\n", yamlQuote(name)))
			b.WriteString("    type: trojan\n")
			b.WriteString(fmt.Sprintf("    server: %s\n", yamlQuote(n.Server)))
			b.WriteString(fmt.Sprintf("    port: %d\n", n.Port))
			b.WriteString(fmt.Sprintf("    password: %s\n", yamlQuote(n.Password)))
			if n.SNI != "" {
				b.WriteString(fmt.Sprintf("    sni: %s\n", yamlQuote(n.SNI)))
			}
			if n.SkipTLSVerify() {
				b.WriteString("    skip-cert-verify: true\n")
			}
		case model.ProtoSS:
			b.WriteString(fmt.Sprintf("  - name: %s\n", yamlQuote(name)))
			b.WriteString("    type: ss\n")
			b.WriteString(fmt.Sprintf("    server: %s\n", yamlQuote(n.Server)))
			b.WriteString(fmt.Sprintf("    port: %d\n", n.Port))
			b.WriteString(fmt.Sprintf("    cipher: %s\n", yamlQuote(n.Method)))
			b.WriteString(fmt.Sprintf("    password: %s\n", yamlQuote(n.Password)))
		case model.ProtoHysteria2:
			b.WriteString(fmt.Sprintf("  - name: %s\n", yamlQuote(name)))
			b.WriteString("    type: hysteria2\n")
			b.WriteString(fmt.Sprintf("    server: %s\n", yamlQuote(n.Server)))
			b.WriteString(fmt.Sprintf("    port: %d\n", n.Port))
			b.WriteString(fmt.Sprintf("    password: %s\n", yamlQuote(n.Password)))
			if n.SNI != "" {
				b.WriteString(fmt.Sprintf("    sni: %s\n", yamlQuote(n.SNI)))
			}
			if n.SkipTLSVerify() {
				b.WriteString("    skip-cert-verify: true\n")
			}
		default:
			// SSR / TUIC 等 Clash 字段差异大，跳过 YAML，仍保留在 raw 中
			continue
		}
	}
	return b.String()
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

func yamlQuote(value string) string {
	return strconv.Quote(value)
}
