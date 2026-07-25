package middleware

import (
	"net"
	"net/http"
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
	tb.mu.Lock()
	defer tb.mu.Unlock()
	v, ok := tb.visitors[key]
	now := time.Now()
	if !ok {
		tb.visitors[key] = &visitor{tokens: tb.burst - 1, last: now}
		return true
	}
	elapsed := now.Sub(v.last).Seconds()
	v.tokens += elapsed * tb.rate
	if v.tokens > tb.burst {
		v.tokens = tb.burst
	}
	v.last = now
	if v.tokens < 1 {
		return false
	}
	v.tokens--
	return true
}

func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("CF-Connecting-IP"); xff != "" {
		return strings.TrimSpace(xff)
	}
	if xff := r.Header.Get("X-Real-IP"); xff != "" {
		return strings.TrimSpace(xff)
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (tb *TokenBucket) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := ClientIP(r)
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
