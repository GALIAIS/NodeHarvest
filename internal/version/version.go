package version

import (
	"runtime"
	"time"
)

// 构建时注入：-ldflags "-X github.com/local/node-hunter/internal/version.Version=..."
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
	startedAt = time.Now()
)

func Info() map[string]any {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	return map[string]any{
		"version":    Version,
		"commit":     Commit,
		"build_time": BuildTime,
		"go":         runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"uptime_sec": int(time.Since(startedAt).Seconds()),
		"started_at": startedAt.In(loc).Format(time.RFC3339),
		"tz":         "Asia/Shanghai",
	}
}

func Uptime() time.Duration { return time.Since(startedAt) }
func StartedAt() time.Time  { return startedAt }
