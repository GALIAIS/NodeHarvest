package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/local/node-hunter/internal/model"
	"github.com/local/node-hunter/internal/timex"
)

// Store SQLite 持久化：jobs 历史、tokens、audit、可选节点快照元数据
type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if path == "" {
		path = filepath.Join("data", "node-hunter.db")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // sqlite 简单安全
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	slog.Info("sqlite opened", "path", path)
	return s, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS jobs (
  id TEXT PRIMARY KEY,
  type TEXT NOT NULL,
  status TEXT NOT NULL,
  progress REAL,
  message TEXT,
  error TEXT,
  stats_json TEXT,
  options_json TEXT,
  created_at TEXT,
  updated_at TEXT,
  started_at TEXT,
  ended_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_jobs_created ON jobs(created_at);

CREATE TABLE IF NOT EXISTS tokens (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  token_prefix TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  max_rps REAL DEFAULT 0,
  allow_countries TEXT,
  expires_at TEXT,
  created_at TEXT NOT NULL,
  last_used_at TEXT,
  note TEXT
);

CREATE TABLE IF NOT EXISTS audit_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  at TEXT NOT NULL,
  actor TEXT,
  action TEXT NOT NULL,
  detail TEXT
);

CREATE TABLE IF NOT EXISTS job_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  job_id TEXT NOT NULL,
  at TEXT NOT NULL,
  level TEXT,
  message TEXT
);
CREATE INDEX IF NOT EXISTS idx_job_events_job ON job_events(job_id);
`
	_, err := s.db.Exec(schema)
	return err
}

func (s *Store) SaveJob(j *model.Job) error {
	if s == nil || j == nil {
		return nil
	}
	stats, _ := json.Marshal(j.Stats)
	opts, _ := json.Marshal(j.Options)
	_, err := s.db.Exec(`
INSERT INTO jobs(id,type,status,progress,message,error,stats_json,options_json,created_at,updated_at,started_at,ended_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
 status=excluded.status, progress=excluded.progress, message=excluded.message, error=excluded.error,
 stats_json=excluded.stats_json, updated_at=excluded.updated_at, started_at=excluded.started_at, ended_at=excluded.ended_at
`, j.ID, j.Type, string(j.Status), j.Progress, j.Message, j.Error, string(stats), string(opts),
		fmtTime(j.CreatedAt), fmtTime(j.UpdatedAt), fmtTimePtr(j.StartedAt), fmtTimePtr(j.EndedAt))
	return err
}

func (s *Store) ListJobs(limit int) ([]*model.Job, error) {
	if limit <= 0 {
		limit = 30
	}
	rows, err := s.db.Query(`SELECT id,type,status,progress,message,error,stats_json,options_json,created_at,updated_at,started_at,ended_at
 FROM jobs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Job
	for rows.Next() {
		var j model.Job
		var status, statsJ, optsJ, ca, ua string
		var sa, ea sql.NullString
		if err := rows.Scan(&j.ID, &j.Type, &status, &j.Progress, &j.Message, &j.Error, &statsJ, &optsJ, &ca, &ua, &sa, &ea); err != nil {
			return nil, err
		}
		j.Status = model.JobStatus(status)
		_ = json.Unmarshal([]byte(statsJ), &j.Stats)
		_ = json.Unmarshal([]byte(optsJ), &j.Options)
		j.CreatedAt = parseTime(ca)
		j.UpdatedAt = parseTime(ua)
		if sa.Valid {
			t := parseTime(sa.String)
			j.StartedAt = &t
		}
		if ea.Valid {
			t := parseTime(ea.String)
			j.EndedAt = &t
		}
		out = append(out, &j)
	}
	return out, nil
}

func (s *Store) AddJobEvent(jobID, level, msg string) error {
	_, err := s.db.Exec(`INSERT INTO job_events(job_id,at,level,message) VALUES(?,?,?,?)`,
		jobID, timex.NowRFC3339(), level, msg)
	return err
}

func (s *Store) Audit(actor, action, detail string) error {
	_, err := s.db.Exec(`INSERT INTO audit_logs(at,actor,action,detail) VALUES(?,?,?,?)`,
		timex.NowRFC3339(), actor, action, detail)
	return err
}

