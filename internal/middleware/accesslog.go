package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"
)

type requestIDKey struct{}

func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

// AccessLog 访问日志（脱敏 token 查询参数）
func AccessLog(next http.Handler, trustedProxies []string, observe func(method, route string, code int)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api") && !strings.HasPrefix(r.URL.Path, "/sub") && r.URL.Path != "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		requestID := trustedRequestID(r.Header.Get("X-Request-ID"))
		if requestID == "" {
			requestID = newRequestID()
		}
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, requestID))
		w.Header().Set("X-Request-ID", requestID)
		sw := &statusWriter{ResponseWriter: w, code: 200}
		next.ServeHTTP(sw, r)
		route := r.Pattern
		if _, p, ok := strings.Cut(route, " "); ok {
			route = p
		}
		if route == "" {
			route = "unmatched"
		}
		if observe != nil {
			observe(r.Method, route, sw.code)
		}
		path := r.URL.Path
		// 不记录完整 query 中的 token
		q := r.URL.Query()
		if q.Get("token") != "" {
			tok := q.Get("token")
			mask := tok
			if len(mask) > 6 {
				mask = mask[:4] + "***"
			} else {
				mask = "***"
			}
			q.Set("token", mask)
		}
		// #nosec G706 -- slog emits structured fields and the configured handler performs escaping.
		slog.Info("http",
			"request_id", requestID,
			"trace_id", trace.SpanContextFromContext(r.Context()).TraceID().String(),
			"method", r.Method,
			"path", path,
			"query", q.Encode(),
			"status", sw.code,
			"ip", ClientIP(r, trustedProxies),
			"dur_ms", time.Since(start).Milliseconds(),
		)
	})
}

func trustedRequestID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 64 {
		return ""
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' {
			continue
		}
		return ""
	}
	return value
}

func newRequestID() string {
	var value [12]byte
	if _, err := rand.Read(value[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("150405.000000")))
	}
	return hex.EncodeToString(value[:])
}
