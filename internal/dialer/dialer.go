package dialer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/proxy"

	"github.com/GALIAIS/NodeHarvest/internal/exporter"
	"github.com/GALIAIS/NodeHarvest/internal/model"
	"gopkg.in/yaml.v3"
)

// Options 拨测选项
type Options struct {
	// Bin sing-box、xray 或 mihomo 可执行文件；空则自动查找
	Bin string
	// Engine force: sing-box | xray | mihomo | auto
	Engine string
	// Concurrency 同时拉起的核心实例数（建议 2–8）
	Concurrency int
	// Timeout 单节点总超时
	Timeout time.Duration
	// TestURL 经代理访问的 URL
	TestURL string
	// DownloadBytes is the maximum body size consumed for throughput measurement.
	DownloadBytes int64
	// WorkDir 临时配置目录
	WorkDir string
	// BasePort 本地 socks 起始端口
	BasePort int
	// OnProgress 进度
	OnProgress func(done, total int)
}

// Dialer 真实协议拨测
type Dialer struct {
	opts   Options
	bin    string
	engine string
	muPort sync.Mutex
	next   int
}

func New(opts Options) (*Dialer, error) {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 4
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 18 * time.Second
	}
	if opts.TestURL == "" {
		opts.TestURL = "https://www.cloudflare.com/cdn-cgi/trace"
	}
	if opts.DownloadBytes <= 0 {
		opts.DownloadBytes = 256 << 10
	}
	if opts.WorkDir == "" {
		opts.WorkDir = filepath.Join(os.TempDir(), "nodeharvest-dial")
	}
	if opts.BasePort <= 0 {
		opts.BasePort = 19000
	}
	if err := os.MkdirAll(opts.WorkDir, 0o700); err != nil {
		return nil, fmt.Errorf("create dial work directory: %w", err)
	}

	bin, engine, err := resolveBinary(opts.Bin, opts.Engine)
	if err != nil {
		return nil, err
	}
	return &Dialer{opts: opts, bin: bin, engine: engine, next: opts.BasePort}, nil
}

func resolveBinary(bin, engine string) (string, string, error) {
	engine = strings.ToLower(strings.TrimSpace(engine))
	if engine == "auto" {
		engine = ""
	}
	switch engine {
	case "", "sing-box", "xray", "mihomo":
	default:
		return "", "", fmt.Errorf("unsupported dial engine %q", engine)
	}
	if bin != "" {
		if _, err := os.Stat(bin); err == nil {
			if engine == "" {
				base := strings.ToLower(filepath.Base(bin))
				switch {
				case strings.Contains(base, "mihomo"), strings.Contains(base, "clash-meta"):
					engine = "mihomo"
				case strings.Contains(base, "xray"):
					engine = "xray"
				default:
					engine = "sing-box"
				}
			}
			return bin, engine, nil
		}
		return "", "", fmt.Errorf("%s binary not found at %s", firstNonEmpty(engine, "dial"), bin)
	}
	candidates := []struct{ path, eng string }{
		{"sing-box", "sing-box"},
		{"mihomo", "mihomo"},
		{"clash-meta", "mihomo"},
		{"xray", "xray"},
		{"/usr/local/bin/sing-box", "sing-box"},
		{"/usr/bin/sing-box", "sing-box"},
		{"/usr/local/bin/mihomo", "mihomo"},
		{"/usr/bin/mihomo", "mihomo"},
		{"/usr/local/bin/xray", "xray"},
		{"/opt/sing-box/sing-box", "sing-box"},
		{"/opt/nodeharvest/bin/sing-box", "sing-box"},
		{"/opt/nodeharvest/bin/mihomo", "mihomo"},
		{"/app/bin/sing-box", "sing-box"},
		{"/app/bin/mihomo", "mihomo"},
	}
	switch engine {
	case "xray":
		candidates = []struct{ path, eng string }{
			{"xray", "xray"}, {"/usr/local/bin/xray", "xray"}, {"/usr/bin/xray", "xray"},
			{"/opt/nodeharvest/bin/xray", "xray"}, {"/app/bin/xray", "xray"},
		}
	case "mihomo":
		candidates = []struct{ path, eng string }{
			{"mihomo", "mihomo"}, {"clash-meta", "mihomo"}, {"/usr/local/bin/mihomo", "mihomo"},
			{"/usr/bin/mihomo", "mihomo"}, {"/opt/nodeharvest/bin/mihomo", "mihomo"}, {"/app/bin/mihomo", "mihomo"},
		}
	case "sing-box":
		candidates = []struct{ path, eng string }{
			{"sing-box", "sing-box"}, {"/usr/local/bin/sing-box", "sing-box"}, {"/usr/bin/sing-box", "sing-box"},
			{"/opt/sing-box/sing-box", "sing-box"}, {"/opt/nodeharvest/bin/sing-box", "sing-box"}, {"/app/bin/sing-box", "sing-box"},
		}
	}
	for _, c := range candidates {
		if p, err := exec.LookPath(c.path); err == nil {
			return p, c.eng, nil
		}
		if _, err := os.Stat(c.path); err == nil {
			return c.path, c.eng, nil
		}
	}
	return "", "", fmt.Errorf("%s binary not found", firstNonEmpty(engine, "sing-box/mihomo/xray"))
}

