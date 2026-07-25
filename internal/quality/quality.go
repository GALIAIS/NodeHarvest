package quality

import (
	"context"
	"crypto/tls"
	"fmt"
	"math"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/local/node-hunter/internal/model"
)

// Options 智能测速选项
type Options struct {
	Concurrency int
	Timeout     time.Duration
	Rounds      int  // 每节点探测轮数
	TLSProbe    bool // 对 TLS 节点做握手
	EdgeProbe   bool // 对 AI/CDN 边缘做启发连通（本机直连，作参考）
	OnProgress  func(done, total int)
}

// Tester 质量探测
type Tester struct {
	opts Options
}

func New(opts Options) *Tester {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 64
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Second
	}
	if opts.Rounds <= 0 {
		opts.Rounds = 3
	}
	return &Tester{opts: opts}
}

// TestAll 并发质量探测
func (t *Tester) TestAll(ctx context.Context, nodes []*model.Node) {
	total := len(nodes)
	if total == 0 {
		return
	}
	sem := make(chan struct{}, t.opts.Concurrency)
	var wg sync.WaitGroup
	var done atomic.Int64

	for _, n := range nodes {
		n := n
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				n.Alive = false
				n.Error = "canceled"
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			t.testOne(ctx, n)
			cur := int(done.Add(1))
			if t.opts.OnProgress != nil {
				t.opts.OnProgress(cur, total)
			}
		}()
	}
	wg.Wait()
}

func (t *Tester) testOne(ctx context.Context, n *model.Node) {
	n.TestedAt = time.Now()
	q := &model.Quality{Rounds: t.opts.Rounds, Notes: []string{}}
	latencies := make([]time.Duration, 0, t.opts.Rounds)
	success := 0

	for i := 0; i < t.opts.Rounds; i++ {
		if ctx.Err() != nil {
			break
		}
		start := time.Now()
		d := net.Dialer{Timeout: t.opts.Timeout}
		cctx, cancel := context.WithTimeout(ctx, t.opts.Timeout)
		conn, err := d.DialContext(cctx, "tcp", n.Address())
		cancel()
		if err != nil {
			continue
		}
		lat := time.Since(start)
		_ = conn.Close()
		latencies = append(latencies, lat)
		success++
		// 轻微间隔，减少突发
		time.Sleep(30 * time.Millisecond)
	}

	q.SuccessRate = float64(success) / float64(t.opts.Rounds)
	if success == 0 {
		n.Alive = false
		n.Error = "all probes failed"
		n.Latency = t.opts.Timeout
		q.Score = 0
		n.Quality = q
		n.Score = 0
		n.Grade = "F"
		return
	}

	n.Alive = true
	n.Error = ""
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	q.MinLatencyMS = latencies[0].Milliseconds()
	q.MaxLatencyMS = latencies[len(latencies)-1].Milliseconds()
	var sum time.Duration
	for _, l := range latencies {
		sum += l
	}
	avg := sum / time.Duration(len(latencies))
	q.AvgLatencyMS = avg.Milliseconds()
	n.Latency = avg

	// 抖动：标准差近似
	var variance float64
	avgF := float64(avg.Milliseconds())
	for _, l := range latencies {
		d := float64(l.Milliseconds()) - avgF
		variance += d * d
	}
	q.JitterMS = int64(math.Sqrt(variance / float64(len(latencies))))

	// TLS 握手
	if t.opts.TLSProbe && n.TLS {
		tlsOK, tlsMS := t.probeTLS(ctx, n)
		q.TLSOK = tlsOK
		q.TLSMS = tlsMS
		if !tlsOK {
			q.Notes = append(q.Notes, "tls handshake failed")
		}
	} else if n.TLS {
		q.TLSOK = true // 未测时不扣分
	}

	// 边缘启发：本机到 AI 域名 443 的可达性（不经节点，作环境参考）
	if t.opts.EdgeProbe {
		q.EdgeScore = probeEdgeScore(ctx, t.opts.Timeout)
	} else {
		q.EdgeScore = 50
	}

	q.Score = computeScore(n, q)
	n.Quality = q
	n.Score = q.Score
	n.Grade = model.AssignGrade(q.Score)
	n.Tags = buildTags(n, q)
}

