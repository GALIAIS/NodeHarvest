package middleware

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// TokenBucket 简易按 key 限流
type TokenBucket struct {
	mu       sync.Mutex
	rate     float64 // tokens per second
	burst    float64
	visitors map[string]*visitor
}

type visitor struct {
	tokens float64
	last   time.Time
}

func NewTokenBucket(rps float64, burst int) *TokenBucket {
	if rps <= 0 {
		rps = 10
	}
	if burst <= 0 {
		burst = 20
	}
	tb := &TokenBucket{
		rate:     rps,
		burst:    float64(burst),
		visitors: map[string]*visitor{},
	}
	go tb.cleanup()
	return tb
}

func (tb *TokenBucket) cleanup() {
	t := time.NewTicker(5 * time.Minute)
	for range t.C {
		tb.mu.Lock()
		cutoff := time.Now().Add(-10 * time.Minute)
		for k, v := range tb.visitors {
			if v.last.Before(cutoff) {
				delete(tb.visitors, k)
			}
		}
		tb.mu.Unlock()
	}
}

func (tb *TokenBucket) Allow(key string) bool {
	return tb.AllowRate(key, tb.rate, int(tb.burst))
}

func (tb *TokenBucket) AllowRate(key string, rps float64, burst int) bool {
	if rps <= 0 {
		rps = tb.rate
	}
	if burst <= 0 {
		burst = int(tb.burst)
	}
	tb.mu.Lock()
	defer tb.mu.Unlock()
	v, ok := tb.visitors[key]
	now := time.Now()
	if !ok {
		tb.visitors[key] = &visitor{tokens: float64(burst - 1), last: now}
		return true
	}
	elapsed := now.Sub(v.last).Seconds()
	v.tokens += elapsed * rps
	if v.tokens > float64(burst) {
		v.tokens = float64(burst)
	}
	v.last = now
	if v.tokens < 1 {
		return false
	}
	v.tokens--
	return true
}

func ClientIP(r *http.Request, trustedProxies []string) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	remote, err := netip.ParseAddr(strings.TrimSpace(host))
	if err != nil || !trustedProxy(remote, trustedProxies) {
		return host
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			ip, err := netip.ParseAddr(strings.TrimSpace(parts[i]))
			if err != nil {
				continue
			}
			if !trustedProxy(ip, trustedProxies) || i == 0 {
				return ip.String()
			}
		}
	}
	for _, raw := range []string{r.Header.Get("CF-Connecting-IP"), r.Header.Get("X-Real-IP")} {
		if ip, err := netip.ParseAddr(strings.TrimSpace(raw)); err == nil {
			return ip.String()
		}
	}
	return remote.String()
}

func trustedProxy(ip netip.Addr, entries []string) bool {
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if prefix, err := netip.ParsePrefix(entry); err == nil && prefix.Contains(ip) {
			return true
		}
		if addr, err := netip.ParseAddr(entry); err == nil && addr == ip {
			return true
		}
	}
	return false
}

func (tb *TokenBucket) Middleware(next http.Handler, trustedProxies []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := ClientIP(r, trustedProxies)
		// 订阅路径叠加 token 维度
		if strings.Contains(r.URL.Path, "/sub") {
			if t := r.URL.Query().Get("token"); t != "" {
				if len(t) > 8 {
					key = key + "|" + t[:8]
				} else {
					key = key + "|" + t
				}
			}
		}
		if !tb.Allow(key) {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
