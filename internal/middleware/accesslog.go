package middleware

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

// AccessLog 访问日志（脱敏 token 查询参数）
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api") && !strings.HasPrefix(r.URL.Path, "/sub") && r.URL.Path != "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: 200}
		next.ServeHTTP(sw, r)
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
		slog.Info("http",
			"method", r.Method,
			"path", path,
			"query", q.Encode(),
			"status", sw.code,
			"ip", ClientIP(r),
			"dur_ms", time.Since(start).Milliseconds(),
		)
	})
}
