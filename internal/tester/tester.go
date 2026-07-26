package tester

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GALIAIS/NodeHarvest/internal/model"
)

// Options 测活选项
type Options struct {
	Concurrency int
	Timeout     time.Duration
	// PreferTLSDial 对声明 TLS 的节点尝试 TLS 握手（更严格）
	PreferTLSDial bool
	OnProgress    func(done, total int)
}

// Tester 并发 TCP/TLS 可达性测试
type Tester struct {
	opts Options
}

func New(opts Options) *Tester {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 64
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 6 * time.Second
	}
	return &Tester{opts: opts}
}

// TestAll 原地填充 Alive / Latency / Error
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
	start := time.Now()
	n.TestedAt = start

	addr := n.Address()
	d := net.Dialer{Timeout: t.opts.Timeout}

	cctx, cancel := context.WithTimeout(ctx, t.opts.Timeout)
	defer cancel()

	conn, err := d.DialContext(cctx, "tcp", addr)
	if err != nil {
		n.Alive = false
		n.Error = err.Error()
		n.Latency = time.Since(start)
		return
	}
	defer conn.Close()

	// 可选 TLS 握手，同时验证证书链与节点声明的服务名。
	if t.opts.PreferTLSDial && n.TLS {
		serverName := n.SNI
		if serverName == "" {
			serverName = n.Server
		}
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName: serverName,
			MinVersion: tls.VersionTLS12,
		})
		_ = tlsConn.SetDeadline(time.Now().Add(t.opts.Timeout))
		if err := tlsConn.HandshakeContext(cctx); err != nil {
			// TLS 失败不代表完全不可用（部分节点端口仅 TCP 代理），记为半存活但降低分数
			n.Alive = true
			n.Error = "tls: " + err.Error()
			n.Latency = time.Since(start)
			n.Score = scoreOf(n.Latency, true, true)
			return
		}
		_ = tlsConn.Close()
	}

	n.Alive = true
	n.Error = ""
	n.Latency = time.Since(start)
	n.Score = scoreOf(n.Latency, false, n.TLS)
}

// scoreOf 简单质量分：延迟越低越好，TLS 失败略扣分，有 TLS 略加分
func scoreOf(lat time.Duration, tlsFail, hasTLS bool) float64 {
	ms := float64(lat.Milliseconds())
	if ms <= 0 {
		ms = 1
	}
	// 0~100，延迟 50ms 约 95 分，2000ms 约 20 分
	base := 100.0 * (1.0 / (1.0 + ms/400.0))
	if hasTLS {
		base += 3
	}
	if tlsFail {
		base -= 15
	}
	if base < 0 {
		base = 0
	}
	if base > 100 {
		base = 100
	}
	return base
}

// Summary 简要统计
func Summary(nodes []*model.Node) string {
	alive := 0
	var sum time.Duration
	for _, n := range nodes {
		if n.Alive {
			alive++
			sum += n.Latency
		}
	}
	avg := time.Duration(0)
	if alive > 0 {
		avg = sum / time.Duration(alive)
	}
	return fmt.Sprintf("alive=%d/%d avg_latency=%s", alive, len(nodes), avg.Round(time.Millisecond))
}
