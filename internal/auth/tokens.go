package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/local/node-hunter/internal/db"
	"github.com/local/node-hunter/internal/timex"
)

// Manager 多 Token + 环境变量主 Token
type Manager struct {
	DB          *db.Store
	MasterToken string // NODE_HUNTER_TOKEN / publish.token
	AdminToken  string // NODE_HUNTER_ADMIN_TOKEN，管理 API
	// QueryTokenAllowed 是否允许 ?token=
	QueryTokenAllowed bool
}

type Principal struct {
	Kind           string // master | db | admin | public
	TokenID        string
	Name           string
	AllowCountries []string
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
	if m.MasterToken != "" && plain == m.MasterToken {
		return &Principal{Kind: "master", Name: "master", TokenID: "master"}, nil
	}
	if m.DB == nil {
		return nil, fmt.Errorf("invalid token")
	}
	t, err := m.DB.FindTokenByHash(HashToken(plain))
	if err != nil || t == nil {
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
	}, nil
}

func (m *Manager) ValidateAdmin(plain string) bool {
	if m.AdminToken == "" {
		// 回退 master token
		return m.MasterToken != "" && plain == m.MasterToken
	}
	return plain == m.AdminToken
}

func (m *Manager) CreateToken(name, note string, countries []string, days int) (*db.Token, error) {
	if m.DB == nil {
		return nil, fmt.Errorf("db not available")
	}
	plain, id, prefix, err := NewTokenPlain()
	if err != nil {
		return nil, err
	}
	t := &db.Token{
		ID:             id,
		Name:           name,
		TokenHash:      HashToken(plain),
		TokenPrefix:    prefix,
		Enabled:        true,
		AllowCountries: countries,
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
	_ = m.DB.Audit("admin", "token.create", name+" "+prefix)
	return t, nil
}
