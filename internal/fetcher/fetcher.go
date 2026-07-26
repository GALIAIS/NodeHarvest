package fetcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GALIAIS/NodeHarvest/internal/config"
)

// Document 拉取到的原始文档
type Document struct {
	Source     config.Source
	Body       string
	Err        error
	Latency    time.Duration
	Bytes      int
	StatusCode int
	Attempts   int
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
	if len(sources) == 0 {
		return nil
	}
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
	send:
		for _, s := range sources {
			select {
			case <-ctx.Done():
				break send
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
	sort.SliceStable(docs, func(i, j int) bool {
		if docs[i].Source.Priority != docs[j].Source.Priority {
			return docs[i].Source.Priority > docs[j].Source.Priority
		}
		return docs[i].Source.Name < docs[j].Source.Name
	})
	return docs
}

// FetchOne 拉取单个源
func (f *Fetcher) FetchOne(ctx context.Context, src config.Source) Document {
	start := time.Now()
	doc := Document{Source: src}
	for attempt := 1; attempt <= 3; attempt++ {
		doc = f.fetchAttempt(ctx, src)
		doc.Attempts = attempt
		doc.Latency = time.Since(start)
		if doc.Err == nil || !retryable(doc.StatusCode, doc.Err) || attempt == 3 {
			return doc
		}
		delay := time.Duration(attempt*250) * time.Millisecond
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			doc.Err = ctx.Err()
			return doc
		case <-timer.C:
		}
	}
	return doc
}

func (f *Fetcher) fetchAttempt(ctx context.Context, src config.Source) Document {
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
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			doc.Err = urlErr.Err
		} else {
			doc.Err = err
		}
		return doc
	}
	defer resp.Body.Close()
	doc.StatusCode = resp.StatusCode

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		doc.Err = fmt.Errorf("http %d", resp.StatusCode)
		return doc
	}

	maxBytes := src.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 32 << 20
	}
	if resp.ContentLength > maxBytes {
		doc.Err = fmt.Errorf("response too large: %d > %d bytes", resp.ContentLength, maxBytes)
		return doc
	}
	limited := io.LimitReader(resp.Body, maxBytes+1)
	b, err := io.ReadAll(limited)
	if err != nil {
		doc.Err = err
		return doc
	}
	if int64(len(b)) > maxBytes {
		doc.Err = fmt.Errorf("response exceeds %d bytes", maxBytes)
		return doc
	}
	doc.Body = string(b)
	doc.Bytes = len(b)

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
	return strings.Contains(sample, "<html") ||
		strings.Contains(sample, "<!doctype html") ||
		strings.Contains(sample, "<title>404") ||
		strings.Contains(sample, "repository not found")
}

func retryable(status int, err error) bool {
	if err == nil {
		return false
	}
	if status == 0 {
		return true
	}
	switch status {
	case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests,
		http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}