func (s *Store) ListAudit(limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id,at,actor,action,detail FROM audit_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id int
		var at, actor, action, detail string
		if err := rows.Scan(&id, &at, &actor, &action, &detail); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "at": at, "actor": actor, "action": action, "detail": detail})
	}
	return out, nil
}

// Token 记录
type Token struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	TokenHash      string   `json:"-"`
	TokenPrefix    string   `json:"token_prefix"`
	Enabled        bool     `json:"enabled"`
	MaxRPS         float64  `json:"max_rps"`
	AllowCountries []string `json:"allow_countries,omitempty"`
	ExpiresAt      string   `json:"expires_at,omitempty"`
	CreatedAt      string   `json:"created_at"`
	LastUsedAt     string   `json:"last_used_at,omitempty"`
	Note           string   `json:"note,omitempty"`
	// only on create
	PlainToken string `json:"token,omitempty"`
}

func (s *Store) InsertToken(t *Token) error {
	countries, _ := json.Marshal(t.AllowCountries)
	en := 0
	if t.Enabled {
		en = 1
	}
	_, err := s.db.Exec(`INSERT INTO tokens(id,name,token_hash,token_prefix,enabled,max_rps,allow_countries,expires_at,created_at,note)
 VALUES(?,?,?,?,?,?,?,?,?,?)`, t.ID, t.Name, t.TokenHash, t.TokenPrefix, en, t.MaxRPS, string(countries), t.ExpiresAt, t.CreatedAt, t.Note)
	return err
}

func (s *Store) ListTokens() ([]*Token, error) {
	rows, err := s.db.Query(`SELECT id,name,token_hash,token_prefix,enabled,max_rps,allow_countries,expires_at,created_at,last_used_at,note FROM tokens ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Token
	for rows.Next() {
		var t Token
		var en int
		var countries string
		var last, exp sql.NullString
		if err := rows.Scan(&t.ID, &t.Name, &t.TokenHash, &t.TokenPrefix, &en, &t.MaxRPS, &countries, &exp, &t.CreatedAt, &last, &t.Note); err != nil {
			return nil, err
		}
		t.Enabled = en == 1
		_ = json.Unmarshal([]byte(countries), &t.AllowCountries)
		if exp.Valid {
			t.ExpiresAt = exp.String
		}
		if last.Valid {
			t.LastUsedAt = last.String
		}
		out = append(out, &t)
	}
	return out, nil
}

func (s *Store) FindTokenByHash(hash string) (*Token, error) {
	row := s.db.QueryRow(`SELECT id,name,token_hash,token_prefix,enabled,max_rps,allow_countries,expires_at,created_at,last_used_at,note FROM tokens WHERE token_hash=?`, hash)
	var t Token
	var en int
	var countries string
	var last, exp sql.NullString
	if err := row.Scan(&t.ID, &t.Name, &t.TokenHash, &t.TokenPrefix, &en, &t.MaxRPS, &countries, &exp, &t.CreatedAt, &last, &t.Note); err != nil {
		return nil, err
	}
	t.Enabled = en == 1
	_ = json.Unmarshal([]byte(countries), &t.AllowCountries)
	if exp.Valid {
		t.ExpiresAt = exp.String
	}
	if last.Valid {
		t.LastUsedAt = last.String
	}
	return &t, nil
}

func (s *Store) DeleteToken(id string) error {
	_, err := s.db.Exec(`DELETE FROM tokens WHERE id=?`, id)
	return err
}

func (s *Store) SetTokenEnabled(id string, enabled bool) error {
	en := 0
	if enabled {
		en = 1
	}
	_, err := s.db.Exec(`UPDATE tokens SET enabled=? WHERE id=?`, en, id)
	return err
}

func (s *Store) TouchToken(id string) error {
	_, err := s.db.Exec(`UPDATE tokens SET last_used_at=? WHERE id=?`, timex.NowRFC3339(), id)
	return err
}

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return timex.FormatRFC3339(t)
}
func fmtTimePtr(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return timex.FormatRFC3339(*t)
}
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// Ping 检查 DB
func (s *Store) Ping() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("db nil")
	}
	return s.db.Ping()
}
