package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"

	"github.com/GALIAIS/NodeHarvest/internal/timex"
)

type User struct {
	ID           string `json:"id"`
	TenantID     string `json:"tenant_id"`
	Username     string `json:"username"`
	Email        string `json:"email,omitempty"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"`
	Enabled      bool   `json:"enabled"`
	OIDCIssuer   string `json:"oidc_issuer,omitempty"`
	OIDCSubject  string `json:"oidc_subject,omitempty"`
	CreatedAt    string `json:"created_at"`
	LastLoginAt  string `json:"last_login_at,omitempty"`
}

func (s *Store) InsertUser(user *User) error {
	if s == nil || user == nil {
		return fmt.Errorf("user is required")
	}
	if user.ID == "" || user.Username == "" {
		return fmt.Errorf("user id and username are required")
	}
	if user.CreatedAt == "" {
		user.CreatedAt = timex.NowRFC3339()
	}
	user.TenantID = tenantOrDefault(user.TenantID)
	enabled := 0
	if user.Enabled {
		enabled = 1
	}
	_, err := s.exec(`INSERT INTO users(id,tenant_id,username,email,password_hash,role,enabled,oidc_issuer,oidc_subject,created_at,last_login_at)
 VALUES(?,?,?,?,?,?,?,?,?,?,?)`, user.ID, user.TenantID, user.Username, user.Email, user.PasswordHash,
		user.Role, enabled, user.OIDCIssuer, user.OIDCSubject, user.CreatedAt, nullIfEmpty(user.LastLoginAt))
	return err
}

func (s *Store) EnsureBootstrapUser(username, passwordHash, role, tenant string) error {
	if username == "" || passwordHash == "" {
		return nil
	}
	tenant = tenantOrDefault(tenant)
	_, err := s.exec(`INSERT INTO users(id,tenant_id,username,password_hash,role,enabled,created_at)
 VALUES(?,?,?,?,?,1,?)
 ON CONFLICT(tenant_id,username) DO UPDATE SET
 password_hash=CASE WHEN users.password_hash='' THEN excluded.password_hash ELSE users.password_hash END,
 enabled=1`, "bootstrap-"+hashID(tenant+"\x00"+username), tenant, username, passwordHash, role, timex.NowRFC3339())
	return err
}

func (s *Store) UpsertOIDCUser(user *User) error {
	if user == nil || user.OIDCIssuer == "" || user.OIDCSubject == "" {
		return fmt.Errorf("OIDC issuer and subject are required")
	}
	user.TenantID = tenantOrDefault(user.TenantID)
	if user.ID == "" {
		user.ID = "oidc-" + hashID(user.OIDCIssuer+"\x00"+user.OIDCSubject)
	}
	if user.CreatedAt == "" {
		user.CreatedAt = timex.NowRFC3339()
	}
	_, err := s.exec(`INSERT INTO users(id,tenant_id,username,email,password_hash,role,enabled,oidc_issuer,oidc_subject,created_at,last_login_at)
 VALUES(?,?,?,?,?,?,1,?,?,?,?)
 ON CONFLICT(oidc_issuer,oidc_subject) DO UPDATE SET
 tenant_id=excluded.tenant_id,username=excluded.username,email=excluded.email,role=excluded.role,
 enabled=1,last_login_at=excluded.last_login_at`,
		user.ID, user.TenantID, user.Username, user.Email, "", user.Role, user.OIDCIssuer, user.OIDCSubject,
		user.CreatedAt, timex.NowRFC3339())
	return err
}

func (s *Store) FindUser(tenant, username string) (*User, error) {
	return scanUser(s.queryRow(`SELECT id,tenant_id,username,email,password_hash,role,enabled,
 oidc_issuer,oidc_subject,created_at,last_login_at FROM users WHERE tenant_id=? AND username=?`,
		tenantOrDefault(tenant), username))
}

func (s *Store) FindOIDCUser(issuer, subject string) (*User, error) {
	return scanUser(s.queryRow(`SELECT id,tenant_id,username,email,password_hash,role,enabled,
 oidc_issuer,oidc_subject,created_at,last_login_at FROM users WHERE oidc_issuer=? AND oidc_subject=?`,
		issuer, subject))
}

func scanUser(row rowScanner) (*User, error) {
	var user User
	var email, password, issuer, subject, last sql.NullString
	var enabled int
	if err := row.Scan(&user.ID, &user.TenantID, &user.Username, &email, &password, &user.Role,
		&enabled, &issuer, &subject, &user.CreatedAt, &last); err != nil {
		return nil, err
	}
	user.Email = email.String
	user.PasswordHash = password.String
	user.OIDCIssuer = issuer.String
	user.OIDCSubject = subject.String
	user.LastLoginAt = last.String
	user.Enabled = enabled == 1
	return &user, nil
}

func (s *Store) ListUsers(tenant string) ([]*User, error) {
	query := `SELECT id,tenant_id,username,email,password_hash,role,enabled,
 oidc_issuer,oidc_subject,created_at,last_login_at FROM users`
	args := []any{}
	if tenant != "" {
		query += ` WHERE tenant_id=?`
		args = append(args, tenantOrDefault(tenant))
	}
	query += ` ORDER BY tenant_id,username`
	rows, err := s.query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []*User
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) SetUserEnabled(id, tenant string, enabled bool) error {
	value := 0
	if enabled {
		value = 1
	}
	result, err := s.exec(`UPDATE users SET enabled=? WHERE id=? AND tenant_id=?`, value, id, tenantOrDefault(tenant))
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) TouchUserLogin(id string) error {
	_, err := s.exec(`UPDATE users SET last_login_at=? WHERE id=?`, timex.NowRFC3339(), id)
	return err
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func hashID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:12])
}
