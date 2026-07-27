package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/GALIAIS/NodeHarvest/internal/config"
	"github.com/GALIAIS/NodeHarvest/internal/db"
)

type Role string

const (
	RoleViewer   Role = "viewer"
	RoleOperator Role = "operator"
	RoleAdmin    Role = "admin"
)

func (r Role) Allows(required Role) bool {
	level := map[Role]int{RoleViewer: 1, RoleOperator: 2, RoleAdmin: 3}
	return level[r] >= level[required]
}

func normalizeRole(value string, fallback Role) Role {
	switch Role(strings.ToLower(strings.TrimSpace(value))) {
	case RoleViewer:
		return RoleViewer
	case RoleOperator:
		return RoleOperator
	case RoleAdmin:
		return RoleAdmin
	default:
		if fallback == "" {
			return RoleViewer
		}
		return fallback
	}
}

type sessionClaims struct {
	Issuer   string `json:"iss"`
	Subject  string `json:"sub"`
	Name     string `json:"name"`
	Email    string `json:"email,omitempty"`
	Role     Role   `json:"role"`
	TenantID string `json:"tenant"`
	IssuedAt int64  `json:"iat"`
	Expires  int64  `json:"exp"`
}

func NewManager(cfg config.AuthConfig, database *db.Store, masterToken string) (*Manager, error) {
	manager := &Manager{
		DB:            database,
		MasterToken:   masterToken,
		SessionSecret: cfg.SessionSecret,
		SessionTTL:    time.Duration(cfg.SessionTTLHours) * time.Hour,
		CookieName:    cfg.SessionCookieName,
		LocalEnabled:  cfg.LocalEnabled,
	}
	if manager.SessionTTL <= 0 {
		manager.SessionTTL = 12 * time.Hour
	}
	if manager.CookieName == "" {
		manager.CookieName = "nh_session"
	}
	if database != nil && cfg.LocalEnabled {
		if err := database.EnsureBootstrapUser(cfg.BootstrapUser, cfg.BootstrapHash, string(RoleAdmin), cfg.BootstrapTenant); err != nil {
			return nil, fmt.Errorf("bootstrap user: %w", err)
		}
	}
	return manager, nil
}

func (m *Manager) IssueSession(principal *Principal) (string, error) {
	if m == nil || len(m.SessionSecret) < 32 {
		return "", fmt.Errorf("session signing is not configured")
	}
	if principal == nil || !principal.Authenticated {
		return "", fmt.Errorf("authenticated principal is required")
	}
	now := time.Now()
	claims := sessionClaims{
		Issuer: "nodeharvest", Subject: principal.Subject, Name: principal.Name, Email: principal.Email,
		Role: principal.Role, TenantID: tenantOrDefault(principal.TenantID),
		IssuedAt: now.Unix(), Expires: now.Add(m.SessionTTL).Unix(),
	}
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	unsigned := rawBase64(header) + "." + rawBase64(payload)
	signature := sign(unsigned, m.SessionSecret)
	return unsigned + "." + rawBase64(signature), nil
}

func (m *Manager) ValidateSession(raw string) (*Principal, error) {
	if m == nil || len(m.SessionSecret) < 32 {
		return nil, fmt.Errorf("session signing is not configured")
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid session")
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid session")
	}
	var header map[string]string
	if json.Unmarshal(headerBytes, &header) != nil || header["alg"] != "HS256" || header["typ"] != "JWT" {
		return nil, fmt.Errorf("invalid session algorithm")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(signature, sign(parts[0]+"."+parts[1], m.SessionSecret)) {
		return nil, fmt.Errorf("invalid session signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid session")
	}
	var claims sessionClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("invalid session")
	}
	now := time.Now().Unix()
	if claims.Issuer != "nodeharvest" || claims.Subject == "" || claims.Expires <= now ||
		claims.IssuedAt > now+60 || !normalizeRole(string(claims.Role), "").Allows(RoleViewer) {
		return nil, fmt.Errorf("expired or invalid session")
	}
	return &Principal{
		Kind: "session", Name: claims.Name, Role: claims.Role, TenantID: tenantOrDefault(claims.TenantID),
		Subject: claims.Subject, Email: claims.Email, Authenticated: true,
	}, nil
}

func (m *Manager) LoginLocal(tenant, username, password string) (*Principal, string, error) {
	if m == nil || !m.LocalEnabled || m.DB == nil {
		return nil, "", fmt.Errorf("local login is disabled")
	}
	user, err := m.DB.FindUser(tenant, strings.TrimSpace(username))
	if err != nil || user == nil || !user.Enabled ||
		bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return nil, "", fmt.Errorf("invalid credentials")
	}
	principal := principalFromUser(user, "local")
	token, err := m.IssueSession(principal)
	if err != nil {
		return nil, "", err
	}
	_ = m.DB.TouchUserLogin(user.ID)
	_ = m.DB.Audit(principal.Actor(), "auth.login", "local")
	return principal, token, nil
}

func (m *Manager) RequestPrincipal(r *http.Request) (*Principal, error) {
	if r == nil {
		return nil, fmt.Errorf("request is required")
	}
	if cookie, err := r.Cookie(m.CookieName); err == nil && cookie.Value != "" {
		return m.ValidateSession(cookie.Value)
	}
	return &Principal{Kind: "public", Name: "public", Role: RoleViewer, TenantID: "public"}, nil
}

func (p *Principal) Actor() string {
	if p == nil {
		return "unknown"
	}
	if p.Subject != "" {
		return tenantOrDefault(p.TenantID) + ":" + p.Subject
	}
	if p.Name != "" {
		return p.Name
	}
	return p.Kind
}

func principalFromUser(user *db.User, kind string) *Principal {
	return &Principal{
		Kind: kind, Name: user.Username, Role: normalizeRole(user.Role, RoleViewer),
		TenantID: tenantOrDefault(user.TenantID), Subject: user.ID, Email: user.Email, Authenticated: true,
	}
}

func rawBase64(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func sign(message, secret string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(message))
	return mac.Sum(nil)
}

func HashPassword(password string) (string, error) {
	if len(password) < 12 {
		return "", fmt.Errorf("password must contain at least 12 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}