func (d *Dialer) Engine() string { return d.engine }
func (d *Dialer) Bin() string    { return d.bin }

func (d *Dialer) allocPort() int {
	d.muPort.Lock()
	defer d.muPort.Unlock()
	for i := 0; i < 200; i++ {
		p := d.next
		d.next++
		if d.next > d.opts.BasePort+5000 {
			d.next = d.opts.BasePort
		}
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			continue
		}
		_ = ln.Close()
		return p
	}
	return d.opts.BasePort + int(time.Now().UnixNano()%3000)
}

// TestAll 并发拨测，原地写 n.Dial / n.Verified
func (d *Dialer) TestAll(ctx context.Context, nodes []*model.Node) {
	total := len(nodes)
	if total == 0 {
		return
	}
	sem := make(chan struct{}, d.opts.Concurrency)
	var wg sync.WaitGroup
	var done atomic.Int64

	for _, n := range nodes {
		n := n
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				cur := int(done.Add(1))
				if d.opts.OnProgress != nil {
					d.opts.OnProgress(cur, total)
				}
			}()
			select {
			case <-ctx.Done():
				markCanceled(n, d.engine)
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			if ctx.Err() != nil {
				markCanceled(n, d.engine)
				return
			}

			d.testOne(ctx, n)
		}()
	}
	wg.Wait()
}

func markCanceled(n *model.Node, engine string) {
	n.Dial = &model.DialResult{OK: false, Error: "canceled", Engine: engine, TestedAt: time.Now()}
	n.Verified = false
	n.Tags = replaceTag(n.Tags, "verified", "dial-fail")
}

