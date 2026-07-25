package fetcher

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/local/node-hunter/internal/config"
)

// Document 拉取到的原始文档
type Document struct {
	Source  config.Source
	Body    string
	Err     error
	Latency time.Duration
	Bytes   int
}

// Fetcher HTTP 抓取器
type Fetcher struct {
	client    *http.Client
	userAgent string
}

func New(timeout time.Duration, userAgent string) *Fetcher {
	return &Fetcher{
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 8 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
		userAgent: userAgent,
	}
}

// FetchAll 并发拉取所有源
func (f *Fetcher) FetchAll(ctx context.Context, sources []config.Source, workers int) []Document {
	if workers <= 0 {
		workers = 8
	}
	in := make(chan config.Source)
	out := make(chan Document, len(sources))

	var wg sync.WaitGroup
	for i := 0; i < workers && i < len(sources); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for src := range in {
				out <- f.FetchOne(ctx, src)
			}
		}()
	}

	go func() {
		for _, s := range sources {
			select {
			case <-ctx.Done():
				return
			case in <- s:
			}
		}
		close(in)
		wg.Wait()
		close(out)
	}()

	docs := make([]Document, 0, len(sources))
	for d := range out {
		docs = append(docs, d)
	}
	return docs
}

// FetchOne 拉取单个源
func (f *Fetcher) FetchOne(ctx context.Context, src config.Source) Document {
	start := time.Now()
	doc := Document{Source: src}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		doc.Err = err
		return doc
	}
	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := f.client.Do(req)
	if err != nil {
		doc.Err = err
		doc.Latency = time.Since(start)
		return doc
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		doc.Err = fmt.Errorf("http %d", resp.StatusCode)
		doc.Latency = time.Since(start)
		return doc
	}

	// 限制 32MB（部分聚合源如 SubCrawler 可达 20MB+）
	limited := io.LimitReader(resp.Body, 32<<20)
	b, err := io.ReadAll(limited)
	if err != nil {
		doc.Err = err
		doc.Latency = time.Since(start)
		return doc
	}
	doc.Body = string(b)
	doc.Bytes = len(b)
	doc.Latency = time.Since(start)

	// 部分 CDN 返回 HTML 错误页
	if looksLikeHTMLError(doc.Body) {
		doc.Err = fmt.Errorf("html error page")
	}
	return doc
}

func looksLikeHTMLError(body string) bool {
	sample := strings.ToLower(body)
	if len(sample) > 512 {
		sample = sample[:512]
	}
	if strings.Contains(sample, "<html") && (strings.Contains(sample, "404") || strings.Contains(sample, "not found")) {
		return true
	}
	return false
}
