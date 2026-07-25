package timex

import (
	"sync"
	"time"
)

// 统一业务时区：Asia/Shanghai（东八区）
// API / 订阅 / 调度状态一律输出带 +08:00 的 RFC3339，避免 Z/UTC 歧义。

var (
	once sync.Once
	loc  *time.Location
)

// Location 返回 Asia/Shanghai；加载失败时回退 FixedZone CST+8
func Location() *time.Location {
	once.Do(func() {
		l, err := time.LoadLocation("Asia/Shanghai")
		if err != nil {
			l = time.FixedZone("CST", 8*3600)
		}
		loc = l
	})
	return loc
}

// Now 当前上海时间
func Now() time.Time {
	return time.Now().In(Location())
}

// In 转换到上海时区（保留瞬时点）
func In(t time.Time) time.Time {
	return t.In(Location())
}

// FormatRFC3339 上海时区 RFC3339，例如 2026-07-25T11:03:06+08:00
func FormatRFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(Location()).Format(time.RFC3339)
}

// NowRFC3339 当前上海时间字符串
func NowRFC3339() string {
	return FormatRFC3339(Now())
}

// FormatFileTS 文件名时间戳（上海本地墙钟）
func FormatFileTS(t time.Time) string {
	return t.In(Location()).Format("20060102-150405")
}
