package fetcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GALIAIS/NodeHarvest/internal/config"
)

func TestFetchOneRetriesAndRejectsOversize(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			http.Error(w, "retry", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("vless://id@example.com:443"))
	}))
	defer srv.Close()

	f := New(3*time.Second, "test")
	doc := f.FetchOne(context.Background(), config.Source{
		Name: "retry", URL: srv.URL, MaxBytes: 1024,
	})
	if doc.Err != nil || doc.Attempts != 3 {
		t.Fatalf("retry result: attempts=%d err=%v", doc.Attempts, doc.Err)
	}

	doc = f.FetchOne(context.Background(), config.Source{
		Name: "small-cap", URL: srv.URL, MaxBytes: 4,
	})
	if doc.Err == nil {
		t.Fatal("expected oversize response error")
	}
}

func TestFetchAllReturnsWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		New(time.Second, "test").FetchAll(ctx, []config.Source{
			{Name: "a", URL: "https://example.test/a"},
			{Name: "b", URL: "https://example.test/b"},
		}, 2)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("FetchAll hung after cancellation")
	}
}
