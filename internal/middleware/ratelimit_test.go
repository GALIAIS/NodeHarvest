package middleware

import (
	"net/http/httptest"
	"testing"
)

func TestClientIPOnlyTrustsConfiguredProxy(t *testing.T) {
	r := httptest.NewRequest("GET", "http://example.test/", nil)
	r.RemoteAddr = "203.0.113.10:1234"
	r.Header.Set("X-Forwarded-For", "198.51.100.20")
	if got := ClientIP(r, []string{"127.0.0.1/32"}); got != "203.0.113.10" {
		t.Fatalf("trusted spoofed header: %s", got)
	}

	r.RemoteAddr = "127.0.0.1:1234"
	if got := ClientIP(r, []string{"127.0.0.1/32"}); got != "198.51.100.20" {
		t.Fatalf("ignored trusted proxy header: %s", got)
	}

	r.Header.Set("X-Forwarded-For", "192.0.2.99, 198.51.100.20")
	if got := ClientIP(r, []string{"127.0.0.1/32"}); got != "198.51.100.20" {
		t.Fatalf("trusted spoofed xff chain: %s", got)
	}
}
