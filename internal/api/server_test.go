package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GALIAIS/NodeHarvest/internal/auth"
	"github.com/GALIAIS/NodeHarvest/internal/config"
	"github.com/GALIAIS/NodeHarvest/internal/db"
	"github.com/GALIAIS/NodeHarvest/internal/model"
	"github.com/GALIAIS/NodeHarvest/internal/service"
	"github.com/GALIAIS/NodeHarvest/internal/store"
)

func TestMutationAuthAndNodeCursor(t *testing.T) {
	cfg := config.Default()
	cfg.Geo.Enabled = false
	cfg.Publish.PreRender = false
	cfg.Export.Dir = t.TempDir()
	cfg.Security.AdminToken = "admin-secret"
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceNodes([]*model.Node{
		{Protocol: model.ProtoVLESS, Server: "a.example", Port: 443, UUID: "a", RawURI: "vless://a@a.example:443"},
		{Protocol: model.ProtoVLESS, Server: "b.example", Port: 443, UUID: "b"},
		{Protocol: model.ProtoVLESS, Server: "c.example", Port: 443, UUID: "c"},
	}); err != nil {
		t.Fatal(err)
	}
	svc := service.New(cfg, st)
	h := New(svc, nil, &auth.Manager{AdminToken: cfg.Security.AdminToken}).Handler()

	req := httptest.NewRequest(http.MethodPost, "/api/jobs/fetch", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated mutation status=%d", res.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/jobs/cancel", nil)
	req.Header.Set("X-Admin-Token", "admin-secret")
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("authenticated mutation status=%d", res.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/jobs/fetch", strings.NewReader(`{} {}`))
	req.Header.Set("X-Admin-Token", "admin-secret")
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("multiple JSON values status=%d", res.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/nodes?limit=2", nil)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	var page struct {
		Total      int           `json:"total"`
		Nodes      []*model.Node `json:"nodes"`
		NextCursor string        `json:"next_cursor"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || len(page.Nodes) != 2 || page.NextCursor == "" {
		t.Fatalf("bad first page: %+v", page)
	}
	for _, node := range page.Nodes {
		if node.RawURI != "" || node.UUID != "" || node.Fingerprint != "" {
			t.Fatalf("public node leaked credentials: %+v", node)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/api/nodes?limit=2&cursor="+page.NextCursor, nil)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if err := json.Unmarshal(res.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Nodes) != 1 {
		t.Fatalf("bad second page size=%d", len(page.Nodes))
	}

	req = httptest.NewRequest(http.MethodGet, "/api/pools/global-hq/export/raw", nil)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated pool export status=%d", res.Code)
	}
}

func TestRedactSourceURL(t *testing.T) {
	got := redactSourceURL("https://user:pass@example.test/sub?token=secret#fragment")
	if got != "https://example.test/sub" {
		t.Fatalf("redacted URL=%q", got)
	}
}

func TestAuditBoundsAndSourceSortValidation(t *testing.T) {
	from, err := auditBound("2026-07-25", false)
	if err != nil || !strings.HasPrefix(from, "2026-07-25T00:00:00") {
		t.Fatalf("from=%q err=%v", from, err)
	}
	to, err := auditBound("2026-07-25", true)
	if err != nil || !strings.HasPrefix(to, "2026-07-25T23:59:59") {
		t.Fatalf("to=%q err=%v", to, err)
	}
	if _, err := auditBound("not-a-date", false); err == nil {
		t.Fatal("invalid audit bound was accepted")
	}

	cfg := config.Default()
	cfg.Geo.Enabled = false
	cfg.Publish.PreRender = false
	cfg.Export.Dir = t.TempDir()
	handler := New(service.New(cfg, store.NewMemory()), nil, &auth.Manager{}).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/sources?sort=unknown", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("source sort status=%d body=%s", res.Code, res.Body)
	}
}

func TestCachedSubscriptionHonorsTokenCountryACL(t *testing.T) {
	cfg := config.Default()
	cfg.Geo.Enabled = false
	cfg.Publish.PreRender = false
	cfg.Export.Dir = t.TempDir()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := st.ReplaceNodes([]*model.Node{
		{
			Protocol: model.ProtoVLESS, Server: "us.example", Port: 443, UUID: "us",
			RawURI: "vless://us@us.example:443", Alive: true, Score: 90, Country: "US",
			LastSeenAt: now,
		},
		{
			Protocol: model.ProtoVLESS, Server: "jp.example", Port: 443, UUID: "jp",
			RawURI: "vless://jp@jp.example:443", Alive: true, Score: 90, Country: "JP",
			LastSeenAt: now,
		},
		{
			Protocol: model.ProtoVMess, Server: "us-vmess.example", Port: 443, UUID: "us-vmess",
			RawURI: "vmess://dGVzdA==", Alive: true, Score: 90, Country: "US",
			LastSeenAt: now,
		},
	}); err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	manager := &auth.Manager{DB: database}
	token, err := manager.CreateToken("us-vless-only", "", "default", []string{"US"}, []string{"vless"}, 0, 0, 1, "test")
	if err != nil {
		t.Fatal(err)
	}
	svc := service.NewWithOptions(cfg, st, service.Options{DB: database})
	svc.RefreshPublishCache()
	h := New(svc, nil, manager).Handler()

	req := httptest.NewRequest(http.MethodGet, "/sub/raw", nil)
	req.Header.Set("X-Sub-Token", token.PlainToken)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("subscription status=%d body=%s", res.Code, res.Body)
	}
	body := res.Body.String()
	if !strings.Contains(body, "us.example") || strings.Contains(body, "jp.example") ||
		strings.Contains(body, "us-vmess.example") {
		t.Fatalf("subscription ACL bypassed: %q", body)
	}
	req = httptest.NewRequest(http.MethodGet, "/sub/raw", nil)
	req.Header.Set("X-Sub-Token", token.PlainToken)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusTooManyRequests {
		t.Fatalf("daily quota status=%d body=%s", res.Code, res.Body)
	}
}

func TestManagementRBACAndHostBoundary(t *testing.T) {
	cfg := config.Default()
	cfg.Geo.Enabled = false
	cfg.Publish.PreRender = false
	cfg.Export.Dir = t.TempDir()
	cfg.Auth.AdminHost = "admin.example"
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := &auth.Manager{
		SessionSecret: strings.Repeat("k", 32), SessionTTL: time.Hour, CookieName: "nh_session",
	}
	viewerToken, err := manager.IssueSession(&auth.Principal{
		Kind: "local", Name: "viewer", Subject: "viewer", Role: auth.RoleViewer,
		TenantID: "default", Authenticated: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	operatorToken, err := manager.IssueSession(&auth.Principal{
		Kind: "local", Name: "operator", Subject: "operator", Role: auth.RoleOperator,
		TenantID: "default", Authenticated: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	h := New(service.New(cfg, st), nil, manager).Handler()

	request := func(host, session string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "http://"+host+"/api/jobs/quality", strings.NewReader(`{}`))
		req.Host = host
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: manager.CookieName, Value: session})
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		return res
	}
	if res := request("admin.example", viewerToken); res.Code != http.StatusForbidden {
		t.Fatalf("viewer mutation status=%d body=%s", res.Code, res.Body)
	}
	if res := request("public.example", operatorToken); res.Code != http.StatusNotFound {
		t.Fatalf("public-host management status=%d body=%s", res.Code, res.Body)
	}
	if res := request("admin.example", operatorToken); res.Code != http.StatusOK {
		t.Fatalf("operator mutation status=%d body=%s", res.Code, res.Body)
	}
}

func TestRuntimeConfigIsConfirmedPersistedAndReloaded(t *testing.T) {
	cfg := config.Default()
	cfg.Geo.Enabled = false
	cfg.Publish.PreRender = false
	cfg.Export.Dir = t.TempDir()
	cfg.Security.AdminToken = "admin-secret"
	database, err := db.Open(filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	newStore := func() *store.Store {
		st, err := store.New(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return st
	}
	svc := service.NewWithOptions(cfg, newStore(), service.Options{DB: database})
	handler := New(svc, nil, &auth.Manager{AdminToken: cfg.Security.AdminToken}).Handler()

	update := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPatch, "/api/admin/config", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Token", cfg.Security.AdminToken)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
	if res := update(`{"publish_min_score":82}`); res.Code != http.StatusBadRequest {
		t.Fatalf("unconfirmed update status=%d body=%s", res.Code, res.Body)
	}
	if res := update(`{"confirm":true,"publish_min_score":82,"publish_max_nodes":321}`); res.Code != http.StatusOK {
		t.Fatalf("config update status=%d body=%s", res.Code, res.Body)
	}
	if svc.Config().Publish.MinScore != 82 || svc.Config().Publish.MaxNodes != 321 {
		t.Fatalf("runtime config not applied: %+v", svc.Config().Publish)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/config/versions", nil)
	req.Header.Set("X-Admin-Token", cfg.Security.AdminToken)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"checksum"`) {
		t.Fatalf("config versions status=%d body=%s", res.Code, res.Body)
	}

	reloaded := service.NewWithOptions(cfg, newStore(), service.Options{DB: database})
	if reloaded.Config().Publish.MinScore != 82 || reloaded.Config().Publish.MaxNodes != 321 {
		t.Fatalf("persisted config not reloaded: %+v", reloaded.Config().Publish)
	}
}

func TestLoginHasDedicatedBruteForceLimit(t *testing.T) {
	cfg := config.Default()
	cfg.Geo.Enabled = false
	cfg.Publish.PreRender = false
	cfg.Export.Dir = t.TempDir()
	cfg.Security.LoginRPS = 0.0001
	cfg.Security.LoginBurst = 2
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handler := New(service.New(cfg, st), nil, &auth.Manager{}).Handler()
	for attempt, want := range []int{http.StatusUnauthorized, http.StatusUnauthorized, http.StatusTooManyRequests} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
			strings.NewReader(`{"tenant":"default","username":"admin","password":"wrong-password"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "192.0.2.10:1234"
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("attempt %d status=%d want=%d body=%s", attempt+1, res.Code, want, res.Body)
		}
	}
}
