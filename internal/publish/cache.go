package publish

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GALIAIS/NodeHarvest/internal/exporter"
	"github.com/GALIAIS/NodeHarvest/internal/model"
	"github.com/GALIAIS/NodeHarvest/internal/timex"
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
	Policy        string
	CountryRaw    map[string]string
	CountryBase64 map[string]string
	CountryClash  map[string]string
	CountryCount  map[string]int
}

// Cache 原子指针缓存 + 磁盘落盘
type Cache struct {
	dir string
	ptr atomic.Pointer[Blob]
	mu  sync.Mutex
}

func NewCache(dir string) *Cache {
	if dir == "" {
		dir = "output"
	}
	_ = os.MkdirAll(dir, 0o700)
	// #nosec G302 -- dir is a directory and 0700 is the restrictive directory mode.
	_ = os.Chmod(dir, 0o700)
	c := &Cache{dir: dir}
	if b := c.loadFromDisk(); b != nil {
		c.ptr.Store(b)
	}
	return c
}

func (c *Cache) Get() *Blob {
	return c.ptr.Load()
}

func (c *Cache) Clear() {
	if c != nil {
		c.ptr.Store(nil)
	}
}

func (c *Cache) Store(blob *Blob, persist bool) error {
	if c == nil || blob == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if blob.ByCountry == nil {
		blob.ByCountry = map[string]int{}
	}
	if blob.CountryRaw == nil {
		blob.CountryRaw = map[string]string{}
	}
	if blob.CountryBase64 == nil {
		blob.CountryBase64 = map[string]string{}
	}
	if blob.CountryClash == nil {
		blob.CountryClash = map[string]string{}
	}
	if blob.CountryCount == nil {
		blob.CountryCount = map[string]int{}
	}
	c.ptr.Store(blob)
	if persist {
		return c.persist(blob)
	}
	return nil
}

// Update 从高质量节点重建全局与分国家缓存
func (c *Cache) Update(nodes []*model.Node, maxCountryVariants int, policy string) *Blob {
	c.mu.Lock()
	defer c.mu.Unlock()
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
		ETag:          etagOf(raw + b64 + clash),
		UpdatedAt:     timex.NowRFC3339(),
		Policy:        policy,
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

	c.ptr.Store(blob)
	if err := c.persist(blob); err != nil {
		slog.Warn("publish cache persist", "err", err)
	} else {
		slog.Info("publish cache updated", "nodes", blob.Count, "etag", blob.ETag, "countries", len(blob.CountryCount))
	}
	return blob
}

func (c *Cache) persist(b *Blob) error {
	_ = os.MkdirAll(c.dir, 0o700)
	// #nosec G302 -- c.dir is a directory and 0700 is the restrictive directory mode.
	_ = os.Chmod(c.dir, 0o700)
	writes := []struct {
		name string
		body string
	}{
		{name: "sub.txt", body: b.Raw},
		{name: "sub.base64", body: b.Base64},
		{name: "clash.yaml", body: b.Clash},
		{name: "sub.meta.json", body: fmt.Sprintf(
			`{"count":%d,"etag":%q,"updated_at":%q,"policy":%q}`,
			b.Count, b.ETag, b.UpdatedAt, b.Policy,
		)},
	}
	for _, write := range writes {
		path := filepath.Join(c.dir, write.name)
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, []byte(write.body), 0o600); err != nil {
			return err
		}
		if err := replaceFile(tmp, path); err != nil {
			return err
		}
	}
	return nil
}

func replaceFile(tmp, path string) error {
	if err := os.Rename(tmp, path); err == nil {
		return nil
	} else if runtime.GOOS != "windows" {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(tmp, path)
}

func (c *Cache) loadFromDisk() *Blob {
	raw, err1 := os.ReadFile(filepath.Join(c.dir, "sub.txt"))
	b64, err2 := os.ReadFile(filepath.Join(c.dir, "sub.base64"))
	clash, err3 := os.ReadFile(filepath.Join(c.dir, "clash.yaml"))
	if err1 != nil && err2 != nil && err3 != nil {
		return nil
	}
	b := &Blob{
		Raw: string(raw), Base64: string(b64), Clash: string(clash), ByCountry: map[string]int{},
	}
	var meta struct {
		Count     int    `json:"count"`
		ETag      string `json:"etag"`
		UpdatedAt string `json:"updated_at"`
		Policy    string `json:"policy"`
	}
	if data, err := os.ReadFile(filepath.Join(c.dir, "sub.meta.json")); err == nil {
		if json.Unmarshal(data, &meta) == nil {
			b.Count = meta.Count
			b.ETag = meta.ETag
			b.UpdatedAt = meta.UpdatedAt
			b.Policy = meta.Policy
		}
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
	b.ETag = etagOf(b.Raw + b.Base64 + b.Clash)
	if b.UpdatedAt == "" {
		if info, err := os.Stat(filepath.Join(c.dir, "sub.txt")); err == nil {
			b.UpdatedAt = timex.FormatRFC3339(info.ModTime())
		}
	}
	return b
}

func etagOf(s string) string {
	h := sha256.Sum256([]byte(s))
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

func (b *Blob) Fresh(maxAge time.Duration) bool {
	if b == nil || b.UpdatedAt == "" || maxAge <= 0 {
		return false
	}
	updated, err := time.Parse(time.RFC3339, b.UpdatedAt)
	age := time.Since(updated)
	return err == nil && age >= -5*time.Minute && age <= maxAge
}

func (b *Blob) MatchesPolicy(policy string) bool {
	return b != nil && policy != "" && b.Policy == policy
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
