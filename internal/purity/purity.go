// Package purity 经真实代理出口检测 IP 纯净度与 Cloudflare 挑战启发式结果。
// 无法代替真人完成 Turnstile/验证码；仅判断是否出挑战页、以及公开风险库标记。
package purity

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/proxy"

	"github.com/GALIAIS/NodeHarvest/internal/dialer"
	"github.com/GALIAIS/NodeHarvest/internal/model"
)

// Options 探测选项
type Options struct {
	Bin         string
	Engine      string
	Concurrency int
	Timeout     time.Duration
	WorkDir     string
	BasePort    int
	// MaxNodes 最多测多少；0=不限
	MaxNodes   int
	OnProgress func(done, total int)
}

// Prober 纯净度探测
type Prober struct {
	opts   Options
	bin    string
	engine string
	muPort sync.Mutex
	next   int
}

func New(opts Options) (*Prober, error) {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 3
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 25 * time.Second
	}
	if opts.WorkDir == "" {
		opts.WorkDir = filepath.Join(os.TempDir(), "nodeharvest-purity")
	}
	if opts.BasePort <= 0 {
		opts.BasePort = 21000
	}
	if err := os.MkdirAll(opts.WorkDir, 0o700); err != nil {
		return nil, fmt.Errorf("create purity work directory: %w", err)
	}
	bin, engine, err := dialer.AvailableFor(opts.Bin, "sing-box")
	if err != nil {
		return nil, err
	}
	return &Prober{opts: opts, bin: bin, engine: engine, next: opts.BasePort}, nil
}

// TestAll 并发探测，写入 n.Purity
func (p *Prober) TestAll(ctx context.Context, nodes []*model.Node) {
	if len(nodes) == 0 {
		return
	}
	if p.opts.MaxNodes > 0 && len(nodes) > p.opts.MaxNodes {
		nodes = nodes[:p.opts.MaxNodes]
	}
	sem := make(chan struct{}, p.opts.Concurrency)
	var wg sync.WaitGroup
	var done atomic.Int64
	total := len(nodes)
	for _, n := range nodes {
		n := n
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				n.Purity = &model.PurityResult{Error: "canceled", TestedAt: time.Now()}
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			p.testOne(ctx, n)
			cur := int(done.Add(1))
			if p.opts.OnProgress != nil {
				p.opts.OnProgress(cur, total)
			}
		}()
	}
	wg.Wait()
}

