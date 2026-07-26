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

	"github.com/GALIAIS/NodeHarvest/internal/model"
)

// Options 拨测选项
type Options struct {
	// Bin sing-box 或 xray 可执行文件；空则自动查找
	Bin string
	// Engine force: sing-box | xray | auto
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
	_ = os.MkdirAll(opts.WorkDir, 0o700)

	bin, engine, err := resolveBinary(opts.Bin, opts.Engine)
	if err != nil {
		return nil, err
	}
	return &Dialer{opts: opts, bin: bin, engine: engine, next: opts.BasePort}, nil
}

func resolveBinary(bin, engine string) (string, string, error) {
	engine = strings.ToLower(strings.TrimSpace(engine))
	if bin != "" {
		if _, err := os.Stat(bin); err == nil {
			if engine == "" {
				base := strings.ToLower(filepath.Base(bin))
				if strings.Contains(base, "xray") {
					engine = "xray"
				} else {
					engine = "sing-box"
				}
			}
			return bin, engine, nil
		}
	}
	// PATH search preference: sing-box then xray
	candidates := []struct{ path, eng string }{
		{"sing-box", "sing-box"},
		{"xray", "xray"},
		{"/usr/local/bin/sing-box", "sing-box"},
		{"/usr/bin/sing-box", "sing-box"},
		{"/usr/local/bin/xray", "xray"},
		{"/opt/sing-box/sing-box", "sing-box"},
		{"/opt/nodeharvest/bin/sing-box", "sing-box"},
	}
	if engine == "xray" {
		candidates = []struct{ path, eng string }{
			{"xray", "xray"}, {"/usr/local/bin/xray", "xray"}, {"sing-box", "sing-box"},
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
	return "", "", fmt.Errorf("sing-box/xray binary not found; install sing-box or set dial.bin")
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
			select {
			case <-ctx.Done():
				n.Dial = &model.DialResult{OK: false, Error: "canceled", Engine: d.engine, TestedAt: time.Now()}
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			d.testOne(ctx, n)
			cur := int(done.Add(1))
			if d.opts.OnProgress != nil {
				d.opts.OnProgress(cur, total)
			}
		}()
	}
	wg.Wait()
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
			n.Tags = mergeTag(n.Tags, "verified")
		} else {
			n.Tags = mergeTag(n.Tags, "dial-fail")
		}
	}()

	if !Supports(n) {
		res.Error = "unsupported protocol"
		return
	}
	port := d.allocPort()
	cfgPath := filepath.Join(d.opts.WorkDir, fmt.Sprintf("sb-%d-%s.json", port, n.ID))
	logPath := cfgPath + ".log"
	cfg, args, err := d.coreConfig(n, port, logPath)
	if err != nil {
		res.Error = err.Error()
		return
	}
	raw, _ := json.Marshal(cfg)
	if err := os.WriteFile(cfgPath, raw, 0o600); err != nil {
		res.Error = "write config: " + err.Error()
		return
	}
	defer func() {
		_ = os.Remove(cfgPath)
		_ = os.Remove(logPath)
	}()

	cctx, cancel := context.WithTimeout(ctx, d.opts.Timeout)
	defer cancel()

	args = append(args, cfgPath)
	// #nosec G204 -- the operator-configured executable and generated config path are never derived from requests.
	cmd := exec.CommandContext(cctx, d.bin, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		res.Error = "start sing-box: " + err.Error()
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
		// attach last log lines if any
		// #nosec G304 -- logPath is generated inside the private per-probe work directory.
		if b, e := os.ReadFile(logPath); e == nil && len(b) > 0 {
			msg := string(b)
			if len(msg) > 200 {
				msg = msg[len(msg)-200:]
			}
			res.Error = res.Error + " | " + strings.TrimSpace(msg)
		}
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

func (d *Dialer) coreConfig(n *model.Node, port int, logPath string) (map[string]any, []string, error) {
	if d.engine == "xray" {
		outbound, err := BuildXrayOutbound(n)
		if err != nil {
			return nil, nil, err
		}
		return map[string]any{
			"log": map[string]any{"loglevel": "warning", "error": logPath},
			"inbounds": []any{map[string]any{
				"listen": "127.0.0.1", "port": port, "protocol": "socks",
				"settings": map[string]any{"auth": "noauth", "udp": true},
			}},
			"outbounds": []any{outbound},
		}, []string{"run", "-config"}, nil
	}
	outbound, err := BuildOutbound(n)
	if err != nil {
		return nil, nil, err
	}
	return map[string]any{
		"log": map[string]any{"level": "error", "output": logPath},
		"inbounds": []any{map[string]any{
			"type": "socks", "tag": "socks-in", "listen": "127.0.0.1", "listen_port": port,
		}},
		"outbounds": []any{outbound, map[string]any{"type": "direct", "tag": "direct"}},
		"route":     map[string]any{"final": "proxy"},
	}, []string{"run", "-c"}, nil
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

func mergeTag(tags []string, add string) []string {
	for _, t := range tags {
		if t == add {
			return tags
		}
	}
	return append(tags, add)
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
	return "run deploy/install-singbox.sh (pinned version and SHA-256), or set dial.bin to a verified sing-box/xray binary"
}

// ParseProxyURL unused helper
func ParseProxyURL(raw string) (*url.URL, error) {
	return url.Parse(raw)
}

// Available 快速检测环境
func Available() (bin, engine string, err error) {
	return resolveBinary("", "auto")
}

// MustPort free check helper for tests
func MustPort(p int) string {
	return strconv.Itoa(p)
}

// Logf debug
func Logf(format string, args ...any) {
	slog.Debug(fmt.Sprintf(format, args...))
}
