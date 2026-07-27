package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/GALIAIS/NodeHarvest/internal/db"
	"github.com/GALIAIS/NodeHarvest/internal/timex"
)

// Manager 多 Token + 环境变量主 Token
type Manager struct {
	DB            *db.Store
	MasterToken   string // NODE_HARVEST_TOKEN / publish.token
	SessionSecret string
	SessionTTL    time.Duration
	CookieName    string
	LocalEnabled  bool
}

type Principal struct {
	Kind           string   `json:"kind"` // master | db | local | session | public
	TokenID        string   `json:"token_id,omitempty"`
	Name           string   `json:"name"`
	AllowCountries []string `json:"allow_countries,omitempty"`
	AllowProtocols []string `json:"allow_protocols,omitempty"`
	MaxRPS         float64  `json:"max_rps,omitempty"`
	DailyQuota     int64    `json:"daily_quota,omitempty"`
	Role           Role     `json:"role,omitempty"`
	TenantID       string   `json:"tenant_id,omitempty"`
	Subject        string   `json:"subject,omitempty"`
	Email          string   `json:"email,omitempty"`
	Authenticated  bool     `json:"authenticated"`
}

func HashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func NewTokenPlain() (plain, id, prefix string, err error) {
	b := make([]byte, 24)
	if _, err = rand.Read(b); err != nil {
		return "", "", "", err
	}
	plain = hex.EncodeToString(b)
	idb := make([]byte, 8)
	_, _ = rand.Read(idb)
	id = hex.EncodeToString(idb)
	prefix = plain
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	return plain, id, prefix, nil
}

// ValidateSubToken 校验订阅 token；master 或 DB token
func (m *Manager) ValidateSubToken(plain string) (*Principal, error) {
	plain = strings.TrimSpace(plain)
	if plain == "" {
		if m.MasterToken == "" {
			// 公开
			return &Principal{Kind: "public", Name: "public"}, nil
		}
		return nil, fmt.Errorf("token required")
	}
	if secureEqual(plain, m.MasterToken) {
		return &Principal{Kind: "master", Name: "master", TokenID: "master"}, nil
	}
	if m.DB == nil {
		return nil, fmt.Errorf("invalid token")
	}
	prefix := plain
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	candidates, err := m.DB.FindTokensByPrefix(prefix)
	if err != nil {
		return nil, fmt.Errorf("invalid token")
	}
	var t *db.Token
	for _, candidate := range candidates {
		valid := false
		if strings.HasPrefix(candidate.TokenHash, "$2") {
			valid = bcrypt.CompareHashAndPassword([]byte(candidate.TokenHash), []byte(plain)) == nil
		} else {
			valid = secureEqual(candidate.TokenHash, HashToken(plain))
		}
		if valid {
			t = candidate
			break
		}
	}
	if t == nil {
		return nil, fmt.Errorf("invalid token")
	}
	if !t.Enabled {
		return nil, fmt.Errorf("token disabled")
	}
	if t.ExpiresAt != "" {
		exp, err := time.Parse(time.RFC3339, t.ExpiresAt)
		if err == nil && time.Now().After(exp) {
			return nil, fmt.Errorf("token expired")
		}
	}
	_ = m.DB.TouchToken(t.ID)
	return &Principal{
		Kind:           "db",
		TokenID:        t.ID,
		Name:           t.Name,
		AllowCountries: t.AllowCountries,
		AllowProtocols: t.AllowProtocols,
		MaxRPS:         t.MaxRPS,
		DailyQuota:     t.DailyQuota,
		TenantID:       tenantOrDefault(t.TenantID),
	}, nil
}

func (m *Manager) CreateToken(name, note, tenant string, countries, protocols []string, days int, maxRPS float64, dailyQuota int64, actor string) (*db.Token, error) {
	if m.DB == nil {
		return nil, fmt.Errorf("db not available")
	}
	plain, id, prefix, err := NewTokenPlain()
	if err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	countries = normalizeCountries(countries)
	protocols, err = normalizeProtocols(protocols)
	if err != nil {
		return nil, err
	}
	t := &db.Token{
		ID:             id,
		Name:           name,
		TokenHash:      string(hash),
		TokenPrefix:    prefix,
		Enabled:        true,
		MaxRPS:         maxRPS,
		AllowCountries: countries,
		AllowProtocols: protocols,
		TenantID:       tenantOrDefault(tenant),
		DailyQuota:     dailyQuota,
		CreatedAt:      timex.NowRFC3339(),
		Note:           note,
		PlainToken:     plain,
	}
	if days > 0 {
		t.ExpiresAt = timex.FormatRFC3339(time.Now().Add(time.Duration(days) * 24 * time.Hour))
	}
	if err := m.DB.InsertToken(t); err != nil {
		return nil, err
	}
	_ = m.DB.Audit(actor, "token.create", name+" "+prefix)
	return t, nil
}

func normalizeCountries(countries []string) []string {
	seen := make(map[string]bool, len(countries))
	out := make([]string, 0, len(countries))
	for _, country := range countries {
		country = strings.ToUpper(strings.TrimSpace(country))
		if country == "UK" {
			country = "GB"
		}
		if len(country) == 2 && !seen[country] {
			seen[country] = true
			out = append(out, country)
		}
	}
	return out
}

func normalizeProtocols(protocols []string) ([]string, error) {
	allowed := map[string]bool{
		"vmess": true, "vless": true, "trojan": true, "ss": true,
		"ssr": true, "hysteria2": true, "tuic": true,
	}
	seen := make(map[string]bool, len(protocols))
	out := make([]string, 0, len(protocols))
	for _, protocol := range protocols {
		protocol = strings.ToLower(strings.TrimSpace(protocol))
		if protocol == "hy2" {
			protocol = "hysteria2"
		}
		if !allowed[protocol] {
			return nil, fmt.Errorf("unsupported protocol %q", protocol)
		}
		if !seen[protocol] {
			seen[protocol] = true
			out = append(out, protocol)
		}
	}
	return out, nil
}

func tenantOrDefault(tenant string) string {
	if strings.TrimSpace(tenant) == "" {
		return "default"
	}
	return strings.TrimSpace(tenant)
}

func secureEqual(a, b string) bool {
	if a == "" || b == "" || len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