func (d *Dialer) testOne(ctx context.Context, n *model.Node) {
	start := time.Now()
	res := &model.DialResult{
		Target:   d.opts.TestURL,
		Engine:   d.engine,
		TestedAt: start,
	}
	defer func() {
		n.Dial = res
		n.Verified = res.OK
		if res.OK {
			// 用真测延迟修正
			if res.LatencyMS > 0 {
				n.Latency = time.Duration(res.LatencyMS) * time.Millisecond
			}
			n.Alive = true
			n.Tags = replaceTag(n.Tags, "dial-fail", "verified")
		} else {
			n.Tags = replaceTag(n.Tags, "verified", "dial-fail")
		}
	}()

	if !SupportsEngine(n, d.engine) {
		res.Error = "unsupported protocol"
		return
	}
	port := d.allocPort()
	probeDir, err := os.MkdirTemp(d.opts.WorkDir, fmt.Sprintf("%s-%d-", d.engine, port))
	if err != nil {
		res.Error = "create probe directory: " + err.Error()
		return
	}
	defer os.RemoveAll(probeDir)
	ext := ".json"
	if d.engine == "mihomo" {
		ext = ".yaml"
	}
	cfgPath := filepath.Join(probeDir, "config"+ext)
	logPath := filepath.Join(probeDir, "core.log")
	defer appendCoreError(res, logPath)
	raw, err := d.coreConfig(n, port, logPath)
	if err != nil {
		res.Error = err.Error()
		return
	}
	if err := os.WriteFile(cfgPath, raw, 0o600); err != nil {
		res.Error = "write config: " + err.Error()
		return
	}

	cctx, cancel := context.WithTimeout(ctx, d.opts.Timeout)
	defer cancel()

	var args []string
	switch d.engine {
	case "xray":
		args = []string{"run", "-config", cfgPath}
	case "mihomo":
		args = []string{"-d", probeDir, "-f", cfgPath}
	default:
		args = []string{"run", "-c", cfgPath}
	}
	// #nosec G204 -- the operator-configured executable and generated config path are never derived from requests.
	cmd := exec.CommandContext(cctx, d.bin, args...)
	cmd.Stdout = io.Discard
	// #nosec G304 -- logPath is fixed beneath the private directory returned by os.MkdirTemp above.
	logFile, logErr := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if logErr == nil {
		defer logFile.Close()
		cmd.Stderr = logFile
	} else {
		cmd.Stderr = io.Discard
	}
	if err := cmd.Start(); err != nil {
		res.Error = "start " + d.engine + ": " + err.Error()
		return
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	// wait socks ready
	deadline := time.Now().Add(5 * time.Second)
	for {
		if time.Now().After(deadline) {
			res.Error = "socks not ready"
			return
		}
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
		select {
		case <-cctx.Done():
			res.Error = "timeout waiting socks"
			return
		case <-time.After(80 * time.Millisecond):
		}
	}

	// HTTP via SOCKS5
	dialerSocks, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", port), nil, proxy.Direct)
	if err != nil {
		res.Error = "socks dialer: " + err.Error()
		return
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if contextDialer, ok := dialerSocks.(proxy.ContextDialer); ok {
				return contextDialer.DialContext(ctx, network, addr)
			}
			return dialerSocks.Dial(network, addr)
		},
		// avoid keep-alive across tests
		DisableKeepAlives:     true,
		TLSHandshakeTimeout:   8 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   12 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(cctx, http.MethodGet, d.opts.TestURL, nil)
	if err != nil {
		res.Error = err.Error()
		return
	}
	req.Header.Set("User-Agent", "nodeharvest-dial/2.0")

	measurement, err := measureHTTP(client, req, d.opts.DownloadBytes)
	res.LatencyMS = measurement.HeaderMS
	res.HTTPMS = measurement.TotalMS
	res.DownloadBytes = measurement.Bytes
	res.ThroughputBPS = measurement.ThroughputBPS
	if err != nil {
		res.Error = err.Error()
		return
	}
	res.StatusCode = measurement.StatusCode
	// 2xx/3xx 视为代理链路可用
	if measurement.StatusCode >= 200 && measurement.StatusCode < 400 {
		res.OK = true
		return
	}
	res.Error = fmt.Sprintf("http %d", measurement.StatusCode)
}

func (d *Dialer) coreConfig(n *model.Node, port int, logPath string) ([]byte, error) {
	if d.engine == "mihomo" {
		proxyConfig, err := exporter.BuildClashProxy(n, "nodeharvest-proxy")
		if err != nil {
			return nil, err
		}
		return yaml.Marshal(map[string]any{
			"mixed-port":    port,
			"allow-lan":     false,
			"mode":          "rule",
			"log-level":     "error",
			"ipv6":          true,
			"unified-delay": true,
			"proxies":       []any{proxyConfig},
			"proxy-groups": []any{map[string]any{
				"name": "nodeharvest-probe", "type": "select", "proxies": []string{"nodeharvest-proxy"},
			}},
			"rules": []string{"MATCH,nodeharvest-probe"},
		})
	}
	var config map[string]any
	if d.engine == "xray" {
		outbound, err := BuildXrayOutbound(n)
		if err != nil {
			return nil, err
		}
		config = map[string]any{
			"log": map[string]any{"loglevel": "warning", "error": logPath},
			"inbounds": []any{map[string]any{
				"listen": "127.0.0.1", "port": port, "protocol": "socks",
				"settings": map[string]any{"auth": "noauth", "udp": true},
			}},
			"outbounds": []any{outbound},
		}
		raw, err := json.Marshal(config)
		return raw, err
	}
	outbound, err := BuildOutbound(n)
	if err != nil {
		return nil, err
	}
	config = map[string]any{
		"log": map[string]any{"level": "error", "output": logPath},
		"inbounds": []any{map[string]any{
			"type": "socks", "tag": "socks-in", "listen": "127.0.0.1", "listen_port": port,
		}},
		"outbounds": []any{outbound, map[string]any{"type": "direct", "tag": "direct"}},
		"route":     map[string]any{"final": "proxy"},
	}
	raw, err := json.Marshal(config)
	return raw, err
}

type httpMeasurement struct {
	StatusCode    int
	HeaderMS      int64
	TotalMS       int64
	Bytes         int64
	ThroughputBPS int64
}

func measureHTTP(client *http.Client, req *http.Request, maxBytes int64) (httpMeasurement, error) {
	var measurement httpMeasurement
	started := time.Now()
	// #nosec G704 -- req targets the operator-configured probe URL and is sent through the isolated proxy under test.
	resp, err := client.Do(req)
	measurement.HeaderMS = time.Since(started).Milliseconds()
	if err != nil {
		measurement.TotalMS = measurement.HeaderMS
		return measurement, err
	}
	defer resp.Body.Close()
	measurement.StatusCode = resp.StatusCode
	bodyStarted := time.Now()
	measurement.Bytes, err = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBytes))
	bodyDuration := time.Since(bodyStarted)
	measurement.TotalMS = time.Since(started).Milliseconds()
	if measurement.Bytes > 0 {
		elapsedNS := max(bodyDuration.Nanoseconds(), 1)
		measurement.ThroughputBPS = measurement.Bytes * int64(time.Second) / elapsedNS
	}
	return measurement, err
}

