package aiaccess

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GALIAIS/NodeHarvest/internal/model"
	"golang.org/x/net/proxy"
)

// Options AI 探测选项
type Options struct {
	Concurrency int
	Timeout     time.Duration
	// Socks5Addr 若设置（如 127.0.0.1:1080），经本地 SOCKS5 测 AI（需自行把节点导入 xray/sing-box）
	Socks5Addr string
	Targets    []model.AITarget
	// HeuristicTCP 无代理时：仅测节点到目标 host:443 的 TCP（粗粒度，非真实 HTTP）
	HeuristicTCP bool
	OnProgress   func(done, total int)
}

// Prober AI 可达性探测
type Prober struct {
	opts Options
}

func New(opts Options) *Prober {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 32
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 8 * time.Second
	}
	if len(opts.Targets) == 0 {
		opts.Targets = model.DefaultAITargets()
	}
	return &Prober{opts: opts}
}

// ProbeHost 探测运行环境（本机直连）对各 AI 站点的可达性
func (p *Prober) ProbeHost(ctx context.Context) map[string]*model.AIProbeResult {
	out := make(map[string]*model.AIProbeResult)
	client := p.httpClient(nil)
	for _, t := range p.opts.Targets {
		out[t.Key] = p.probeHTTP(ctx, client, t, "direct")
	}
	return out
}

// ProbeNodes 对节点做 AI 相关探测
// - 有 SOCKS5：经代理 HTTP 访问（真实）
// - 否则 HeuristicTCP：节点 IP 到目标域名解析后的 443 TCP（弱启发）
func (p *Prober) ProbeNodes(ctx context.Context, nodes []*model.Node) {
	// 仅对存活或高优先级节点测 AI，避免爆炸
	candidates := make([]*model.Node, 0, len(nodes))
	for _, n := range nodes {
		if n.Alive || n.Score >= 50 {
			candidates = append(candidates, n)
		}
	}
	if len(candidates) == 0 {
		candidates = nodes
	}

	total := len(candidates)
	if total == 0 {
		return
	}

	var client *http.Client
	mode := "heuristic"
	if p.opts.Socks5Addr != "" {
		if c, err := p.socksHTTPClient(p.opts.Socks5Addr); err == nil {
			client = c
			mode = "via_proxy"
		}
	}

	sem := make(chan struct{}, p.opts.Concurrency)
	var wg sync.WaitGroup
	var done atomic.Int64

	for _, n := range candidates {
		n := n
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			n.AIAccess = make(map[string]*model.AIProbeResult)
			if client != nil {
				for _, t := range p.opts.Targets {
					n.AIAccess[t.Key] = p.probeHTTP(ctx, client, t, mode)
				}
			} else if p.opts.HeuristicTCP {
				for _, t := range p.opts.Targets {
					n.AIAccess[t.Key] = p.probeNodeTCP(ctx, n, t)
				}
			}

			// AI 通过率加权进总分
			boostAIScore(n)

			cur := int(done.Add(1))
			if p.opts.OnProgress != nil {
				p.opts.OnProgress(cur, total)
			}
		}()
	}
	wg.Wait()
}

func boostAIScore(n *model.Node) {
	if n.AIAccess == nil || len(n.AIAccess) == 0 {
		return
	}
	ok := 0
	for _, r := range n.AIAccess {
		if r != nil && r.OK {
			ok++
		}
	}
	rate := float64(ok) / float64(len(n.AIAccess))
	// 最多 +15 分
	bonus := rate * 15
	n.Score = clamp(n.Score+bonus, 0, 100)
	n.Grade = model.AssignGrade(n.Score)
	if rate >= 0.75 {
		n.Tags = appendUnique(n.Tags, "ai-friendly")
	}
	if n.Quality != nil {
		n.Quality.Score = n.Score
		if rate > 0 {
			n.Quality.Notes = append(n.Quality.Notes, fmt.Sprintf("ai_pass=%.0f%%", rate*100))
		}
	}
}

func (p *Prober) probeHTTP(ctx context.Context, client *http.Client, t model.AITarget, mode string) *model.AIProbeResult {
	res := &model.AIProbeResult{Target: t.Key, URL: t.URL, Mode: mode}
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.URL, nil)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/122.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/json;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	res.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	res.StatusCode = resp.StatusCode
	// 2xx/3xx/401/403 通常表示网络层可达（WAF/登录页也算通）
	if resp.StatusCode > 0 && resp.StatusCode < 500 {
		res.OK = true
	} else {
		res.Error = fmt.Sprintf("http %d", resp.StatusCode)
	}
	return res
}

// probeNodeTCP：从运行环境解析 AI host，再测「节点地址本身」是否存活，
// 并用节点延迟+本机到 AI 的 TCP 组合成弱启发（无法证明经节点可访问 AI）。
func (p *Prober) probeNodeTCP(ctx context.Context, n *model.Node, t model.AITarget) *model.AIProbeResult {
	res := &model.AIProbeResult{
		Target: t.Key,
		URL:    t.URL,
		Mode:   "heuristic",
	}
	if !n.Alive {
		res.Error = "node not alive"
		return res
	}
	// 本机到 AI host:443
	start := time.Now()
	d := net.Dialer{Timeout: p.opts.Timeout}
	cctx, cancel := context.WithTimeout(ctx, p.opts.Timeout)
	defer cancel()
	conn, err := d.DialContext(cctx, "tcp", net.JoinHostPort(t.Host, "443"))
	res.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		res.Error = "edge unreachable from runner: " + err.Error()
		// 节点本身活着但本机到 AI 不通 → 节点可能仍有用，标记 partial
		if n.Score >= 70 {
			res.OK = false
			res.Error = "runner blocked; node quality high (unverified ai)"
		}
		return res
	}
	_ = conn.Close()

	// 启发：节点延迟低 + 本机能到 AI → 标记为 candidate（非实测经节点）
	if n.LatencyMS() > 0 && n.LatencyMS() < 1200 && n.Score >= 55 {
		res.OK = true
		res.Error = "heuristic candidate (not via-proxy verified)"
	} else {
		res.OK = false
		res.Error = "heuristic reject (latency/score)"
	}
	return res
}

func (p *Prober) httpClient(transport http.RoundTripper) *http.Client {
	if transport == nil {
		transport = &http.Transport{
			TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
			MaxIdleConns:        32,
			IdleConnTimeout:     30 * time.Second,
			TLSHandshakeTimeout: p.opts.Timeout,
			DisableKeepAlives:   true,
		}
	}
	return &http.Client{
		Timeout:   p.opts.Timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
}

func (p *Prober) socksHTTPClient(addr string) (*http.Client, error) {
	dialer, err := proxy.SOCKS5("tcp", addr, nil, proxy.Direct)
	if err != nil {
		return nil, err
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialer.Dial(network, address)
		},
		TLSClientConfig:   &tls.Config{MinVersion: tls.VersionTLS12},
		DisableKeepAlives: true,
	}
	return p.httpClient(tr), nil
}

// ParseSocksURL 解析 socks5://host:port
func ParseSocksURL(raw string) string {
	raw = trim(raw)
	if raw == "" {
		return ""
	}
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}

func trim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	return s
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func appendUnique(ss []string, v string) []string {
	for _, s := range ss {
		if s == v {
			return ss
		}
	}
	return append(ss, v)
}