func (t *Tester) probeTLS(ctx context.Context, n *model.Node) (bool, int64) {
	start := time.Now()
	d := net.Dialer{Timeout: t.opts.Timeout}
	cctx, cancel := context.WithTimeout(ctx, t.opts.Timeout)
	defer cancel()
	conn, err := d.DialContext(cctx, "tcp", n.Address())
	if err != nil {
		return false, 0
	}
	defer conn.Close()
	serverName := n.SNI
	if serverName == "" {
		serverName = n.Server
	}
	tlsConn := tls.Client(conn, &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         serverName,
		MinVersion:         tls.VersionTLS12,
	})
	_ = tlsConn.SetDeadline(time.Now().Add(t.opts.Timeout))
	if err := tlsConn.HandshakeContext(cctx); err != nil {
		return false, time.Since(start).Milliseconds()
	}
	_ = tlsConn.Close()
	return true, time.Since(start).Milliseconds()
}

// computeScore 综合评分 0-100
func computeScore(n *model.Node, q *model.Quality) float64 {
	// 成功率 35%
	sr := q.SuccessRate * 35

	// 延迟 40%：50ms→满分，3000ms→0
	lat := float64(q.AvgLatencyMS)
	if lat < 1 {
		lat = 1
	}
	latScore := 40.0 * (1.0 / (1.0 + lat/500.0))
	// 再按绝对阈值微调
	if lat <= 80 {
		latScore = 40
	} else if lat >= 2500 {
		latScore = 2
	}

	// 抖动 10%
	jit := float64(q.JitterMS)
	jitScore := 10.0 * (1.0 / (1.0 + jit/80.0))

	// TLS / 协议加成 10%
	protoScore := 5.0
	switch n.Protocol {
	case model.ProtoVLESS, model.ProtoHysteria2, model.ProtoTUIC:
		protoScore = 10
	case model.ProtoTrojan, model.ProtoVMess:
		protoScore = 8
	case model.ProtoSS:
		protoScore = 6
	}
	if n.TLS {
		protoScore = math.Min(10, protoScore+2)
	}
	if n.TLS && !q.TLSOK && q.TLSMS > 0 {
		protoScore *= 0.5
	}

	// 边缘环境分 5%
	edge := q.EdgeScore / 100.0 * 5.0

	total := sr + latScore + jitScore + protoScore + edge
	if total > 100 {
		total = 100
	}
	if total < 0 {
		total = 0
	}
	return math.Round(total*10) / 10
}

func buildTags(n *model.Node, q *model.Quality) []string {
	tags := []string{string(n.Protocol)}
	if n.TLS {
		tags = append(tags, "tls")
	}
	if q.AvgLatencyMS > 0 && q.AvgLatencyMS < 150 {
		tags = append(tags, "low-latency")
	}
	if q.SuccessRate >= 1 {
		tags = append(tags, "stable")
	}
	if q.Score >= 80 {
		tags = append(tags, "premium")
	}
	if n.Security == "reality" {
		tags = append(tags, "reality")
	}
	return tags
}

// 常见 AI/CDN 边缘 host，用于本机环境启发
var edgeHosts = []string{
	"chatgpt.com:443",
	"gemini.google.com:443",
	"claude.ai:443",
	"grok.x.ai:443",
	"www.google.com:443",
	"www.cloudflare.com:443",
}

func probeEdgeScore(ctx context.Context, timeout time.Duration) float64 {
	ok := 0
	for _, addr := range edgeHosts {
		d := net.Dialer{Timeout: timeout}
		cctx, cancel := context.WithTimeout(ctx, timeout)
		conn, err := d.DialContext(cctx, "tcp", addr)
		cancel()
		if err == nil {
			ok++
			_ = conn.Close()
		}
	}
	return float64(ok) / float64(len(edgeHosts)) * 100
}

// Summary 文本摘要
func Summary(nodes []*model.Node) string {
	alive, hq := 0, 0
	var sum float64
	for _, n := range nodes {
		if n.Alive {
			alive++
			sum += n.Score
		}
		if n.Score >= 70 {
			hq++
		}
	}
	avg := 0.0
	if alive > 0 {
		avg = sum / float64(alive)
	}
	return fmt.Sprintf("alive=%d/%d high_quality=%d avg_score=%.1f", alive, len(nodes), hq, avg)
}
