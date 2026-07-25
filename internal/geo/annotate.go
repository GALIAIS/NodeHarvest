package geo

import (
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/local/node-hunter/internal/model"
)

// AnnotateOptions 批量标注选项
type AnnotateOptions struct {
	Concurrency int
	// OnlyAlive 仅标注存活节点
	OnlyAlive bool
	// OnlyEmpty 仅填空（不覆盖已有 Country）
	OnlyEmpty bool
	// HQOnly 仅高质量
	HQOnly   bool
	MinScore float64
	OnProgress func(done, total int)
}

// AnnotateNodes 批量写入 Country/City，返回标注成功数
func (l *Locator) AnnotateNodes(nodes []*model.Node, opt AnnotateOptions) int {
	if len(nodes) == 0 {
		return 0
	}
	if opt.Concurrency <= 0 {
		opt.Concurrency = 32
	}
	// 过滤
	list := make([]*model.Node, 0, len(nodes))
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if opt.OnlyAlive && !n.Alive {
			continue
		}
		if opt.HQOnly && n.Score < opt.MinScore {
			continue
		}
		if opt.OnlyEmpty && n.Country != "" {
			continue
		}
		list = append(list, n)
	}
	total := len(list)
	if total == 0 {
		return 0
	}

	sem := make(chan struct{}, opt.Concurrency)
	var wg sync.WaitGroup
	var done atomic.Int64
	var okCount atomic.Int64

	for _, n := range list {
		n := n
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			r := l.LookupHost(n.Server, n.Name)
			if r.CountryCode != "" {
				n.Country = r.CountryCode
				if r.City != "" {
					n.City = r.City
				}
				if r.ISP != "" {
					n.ISP = r.ISP
				}
				// 标签：国家码 + 旗帜（去重）
				tag := r.CountryCode
				flag := FlagEmoji(r.CountryCode)
				n.Tags = mergeTags(n.Tags, tag)
				if flag != "" {
					n.Tags = mergeTags(n.Tags, flag)
				}
				// 名称前缀：若名称不含国家信息则加旗帜
				if flag != "" && !containsFlagOrCode(n.Name, r.CountryCode) {
					// 不强制改名，避免破坏用户备注；仅靠 Country 字段
				}
				okCount.Add(1)
			}
			cur := int(done.Add(1))
			if opt.OnProgress != nil && (cur%20 == 0 || cur == total) {
				opt.OnProgress(cur, total)
			}
		}()
	}
	wg.Wait()
	nOK := int(okCount.Load())
	slog.Info("geo annotate done", "total", total, "ok", nOK, "mmdb", l.Ready())
	return nOK
}

func mergeTags(tags []string, add string) []string {
	if add == "" {
		return tags
	}
	for _, t := range tags {
		if t == add {
			return tags
		}
	}
	return append(tags, add)
}

func containsFlagOrCode(name, code string) bool {
	if code == "" {
		return false
	}
	if flag := FlagEmoji(code); flag != "" && containsFold(name, flag) {
		return true
	}
	return containsFold(name, code)
}

func containsFold(s, sub string) bool {
	return len(sub) > 0 && (len(s) >= len(sub)) &&
		(fmt.Sprintf("%s", s) != "" && // keep simple
			(indexFold(s, sub) >= 0))
}

func indexFold(s, sub string) int {
	// case-insensitive for ascii codes
	sl, subl := []rune(s), []rune(sub)
	if len(subl) == 0 {
		return 0
	}
	for i := 0; i+len(subl) <= len(sl); i++ {
		ok := true
		for j := 0; j < len(subl); j++ {
			a, b := sl[i+j], subl[j]
			if a >= 'a' && a <= 'z' {
				a -= 32
			}
			if b >= 'a' && b <= 'z' {
				b -= 32
			}
			if a != b {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

// CountByCountry 统计 map[ISO]count
func CountByCountry(nodes []*model.Node, aliveOnly, hqOnly bool, minScore float64) map[string]int {
	m := make(map[string]int)
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if aliveOnly && !n.Alive {
			continue
		}
		if hqOnly && n.Score < minScore {
			continue
		}
		c := n.Country
		if c == "" {
			c = "XX"
		}
		m[c]++
	}
	return m
}
