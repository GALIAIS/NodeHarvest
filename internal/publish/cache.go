package publish

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/local/node-hunter/internal/exporter"
	"github.com/local/node-hunter/internal/model"
	"github.com/local/node-hunter/internal/timex"
)

// Blob 预渲染订阅产物
type Blob struct {
	Raw           string
	Base64        string
	Clash         string
	Count         int
	ByCountry     map[string]int
	ETag          string
	UpdatedAt     string
	CountryRaw    map[string]string
	CountryBase64 map[string]string
	CountryClash  map[string]string
	CountryCount  map[string]int
}

// Cache 原子指针缓存 + 磁盘落盘
type Cache struct {
	dir string
	ptr unsafe.Pointer // *Blob
	mu  sync.Mutex
	gen atomic.Uint64
}

func NewCache(dir string) *Cache {
	if dir == "" {
		dir = "output"
	}
	_ = os.MkdirAll(dir, 0o755)
	c := &Cache{dir: dir}
	if b := c.loadFromDisk(); b != nil {
		atomic.StorePointer(&c.ptr, unsafe.Pointer(b))
	}
	return c
}

func (c *Cache) Get() *Blob {
	p := atomic.LoadPointer(&c.ptr)
	if p == nil {
		return nil
	}
	return (*Blob)(p)
}

// Update 从高质量节点重建全局与分国家缓存
func (c *Cache) Update(nodes []*model.Node, maxCountryVariants int) *Blob {
	if maxCountryVariants <= 0 {
		maxCountryVariants = 30
	}
	byCountry := map[string]int{}
	groups := map[string][]*model.Node{}
	for _, n := range nodes {
		cc := n.Country
		if cc == "" {
			cc = "XX"
		}
		if cc == "UK" {
			cc = "GB"
		}
		byCountry[cc]++
		groups[cc] = append(groups[cc], n)
	}

	raw := exporter.RenderRaw(nodes)
	b64 := exporter.RenderBase64(nodes)
	clash := exporter.RenderClash(nodes)

	blob := &Blob{
		Raw:           raw,
		Base64:        b64,
		Clash:         clash,
		Count:         len(nodes),
		ByCountry:     byCountry,
		ETag:          etagOf(raw + b64),
		UpdatedAt:     timex.NowRFC3339(),
		CountryRaw:    map[string]string{},
		CountryBase64: map[string]string{},
		CountryClash:  map[string]string{},
		CountryCount:  map[string]int{},
	}

	type kv struct {
		k string
		v int
	}
	var sorted []kv
	for k, v := range byCountry {
		if k == "XX" {
			continue
		}
		sorted = append(sorted, kv{k, v})
	}
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].v > sorted[i].v {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	if len(sorted) > maxCountryVariants {
		sorted = sorted[:maxCountryVariants]
	}
	for _, it := range sorted {
		ns := groups[it.k]
		blob.CountryRaw[it.k] = exporter.RenderRaw(ns)
		blob.CountryBase64[it.k] = exporter.RenderBase64(ns)
		blob.CountryClash[it.k] = exporter.RenderClash(ns)
		blob.CountryCount[it.k] = len(ns)
	}

	atomic.StorePointer(&c.ptr, unsafe.Pointer(blob))
	c.gen.Add(1)
	if err := c.persist(blob); err != nil {
		slog.Warn("publish cache persist", "err", err)
	} else {
		slog.Info("publish cache updated", "nodes", blob.Count, "etag", blob.ETag, "countries", len(blob.CountryCount))
	}
	return blob
}

func (c *Cache) persist(b *Blob) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = os.MkdirAll(c.dir, 0o755)
	writes := map[string]string{
		"sub.txt":    b.Raw,
		"sub.base64": b.Base64,
		"clash.yaml": b.Clash,
		"sub.meta.json": fmt.Sprintf(
			`{"count":%d,"etag":%q,"updated_at":%q}`,
			b.Count, b.ETag, b.UpdatedAt,
		),
	}
	for name, body := range writes {
		path := filepath.Join(c.dir, name)
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, []byte(body), 0o644); err != nil {
			return err
		}
		if err := os.Rename(tmp, path); err != nil {
			return err
		}
	}
	return nil
}

func (c *Cache) loadFromDisk() *Blob {
	raw, err1 := os.ReadFile(filepath.Join(c.dir, "sub.txt"))
	b64, err2 := os.ReadFile(filepath.Join(c.dir, "sub.base64"))
	clash, err3 := os.ReadFile(filepath.Join(c.dir, "clash.yaml"))
	if err1 != nil && err2 != nil && err3 != nil {
		return nil
	}
	b := &Blob{
		Raw:       string(raw),
		Base64:    string(b64),
		Clash:     string(clash),
		UpdatedAt: timex.NowRFC3339(),
		ByCountry: map[string]int{},
	}
	if b.Raw != "" {
		n := 0
		for _, line := range strings.Split(b.Raw, "\n") {
			if strings.TrimSpace(line) != "" {
				n++
			}
		}
		b.Count = n
	}
	b.ETag = etagOf(b.Raw + b.Base64)
	return b
}

func etagOf(s string) string {
	h := sha1.Sum([]byte(s))
	return `"` + hex.EncodeToString(h[:8]) + `"`
}

// Format returns body for format + optional country
func (b *Blob) Format(format, country string) (body string, count int, ok bool) {
	if b == nil {
		return "", 0, false
	}
	country = normalizeCC(country)
	if country != "" {
		switch format {
		case "raw", "txt":
			body, ok = b.CountryRaw[country]
		case "base64", "b64", "sub":
			body, ok = b.CountryBase64[country]
		case "clash", "yaml":
			body, ok = b.CountryClash[country]
		}
		if ok {
			return body, b.CountryCount[country], true
		}
		return "", 0, false
	}
	switch format {
	case "raw", "txt":
		return b.Raw, b.Count, true
	case "base64", "b64", "sub":
		return b.Base64, b.Count, true
	case "clash", "yaml":
		return b.Clash, b.Count, true
	default:
		return "", 0, false
	}
}

func normalizeCC(c string) string {
	c = strings.ToUpper(strings.TrimSpace(c))
	if c == "UK" {
		return "GB"
	}
	return c
}

// Age 缓存年龄
func (b *Blob) Age() time.Duration {
	if b == nil || b.UpdatedAt == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339, b.UpdatedAt)
	if err != nil {
		return 0
	}
	return time.Since(t)
}