func replaceTag(tags []string, remove, add string) []string {
	out := tags[:0]
	for _, tag := range tags {
		if tag != remove && tag != add {
			out = append(out, tag)
		}
	}
	return append(out, add)
}

func appendCoreError(result *model.DialResult, logPath string) {
	if result == nil || result.OK || result.Error == "" {
		return
	}
	// #nosec G304 -- logPath is generated inside the private per-probe work directory.
	body, err := os.ReadFile(logPath)
	if err != nil || len(body) == 0 {
		return
	}
	if len(body) > 500 {
		body = body[len(body)-500:]
	}
	if message := strings.TrimSpace(string(body)); message != "" && !strings.Contains(result.Error, message) {
		result.Error += " | " + message
	}
}

// Summary 统计
func Summary(nodes []*model.Node) string {
	ok, fail, skip := 0, 0, 0
	var sum int64
	for _, n := range nodes {
		if n.Dial == nil {
			skip++
			continue
		}
		if n.Dial.OK {
			ok++
			sum += n.Dial.LatencyMS
		} else {
			fail++
		}
	}
	avg := int64(0)
	if ok > 0 {
		avg = sum / int64(ok)
	}
	return fmt.Sprintf("dial ok=%d fail=%d skip=%d avg_ms=%d", ok, fail, skip, avg)
}

// InstallHint 提示
func InstallHint() string {
	return "use the bundled image, or set dial.bin/dial.mihomo_bin to verified sing-box, xray, or Mihomo binaries"
}

// ParseProxyURL unused helper
func ParseProxyURL(raw string) (*url.URL, error) {
	return url.Parse(raw)
}

// Available 快速检测环境
func Available() (bin, engine string, err error) {
	return resolveBinary("", "auto")
}

// AvailableFor resolves the configured binary and engine.
func AvailableFor(bin, engine string) (resolvedBin, resolvedEngine string, err error) {
	return resolveBinary(bin, engine)
}

// MustPort free check helper for tests
func MustPort(p int) string {
	return strconv.Itoa(p)
}

// Logf debug
func Logf(format string, args ...any) {
	slog.Debug(fmt.Sprintf(format, args...))
}
