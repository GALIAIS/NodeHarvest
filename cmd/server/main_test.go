package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSPAHandlerDoesNotReserveSubStorePage(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("console"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub-store.html"), []byte("workshop"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := spaHandler(root)
	for path, want := range map[string]int{
		"/sub-store": http.StatusOK,
		"/sub/raw":   http.StatusNotFound,
		"/api/stats": http.StatusNotFound,
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s status=%d want=%d", path, res.Code, want)
		}
		if path == "/sub-store" && res.Body.String() != "workshop" {
			t.Fatalf("sub-store body=%q", res.Body)
		}
	}
}