func (p *Prober) testOne(ctx context.Context, n *model.Node) {
	start := time.Now()
	res := &model.PurityResult{TestedAt: start}
	defer func() {
		n.Purity = res
		if res.OK && res.CleanScore >= 70 {
			n.Tags = mergeTag(n.Tags, "clean")
		}
		if res.CFHumanLikely {
			n.Tags = mergeTag(n.Tags, "cf-ok")
		}
		if res.CFChallenge == "hard" || res.CFChallenge == "blocked" {
			n.Tags = mergeTag(n.Tags, "cf-challenge")
		}
		if res.IsProxy || res.IsHosting {
			n.Tags = mergeTag(n.Tags, "datacenter")
		}
	}()

	if !dialer.SupportsEngine(n, "sing-box") {
		res.Error = "unsupported protocol"
		return
	}
	ob, err := dialer.BuildOutbound(n)
	if err != nil {
		res.Error = err.Error()
		return
	}

	port := p.allocPort()
	cfgPath := filepath.Join(p.opts.WorkDir, fmt.Sprintf("purity-%d-%s.json", port, n.ID))
	logPath := cfgPath + ".log"
	cfg := map[string]any{
		"log": map[string]any{"level": "error", "output": logPath},
		"inbounds": []any{
			map[string]any{
				"type": "socks", "tag": "socks-in",
				"listen": "127.0.0.1", "listen_port": port,
			},
		},
		"outbounds": []any{ob, map[string]any{"type": "direct", "tag": "direct"}},
		"route":     map[string]any{"final": "proxy"},
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

	cctx, cancel := context.WithTimeout(ctx, p.opts.Timeout)
	defer cancel()
	// #nosec G204 -- the discovered proxy executable and generated config path are not request-controlled.
	cmd := exec.CommandContext(cctx, p.bin, "run", "-c", cfgPath)
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

	if err := waitTCP(cctx, fmt.Sprintf("127.0.0.1:%d", port), 5*time.Second); err != nil {
		res.Error = "socks not ready: " + err.Error()
		return
	}

	client, err := socksClient(fmt.Sprintf("127.0.0.1:%d", port), 18*time.Second)
	if err != nil {
		res.Error = err.Error()
		return
	}

	// 1) Cloudflare trace → 出口 IP + CF 基础可达
	t0 := time.Now()
	traceBody, code, err := httpGet(cctx, client, "https://www.cloudflare.com/cdn-cgi/trace")
	res.LatencyMS = time.Since(t0).Milliseconds()
	if err == nil && code >= 200 && code < 400 {
		res.CFTraceOK = true
		res.OK = true
		if ip := parseTraceIP(traceBody); ip != "" {
			res.ExitIP = ip
		}
		res.Notes = append(res.Notes, "cf_trace_ok")
	} else {
		if err != nil {
			res.Notes = append(res.Notes, "cf_trace_err:"+trimErr(err))
		} else {
			res.Notes = append(res.Notes, fmt.Sprintf("cf_trace_http_%d", code))
		}
	}

	// 2) Cloudflare 首页：是否出挑战（启发式，非真人解题）
	chal := classifyCFChallenge(cctx, client)
	res.CFChallenge = chal
	res.CFHumanLikely = chal == "none" && res.CFTraceOK
	if chal == "none" {
		res.Notes = append(res.Notes, "cf_page_no_challenge")
	} else {
		res.Notes = append(res.Notes, "cf_challenge:"+chal)
	}

	// 3) 若还没拿到出口 IP，用 ipify
	if res.ExitIP == "" {
		if body, code, err := httpGet(cctx, client, "https://api.ipify.org?format=text"); err == nil && code == 200 {
			res.ExitIP = strings.TrimSpace(body)
			res.OK = true
		}
	}

	// 4) 第三方风险标记（ip-api 免费）
	if res.ExitIP != "" {
		enrichIPAPI(cctx, client, res)
	}

	// 5) 综合打分
	scoreClean, scoreRisk, grade, notes := scorePurity(res)
	res.CleanScore = scoreClean
	res.RiskScore = scoreRisk
	res.Grade = grade
	res.Notes = append(res.Notes, notes...)
	if res.ExitIP != "" || res.CFTraceOK {
		res.OK = true
	}
}

func enrichIPAPI(ctx context.Context, client *http.Client, res *model.PurityResult) {
	// 经代理访问，避免本机 IP 污染；fields 控制体积
	u := fmt.Sprintf("http://ip-api.com/json/%s?fields=status,message,country,countryCode,isp,as,proxy,hosting,mobile,query", res.ExitIP)
	body, code, err := httpGet(ctx, client, u)
	if err != nil || code != 200 {
		// 回退：不经代理查（仅查 IP 属性，不暴露本机为出口）
		direct := &http.Client{Timeout: 8 * time.Second}
		body, code, err = httpGet(ctx, direct, u)
		if err != nil || code != 200 {
			res.Notes = append(res.Notes, "ipapi_fail")
			return
		}
	}
	var m map[string]any
	if json.Unmarshal([]byte(body), &m) != nil {
		return
	}
	if st, _ := m["status"].(string); st != "success" {
		res.Notes = append(res.Notes, "ipapi:"+fmt.Sprint(m["message"]))
		return
	}
	if v, ok := m["countryCode"].(string); ok {
		res.Country = v
	}
	if v, ok := m["isp"].(string); ok {
		res.ISP = v
	}
	if v, ok := m["as"].(string); ok {
		res.AS = v
	}
	if v, ok := m["proxy"].(bool); ok {
		res.IsProxy = v
	}
	if v, ok := m["hosting"].(bool); ok {
		res.IsHosting = v
	}
	if v, ok := m["mobile"].(bool); ok {
		res.IsMobile = v
	}
	res.Notes = append(res.Notes, "ipapi_ok")
}

func scorePurity(res *model.PurityResult) (clean, risk int, grade string, notes []string) {
	clean = 75
	if res.IsProxy {
		clean -= 45
		notes = append(notes, "flag:proxy")
	}
	if res.IsHosting {
		clean -= 30
		notes = append(notes, "flag:hosting")
	}
	if res.IsMobile {
		clean += 10
		notes = append(notes, "flag:mobile")
	}
	if res.CFTraceOK {
		clean += 10
	} else {
		clean -= 15
	}
	switch res.CFChallenge {
	case "none":
		clean += 10
	case "soft":
		clean -= 15
		notes = append(notes, "cf:soft_challenge")
	case "hard":
		clean -= 35
		notes = append(notes, "cf:hard_challenge")
	case "blocked":
		clean -= 50
		notes = append(notes, "cf:blocked")
	case "error":
		clean -= 10
	}
	// 常见机房关键词
	isp := strings.ToLower(res.ISP + " " + res.AS)
	for _, kw := range []string{"amazon", "google cloud", "digitalocean", "linode", "ovh", "hetzner", "vultr", "contabo", "choopa", "m247", "datacamp"} {
		if strings.Contains(isp, kw) {
			clean -= 15
			notes = append(notes, "isp_dc:"+kw)
			break
		}
	}
	if clean < 0 {
		clean = 0
	}
	if clean > 100 {
		clean = 100
	}
	risk = 100 - clean
	switch {
	case clean >= 85:
		grade = "S"
	case clean >= 70:
		grade = "A"
	case clean >= 55:
		grade = "B"
	case clean >= 40:
		grade = "C"
	case clean >= 25:
		grade = "D"
	default:
		grade = "F"
	}
	return clean, risk, grade, notes
}

// classifyCFChallenge 访问 Cloudflare 相关页面，启发式判断挑战等级
func classifyCFChallenge(ctx context.Context, client *http.Client) string {
	// 使用 cloudflare 自身站点；若返回挑战平台脚本则判 soft/hard
	body, code, err := httpGet(ctx, client, "https://www.cloudflare.com/")
	if err != nil {
		// 再试一个常见受 CF 保护的公开页
		body2, code2, err2 := httpGet(ctx, client, "https://www.cloudflarestatus.com/")
		if err2 != nil {
			return "error"
		}
		body, code = body2, code2
	}
	low := strings.ToLower(body)
	switch {
	case code == 403 || code == 503:
		if strings.Contains(low, "cf-browser-verification") || strings.Contains(low, "challenge-platform") || strings.Contains(low, "turnstile") {
			return "hard"
		}
		return "blocked"
	case strings.Contains(low, "just a moment") || strings.Contains(low, "checking your browser") ||
		strings.Contains(low, "cf-browser-verification") || strings.Contains(low, "challenge-platform") ||
		strings.Contains(low, "turnstile") || strings.Contains(low, "_cf_chl"):
		if strings.Contains(low, "turnstile") || strings.Contains(low, "cf-challenge-running") {
			return "hard"
		}
		return "soft"
	case code >= 200 && code < 400:
		return "none"
	default:
		return "error"
	}
}

func parseTraceIP(body string) string {
	// ip=1.2.3.4
	re := regexp.MustCompile(`(?m)^ip=([0-9a-fA-F\.:]+)\s*$`)
	m := re.FindStringSubmatch(body)
	if len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func httpGet(ctx context.Context, client *http.Client, rawURL string) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/json,*/*")
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	return string(b), resp.StatusCode, nil
}

func socksClient(addr string, timeout time.Duration) (*http.Client, error) {
	d, err := proxy.SOCKS5("tcp", addr, nil, proxy.Direct)
	if err != nil {
		return nil, err
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, a string) (net.Conn, error) {
			return d.Dial(network, a)
		},
		DisableKeepAlives:     true,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 12 * time.Second,
	}
	return &http.Client{Transport: tr, Timeout: timeout}, nil
}

func waitTCP(ctx context.Context, addr string, maxWait time.Duration) error {
	deadline := time.Now().Add(maxWait)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout")
		}
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(80 * time.Millisecond):
		}
	}
}

func (p *Prober) allocPort() int {
	p.muPort.Lock()
	defer p.muPort.Unlock()
	for i := 0; i < 200; i++ {
		port := p.next
		p.next++
		if p.next > p.opts.BasePort+4000 {
			p.next = p.opts.BasePort
		}
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			continue
		}
		_ = ln.Close()
		return port
	}
	return p.opts.BasePort + int(time.Now().UnixNano()%2000)
}

func mergeTag(tags []string, add string) []string {
	for _, t := range tags {
		if t == add {
			return tags
		}
	}
	return append(tags, add)
}

func trimErr(err error) string {
	s := err.Error()
	if len(s) > 80 {
		return s[:80]
	}
	return s
}

// Summary 汇总
func Summary(nodes []*model.Node) string {
	ok, fail, cfOK, cleanA := 0, 0, 0, 0
	var sum int
	for _, n := range nodes {
		if n.Purity == nil {
			fail++
			continue
		}
		if n.Purity.OK {
			ok++
			sum += n.Purity.CleanScore
		} else {
			fail++
		}
		if n.Purity.CFHumanLikely {
			cfOK++
		}
		if n.Purity.CleanScore >= 70 {
			cleanA++
		}
	}
	avg := 0
	if ok > 0 {
		avg = sum / ok
	}
	return fmt.Sprintf("purity ok=%d fail=%d cf_ok=%d cleanA+=%d avg_clean=%d", ok, fail, cfOK, cleanA, avg)
}

// GradeCounts 统计
func GradeCounts(nodes []*model.Node) map[string]int {
	out := map[string]int{}
	for _, n := range nodes {
		if n.Purity == nil || n.Purity.Grade == "" {
			continue
		}
		out[n.Purity.Grade]++
	}
	return out
}

// ParseMax helper
func ParseMax(v string) int {
	n, _ := strconv.Atoi(v)
	return n
}
