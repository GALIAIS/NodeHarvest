package auth

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GALIAIS/NodeHarvest/internal/config"
	"github.com/GALIAIS/NodeHarvest/internal/db"
)

func TestLocalSessionRBACAndTamperDetection(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	hash, err := HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default().Auth
	cfg.LocalEnabled = true
	cfg.BootstrapUser = "admin"
	cfg.BootstrapHash = hash
	cfg.SessionSecret = strings.Repeat("s", 32)
	manager, err := NewManager(cfg, database, "")
	if err != nil {
		t.Fatal(err)
	}
	principal, session, err := manager.LoginLocal("default", "admin", "correct-horse-battery-staple")
	if err != nil {
		t.Fatal(err)
	}
	if !principal.Role.Allows(RoleOperator) || principal.TenantID != "default" {
		t.Fatalf("principal=%+v", principal)
	}
	verified, err := manager.ValidateSession(session)
	if err != nil || verified.Subject == "" || !verified.Authenticated {
		t.Fatalf("verified=%+v err=%v", verified, err)
	}
	replacement := "A"
	if strings.HasSuffix(session, replacement) {
		replacement = "B"
	}
	tampered := session[:len(session)-1] + replacement
	if _, err := manager.ValidateSession(tampered); err == nil {
		t.Fatal("tampered session accepted")
	}
	req := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: manager.CookieName, Value: session})
	fromRequest, err := manager.RequestPrincipal(req)
	if err != nil || fromRequest.Role != RoleAdmin {
		t.Fatalf("request principal=%+v err=%v", fromRequest, err)
	}
}

func TestRejectsNonCanonicalSessionSignature(t *testing.T) {
	manager := &Manager{SessionSecret: strings.Repeat("s", 32), SessionTTL: time.Hour}
	session, err := manager.IssueSession(&Principal{
		Subject: "user", Name: "user", Role: RoleAdmin, TenantID: "default", Authenticated: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(session, ".")
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	last := parts[2][len(parts[2])-1]
	index := strings.IndexByte(alphabet, last)
	if index < 0 || index%4 != 0 {
		t.Fatalf("unexpected canonical signature suffix %q", last)
	}
	parts[2] = parts[2][:len(parts[2])-1] + string(alphabet[index+1])
	if _, err := manager.ValidateSession(strings.Join(parts, ".")); err == nil {
		t.Fatal("non-canonical signature encoding accepted")
	}
}

func TestSubscriptionTokenBcryptAndQuota(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "tokens.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	manager := &Manager{DB: database}
	token, err := manager.CreateToken("limited", "", "tenant-a", []string{"us"}, []string{"hy2"}, 0, 5, 1, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(token.TokenHash, "$2") {
		t.Fatalf("token hash is not bcrypt: %q", token.TokenHash)
	}
	principal, err := manager.ValidateSubToken(token.PlainToken)
	if err != nil || principal.TenantID != "tenant-a" ||
		len(principal.AllowProtocols) != 1 || principal.AllowProtocols[0] != "hysteria2" {
		t.Fatalf("principal=%+v err=%v", principal, err)
	}
	if _, allowed, err := database.ConsumeTokenQuota(token.ID, 1); err != nil || !allowed {
		t.Fatalf("first quota allowed=%v err=%v", allowed, err)
	}
	if _, allowed, err := database.ConsumeTokenQuota(token.ID, 1); err != nil || allowed {
		t.Fatalf("second quota allowed=%v err=%v", allowed, err)
	}
}
