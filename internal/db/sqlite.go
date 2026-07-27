package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"github.com/GALIAIS/NodeHarvest/internal/model"
	"github.com/GALIAIS/NodeHarvest/internal/timex"
)

// Store provides the durable relational backend for SQLite and PostgreSQL.
type Store struct {
	db     *sql.DB
	driver string
}

func Open(path string) (*Store, error) {
	return OpenDatabase(DatabaseOptions{Driver: "sqlite", DSN: path})
}

type DatabaseOptions struct {
	Driver       string
	DSN          string
	MaxOpenConns int
	MaxIdleConns int
	JobDays      int
	AuditDays    int
	MetricDays   int
}

func OpenDatabase(opt DatabaseOptions) (*Store, error) {
	driver := strings.ToLower(strings.TrimSpace(opt.Driver))
	if driver == "" {
		driver = "sqlite"
	}
	dsn := strings.TrimSpace(opt.DSN)
	sqlDriver := driver
	switch driver {
	case "sqlite":
		path := dsn
		if path == "" {
			path = filepath.Join("data", "nodeharvest.db")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
		// #nosec G302 -- the SQLite parent is a directory and 0700 is the restrictive directory mode.
		if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
		dsn = path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	case "postgres":
		if dsn == "" {
			return nil, fmt.Errorf("postgres DSN is required")
		}
		sqlDriver = "pgx"
	default:
		return nil, fmt.Errorf("unsupported database driver %q", driver)
	}
	db, err := sql.Open(sqlDriver, dsn)
	if err != nil {
		return nil, err
	}
	if driver == "sqlite" {
		db.SetMaxOpenConns(1)
	} else {
		if opt.MaxOpenConns <= 0 {
			opt.MaxOpenConns = 10
		}
		if opt.MaxIdleConns < 0 {
			opt.MaxIdleConns = 0
		}
		db.SetMaxOpenConns(opt.MaxOpenConns)
		db.SetMaxIdleConns(opt.MaxIdleConns)
		db.SetConnMaxLifetime(30 * time.Minute)
	}
	s := &Store{db: db, driver: driver}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.Check(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("%s integrity: %w", driver, err)
	}
	if err := s.PruneHistory(opt.JobDays, opt.AuditDays, opt.MetricDays); err != nil {
		slog.Warn("database retention", "driver", driver, "err", err)
	}
	slog.Info("database opened", "driver", driver)
	return s, nil
}

func (s *Store) rebind(query string) string {
	if s == nil || s.driver != "postgres" {
		return query
	}
	var b strings.Builder
	index := 1
	for _, r := range query {
		if r == '?' {
			fmt.Fprintf(&b, "$%d", index)
			index++
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (s *Store) exec(query string, args ...any) (sql.Result, error) {
	return s.db.Exec(s.rebind(query), args...)
}

func (s *Store) query(query string, args ...any) (*sql.Rows, error) {
	return s.db.Query(s.rebind(query), args...)
}

func (s *Store) queryRow(query string, args ...any) *sql.Row {
	return s.db.QueryRow(s.rebind(query), args...)
}

func (s *Store) Driver() string {
	if s == nil {
		return ""
	}
	return s.driver
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	identity := `INTEGER PRIMARY KEY AUTOINCREMENT`
	if s.driver == "postgres" {
		identity = `BIGSERIAL PRIMARY KEY`
	}
	schema := fmt.Sprintf(`
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
  ended_at TEXT,
  actor TEXT,
  tenant_id TEXT NOT NULL DEFAULT 'default'
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
  ,tenant_id TEXT NOT NULL DEFAULT 'default'
  ,allow_protocols TEXT
  ,daily_quota INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_tokens_prefix ON tokens(token_prefix);

CREATE TABLE IF NOT EXISTS audit_logs (
  id %s,
  at TEXT NOT NULL,
  actor TEXT,
  action TEXT NOT NULL,
  detail TEXT
);

CREATE TABLE IF NOT EXISTS job_events (
  id %s,
  job_id TEXT NOT NULL,
  at TEXT NOT NULL,
  level TEXT,
  message TEXT
);
CREATE INDEX IF NOT EXISTS idx_job_events_job ON job_events(job_id);

CREATE TABLE IF NOT EXISTS source_states (
  name TEXT PRIMARY KEY,
  url TEXT NOT NULL,
  priority INTEGER NOT NULL DEFAULT 50,
  last_attempt_at TEXT,
  last_success_at TEXT,
  last_error TEXT,
  consecutive_failures INTEGER NOT NULL DEFAULT 0,
  fetch_count INTEGER NOT NULL DEFAULT 0,
  success_count INTEGER NOT NULL DEFAULT 0,
  latency_ms INTEGER NOT NULL DEFAULT 0,
  bytes INTEGER NOT NULL DEFAULT 0,
  status_code INTEGER NOT NULL DEFAULT 0,
  attempts INTEGER NOT NULL DEFAULT 0,
  disabled_until TEXT,
  manually_disabled INTEGER NOT NULL DEFAULT 0,
  contribution_total INTEGER NOT NULL DEFAULT 0,
  contribution_hq INTEGER NOT NULL DEFAULT 0,
  health_score REAL NOT NULL DEFAULT 100
);
CREATE TABLE IF NOT EXISTS nodes (
  id TEXT PRIMARY KEY,
  fingerprint TEXT NOT NULL,
  protocol TEXT NOT NULL,
  server TEXT NOT NULL,
  port INTEGER NOT NULL,
  country TEXT,
  asn TEXT,
  entry_type TEXT,
  score REAL NOT NULL DEFAULT 0,
  alive INTEGER NOT NULL DEFAULT 0,
  first_seen_at TEXT,
  last_seen_at TEXT,
  payload_json TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_nodes_last_seen ON nodes(last_seen_at);
CREATE INDEX IF NOT EXISTS idx_nodes_country_score ON nodes(country,score);

CREATE TABLE IF NOT EXISTS node_metrics (
  id %s,
  node_id TEXT NOT NULL,
  measured_at TEXT NOT NULL,
  success INTEGER NOT NULL,
  latency_ms INTEGER NOT NULL DEFAULT 0,
  jitter_ms INTEGER NOT NULL DEFAULT 0,
  tls_ms INTEGER NOT NULL DEFAULT 0,
  http_latency_ms INTEGER NOT NULL DEFAULT 0,
  throughput_bps INTEGER NOT NULL DEFAULT 0,
  score REAL NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_node_metrics_node_at ON node_metrics(node_id,measured_at);

CREATE TABLE IF NOT EXISTS task_queue (
  id TEXT PRIMARY KEY,
  type TEXT NOT NULL,
  options_json TEXT NOT NULL,
  priority INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  max_attempts INTEGER NOT NULL DEFAULT 3,
  available_at TEXT NOT NULL,
  lease_until TEXT,
  worker_id TEXT,
  last_error TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  started_at TEXT,
  ended_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_task_queue_claim ON task_queue(status,available_at,priority,created_at);

CREATE TABLE IF NOT EXISTS alerts (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  severity TEXT NOT NULL,
  message TEXT NOT NULL,
  details_json TEXT,
  active INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  resolved_at TEXT,
  acknowledged_at TEXT,
  acknowledged_by TEXT
);
CREATE INDEX IF NOT EXISTS idx_alerts_active ON alerts(active,created_at);

CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  username TEXT NOT NULL,
  email TEXT,
  password_hash TEXT,
  role TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  last_login_at TEXT,
  UNIQUE(tenant_id,username)
);

CREATE TABLE IF NOT EXISTS token_usage (
  token_id TEXT NOT NULL,
  day TEXT NOT NULL,
  requests INTEGER NOT NULL DEFAULT 0,
  bytes INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY(token_id,day)
);

CREATE TABLE IF NOT EXISTS config_versions (
  id TEXT PRIMARY KEY,
  actor TEXT NOT NULL,
  checksum TEXT NOT NULL,
  config_yaml TEXT NOT NULL,
  created_at TEXT NOT NULL
);
`, identity, identity, identity)
	if _, err := s.exec(schema); err != nil {
		return err
	}
	for _, migration := range []string{
		`ALTER TABLE source_states ADD COLUMN disabled_until TEXT`,
		`ALTER TABLE source_states ADD COLUMN manually_disabled INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE source_states ADD COLUMN contribution_total INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE source_states ADD COLUMN contribution_hq INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE source_states ADD COLUMN health_score REAL NOT NULL DEFAULT 100`,
		`ALTER TABLE jobs ADD COLUMN actor TEXT`,
		`ALTER TABLE jobs ADD COLUMN tenant_id TEXT NOT NULL DEFAULT 'default'`,
		`ALTER TABLE tokens ADD COLUMN tenant_id TEXT NOT NULL DEFAULT 'default'`,
		`ALTER TABLE tokens ADD COLUMN allow_protocols TEXT`,
		`ALTER TABLE tokens ADD COLUMN daily_quota INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE alerts ADD COLUMN acknowledged_at TEXT`,
		`ALTER TABLE alerts ADD COLUMN acknowledged_by TEXT`,
	} {
		if _, err := s.exec(migration); err != nil {
			message := strings.ToLower(err.Error())
			if !strings.Contains(message, "duplicate column") && !strings.Contains(message, "already exists") {
				return err
			}
		}
	}
	return nil
}

func (s *Store) SaveJob(j *model.Job) error {
	if s == nil || j == nil {
		return nil
	}
	stats, _ := json.Marshal(j.Stats)
	opts, _ := json.Marshal(j.Options)
	_, err := s.exec(`
INSERT INTO jobs(id,type,status,progress,message,error,stats_json,options_json,created_at,updated_at,started_at,ended_at,actor,tenant_id)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
 status=excluded.status, progress=excluded.progress, message=excluded.message, error=excluded.error,
 stats_json=excluded.stats_json, updated_at=excluded.updated_at, started_at=excluded.started_at, ended_at=excluded.ended_at,
 actor=excluded.actor,tenant_id=excluded.tenant_id
`, j.ID, j.Type, string(j.Status), j.Progress, j.Message, j.Error, string(stats), string(opts),
		fmtTime(j.CreatedAt), fmtTime(j.UpdatedAt), fmtTimePtr(j.StartedAt), fmtTimePtr(j.EndedAt), j.Actor, tenantOrDefault(j.TenantID))
	return err
}

func (s *Store) ListJobs(limit int) ([]*model.Job, error) {
	return s.ListJobsPage(limit, "")
}

func (s *Store) ListJobsPage(limit int, cursor string) ([]*model.Job, error) {
	return s.ListJobsPageTenant(limit, cursor, "")
}

func (s *Store) ListJobsPageTenant(limit int, cursor, tenant string) ([]*model.Job, error) {
	if limit <= 0 {
		limit = 30
	}
	query := `SELECT id,type,status,progress,message,error,stats_json,options_json,created_at,updated_at,started_at,ended_at,actor,tenant_id
 FROM jobs`
	args := []any{}
	var conditions []string
	if tenant != "" {
		conditions = append(conditions, `tenant_id=?`)
		args = append(args, tenantOrDefault(tenant))
	}
	if cursor != "" {
		var created string
		cursorQuery := `SELECT created_at FROM jobs WHERE id=?`
		cursorArgs := []any{cursor}
		if tenant != "" {
			cursorQuery += ` AND tenant_id=?`
			cursorArgs = append(cursorArgs, tenantOrDefault(tenant))
		}
		if err := s.queryRow(cursorQuery, cursorArgs...).Scan(&created); err != nil {
			return nil, err
		}
		conditions = append(conditions, `(created_at < ? OR (created_at = ? AND id < ?))`)
		args = append(args, created, created, cursor)
	}
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, ` AND `)
	}
	query += ` ORDER BY created_at DESC,id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanJobs(rows)
}

func (s *Store) GetJob(id string) (*model.Job, error) {
	return s.GetJobTenant(id, "")
}

func (s *Store) GetJobTenant(id, tenant string) (*model.Job, error) {
	query := `SELECT id,type,status,progress,message,error,stats_json,options_json,created_at,updated_at,started_at,ended_at,actor,tenant_id
 FROM jobs WHERE id=?`
	args := []any{id}
	if tenant != "" {
		query += ` AND tenant_id=?`
		args = append(args, tenantOrDefault(tenant))
	}
	rows, err := s.query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs, err := scanJobs(rows)
	if err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return nil, sql.ErrNoRows
	}
	return jobs[0], nil
}

type rowsScanner interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanJobs(rows rowsScanner) ([]*model.Job, error) {
	var out []*model.Job
	for rows.Next() {
		var j model.Job
		var status, statsJ, optsJ, ca, ua string
		var sa, ea, actor, tenant sql.NullString
		if err := rows.Scan(&j.ID, &j.Type, &status, &j.Progress, &j.Message, &j.Error, &statsJ, &optsJ, &ca, &ua, &sa, &ea, &actor, &tenant); err != nil {
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
		j.Actor = actor.String
		j.TenantID = tenant.String
		out = append(out, &j)
	}
	return out, rows.Err()
}

func (s *Store) AddJobEvent(jobID, level, msg string) error {
	_, err := s.exec(`INSERT INTO job_events(job_id,at,level,message) VALUES(?,?,?,?)`,
		jobID, timex.NowRFC3339(), level, msg)
	return err
}

type JobEvent struct {
	ID      int64  `json:"id"`
	JobID   string `json:"job_id"`
	At      string `json:"at"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

func (s *Store) ListJobEvents(jobID string, afterID int64, limit int) ([]JobEvent, error) {
	return s.ListJobEventsTenant(jobID, afterID, limit, "")
}

func (s *Store) ListJobEventsTenant(jobID string, afterID int64, limit int, tenant string) ([]JobEvent, error) {
	if tenant != "" {
		if _, err := s.GetJobTenant(jobID, tenant); err != nil {
			return nil, err
		}
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.query(`SELECT id,job_id,at,level,message FROM job_events
 WHERE job_id=? AND id>? ORDER BY id LIMIT ?`, jobID, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []JobEvent
	for rows.Next() {
		var event JobEvent
		if err := rows.Scan(&event.ID, &event.JobID, &event.At, &event.Level, &event.Message); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

// SourceState 采集源最近状态；用于健康度、陈旧度和降级判断。
type SourceState struct {
	Name                string  `json:"name"`
	URL                 string  `json:"url"`
	Priority            int     `json:"priority"`
	LastAttemptAt       string  `json:"last_attempt_at,omitempty"`
	LastSuccessAt       string  `json:"last_success_at,omitempty"`
	LastError           string  `json:"last_error,omitempty"`
	ConsecutiveFailures int     `json:"consecutive_failures"`
	FetchCount          int64   `json:"fetch_count"`
	SuccessCount        int64   `json:"success_count"`
	LatencyMS           int64   `json:"latency_ms"`
	Bytes               int     `json:"bytes"`
	StatusCode          int     `json:"status_code"`
	Attempts            int     `json:"attempts"`
	DisabledUntil       string  `json:"disabled_until,omitempty"`
	ManuallyDisabled    bool    `json:"manually_disabled"`
	ContributionTotal   int     `json:"contribution_total"`
	ContributionHQ      int     `json:"contribution_hq"`
	HealthScore         float64 `json:"health_score"`
}

func (s *Store) RecordSourceFetch(st SourceState, ok bool) error {
	if s == nil {
		return nil
	}
	now := timex.NowRFC3339()
	manual := 0
	if st.ManuallyDisabled {
		manual = 1
	}
	if ok {
		_, err := s.exec(`
INSERT INTO source_states(name,url,priority,last_attempt_at,last_success_at,last_error,consecutive_failures,fetch_count,success_count,latency_ms,bytes,status_code,attempts,disabled_until,manually_disabled,contribution_total,contribution_hq,health_score)
VALUES(?,?,?,?,?,'',0,1,1,?,?,?,?,?,?,?,?,?)
ON CONFLICT(name) DO UPDATE SET
 url=excluded.url, priority=excluded.priority, last_attempt_at=excluded.last_attempt_at,
 last_success_at=excluded.last_success_at, last_error='', consecutive_failures=0,
 fetch_count=source_states.fetch_count+1, success_count=source_states.success_count+1,
 latency_ms=excluded.latency_ms, bytes=excluded.bytes, status_code=excluded.status_code, attempts=excluded.attempts,
 disabled_until=excluded.disabled_until, manually_disabled=excluded.manually_disabled,
 contribution_total=excluded.contribution_total, contribution_hq=excluded.contribution_hq, health_score=excluded.health_score
`, st.Name, st.URL, st.Priority, now, now, st.LatencyMS, st.Bytes, st.StatusCode, st.Attempts,
			st.DisabledUntil, manual, st.ContributionTotal, st.ContributionHQ, st.HealthScore)
		return err
	}
	_, err := s.exec(`
INSERT INTO source_states(name,url,priority,last_attempt_at,last_error,consecutive_failures,fetch_count,success_count,latency_ms,bytes,status_code,attempts,disabled_until,manually_disabled,contribution_total,contribution_hq,health_score)
VALUES(?,?,?,?,?,1,1,0,?,?,?,?,?,?,?,?,?)
ON CONFLICT(name) DO UPDATE SET
 url=excluded.url, priority=excluded.priority, last_attempt_at=excluded.last_attempt_at,
 last_error=excluded.last_error, consecutive_failures=source_states.consecutive_failures+1,
 fetch_count=source_states.fetch_count+1, latency_ms=excluded.latency_ms,
 bytes=excluded.bytes, status_code=excluded.status_code, attempts=excluded.attempts,
 disabled_until=excluded.disabled_until, manually_disabled=excluded.manually_disabled,
 contribution_total=excluded.contribution_total, contribution_hq=excluded.contribution_hq, health_score=excluded.health_score
`, st.Name, st.URL, st.Priority, now, st.LastError, st.LatencyMS, st.Bytes, st.StatusCode, st.Attempts,
		st.DisabledUntil, manual, st.ContributionTotal, st.ContributionHQ, st.HealthScore)
	return err
}

func (s *Store) ListSourceStates() ([]SourceState, error) {
	if s == nil {
		return nil, nil
	}
	rows, err := s.query(`SELECT name,url,priority,last_attempt_at,last_success_at,last_error,
 consecutive_failures,fetch_count,success_count,latency_ms,bytes,status_code,attempts,
 disabled_until,manually_disabled,contribution_total,contribution_hq,health_score
 FROM source_states ORDER BY priority DESC,name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SourceState
	for rows.Next() {
		var st SourceState
		var attempt, success, lastErr, disabled sql.NullString
		var manual int
		if err := rows.Scan(&st.Name, &st.URL, &st.Priority, &attempt, &success, &lastErr,
			&st.ConsecutiveFailures, &st.FetchCount, &st.SuccessCount, &st.LatencyMS,
			&st.Bytes, &st.StatusCode, &st.Attempts, &disabled, &manual,
			&st.ContributionTotal, &st.ContributionHQ, &st.HealthScore); err != nil {
			return nil, err
		}
		st.LastAttemptAt = attempt.String
		st.LastSuccessAt = success.String
		st.LastError = lastErr.String
		st.DisabledUntil = disabled.String
		st.ManuallyDisabled = manual == 1
		out = append(out, st)
	}
	return out, rows.Err()
}

func (s *Store) SetSourceEnabled(name string, enabled bool) error {
	manual := 1
	if enabled {
		manual = 0
	}
	result, err := s.exec(`UPDATE source_states SET manually_disabled=?,disabled_until=NULL WHERE name=?`, manual, name)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) EnsureSource(st SourceState) error {
	if st.HealthScore == 0 {
		st.HealthScore = 100
	}
	_, err := s.exec(`INSERT INTO source_states(name,url,priority,health_score)
 VALUES(?,?,?,?) ON CONFLICT(name) DO UPDATE SET url=excluded.url,priority=excluded.priority`,
		st.Name, st.URL, st.Priority, st.HealthScore)
	return err
}

func (s *Store) SetSourceContribution(name string, total, hq int, health float64) error {
	_, err := s.exec(`UPDATE source_states SET contribution_total=?,contribution_hq=?,health_score=? WHERE name=?`,
		total, hq, health, name)
	return err
}

func (s *Store) Audit(actor, action, detail string) error {
	_, err := s.exec(`INSERT INTO audit_logs(at,actor,action,detail) VALUES(?,?,?,?)`,
		timex.NowRFC3339(), actor, action, detail)
	return err
}

func (s *Store) ListAudit(limit int) ([]map[string]any, error) {
	return s.ListAuditTenant(limit, "")
}

func (s *Store) ListAuditTenant(limit int, tenant string) ([]map[string]any, error) {
	return s.ListAuditTenantRange(limit, tenant, "", "")
}

func (s *Store) ListAuditTenantRange(limit int, tenant, from, to string) ([]map[string]any, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	query := `SELECT id,at,actor,action,detail FROM audit_logs`
	conditions, args := auditFilters(tenant, from, to)
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, ` AND `)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAuditRows(rows)
}

func (s *Store) ListAuditPageTenantRange(limit int, cursor, tenant, from, to string) ([]map[string]any, error) {
	if limit <= 0 || limit > 101 {
		limit = 51
	}
	conditions, args := auditFilters(tenant, from, to)
	if cursor != "" {
		id, err := strconv.ParseInt(cursor, 10, 64)
		if err != nil || id < 1 {
			return nil, fmt.Errorf("invalid audit cursor")
		}
		conditions = append(conditions, `id<?`)
		args = append(args, id)
	}
	query := `SELECT id,at,actor,action,detail FROM audit_logs`
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, ` AND `)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAuditRows(rows)
}

func (s *Store) CountAuditTenantRange(tenant, from, to string) (int, error) {
	conditions, args := auditFilters(tenant, from, to)
	query := `SELECT COUNT(*) FROM audit_logs`
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, ` AND `)
	}
	var count int
	if err := s.queryRow(query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func auditFilters(tenant, from, to string) ([]string, []any) {
	conditions := []string{}
	args := []any{}
	if tenant != "" {
		conditions = append(conditions, `(actor LIKE ? OR actor='system')`)
		args = append(args, tenantOrDefault(tenant)+":%")
	}
	if from != "" {
		conditions = append(conditions, `at>=?`)
		args = append(args, from)
	}
	if to != "" {
		conditions = append(conditions, `at<=?`)
		args = append(args, to)
	}
	return conditions, args
}

func scanAuditRows(rows *sql.Rows) ([]map[string]any, error) {
	var out []map[string]any
	for rows.Next() {
		var id int
		var at, actor, action, detail string
		if err := rows.Scan(&id, &at, &actor, &action, &detail); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "at": at, "actor": actor, "action": action, "detail": detail})
	}
	return out, rows.Err()
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
	AllowProtocols []string `json:"allow_protocols,omitempty"`
	TenantID       string   `json:"tenant_id"`
	DailyQuota     int64    `json:"daily_quota"`
	ExpiresAt      string   `json:"expires_at,omitempty"`
	CreatedAt      string   `json:"created_at"`
	LastUsedAt     string   `json:"last_used_at,omitempty"`
	Note           string   `json:"note,omitempty"`
	RequestsToday  int64    `json:"requests_today"`
	BytesToday     int64    `json:"bytes_today"`
	// only on create
	PlainToken string `json:"token,omitempty"`
}

func (s *Store) InsertToken(t *Token) error {
	countries, _ := json.Marshal(t.AllowCountries)
	protocols, _ := json.Marshal(t.AllowProtocols)
	en := 0
	if t.Enabled {
		en = 1
	}
	_, err := s.exec(`INSERT INTO tokens(id,name,token_hash,token_prefix,enabled,max_rps,allow_countries,expires_at,created_at,note,tenant_id,allow_protocols,daily_quota)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, t.ID, t.Name, t.TokenHash, t.TokenPrefix, en, t.MaxRPS, string(countries),
		t.ExpiresAt, t.CreatedAt, t.Note, tenantOrDefault(t.TenantID), string(protocols), t.DailyQuota)
	return err
}

func (s *Store) ListTokens() ([]*Token, error) {
	return s.ListTokensTenant("")
}

func (s *Store) ListTokensTenant(tenant string) ([]*Token, error) {
	query := `SELECT t.id,t.name,t.token_hash,t.token_prefix,t.enabled,t.max_rps,t.allow_countries,t.expires_at,
 t.created_at,t.last_used_at,t.note,t.tenant_id,t.allow_protocols,t.daily_quota,
 COALESCE(u.requests,0),COALESCE(u.bytes,0)
 FROM tokens t LEFT JOIN token_usage u ON u.token_id=t.id AND u.day=?`
	args := []any{timex.Now().Format("2006-01-02")}
	if tenant != "" {
		query += ` WHERE t.tenant_id=?`
		args = append(args, tenant)
	}
	query += ` ORDER BY t.created_at DESC`
	rows, err := s.query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Token
	for rows.Next() {
		token, err := scanTokenUsage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, token)
	}
	return out, nil
}

func (s *Store) ListTokensPageTenant(limit int, cursor, tenant string) ([]*Token, error) {
	if limit <= 0 || limit > 101 {
		limit = 26
	}
	query := `SELECT t.id,t.name,t.token_hash,t.token_prefix,t.enabled,t.max_rps,t.allow_countries,t.expires_at,
 t.created_at,t.last_used_at,t.note,t.tenant_id,t.allow_protocols,t.daily_quota,
 COALESCE(u.requests,0),COALESCE(u.bytes,0)
 FROM tokens t LEFT JOIN token_usage u ON u.token_id=t.id AND u.day=?`
	args := []any{timex.Now().Format("2006-01-02")}
	conditions := []string{}
	if tenant != "" {
		conditions = append(conditions, `t.tenant_id=?`)
		args = append(args, tenantOrDefault(tenant))
	}
	if cursor != "" {
		var created string
		cursorQuery := `SELECT created_at FROM tokens WHERE id=?`
		cursorArgs := []any{cursor}
		if tenant != "" {
			cursorQuery += ` AND tenant_id=?`
			cursorArgs = append(cursorArgs, tenantOrDefault(tenant))
		}
		if err := s.queryRow(cursorQuery, cursorArgs...).Scan(&created); err != nil {
			return nil, err
		}
		conditions = append(conditions, `(t.created_at<? OR (t.created_at=? AND t.id<?))`)
		args = append(args, created, created, cursor)
	}
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, ` AND `)
	}
	query += ` ORDER BY t.created_at DESC,t.id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Token
	for rows.Next() {
		token, err := scanTokenUsage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, token)
	}
	return out, rows.Err()
}

func (s *Store) CountTokensTenant(tenant string) (int, error) {
	query := `SELECT COUNT(*) FROM tokens`
	args := []any{}
	if tenant != "" {
		query += ` WHERE tenant_id=?`
		args = append(args, tenantOrDefault(tenant))
	}
	var count int
	if err := s.queryRow(query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func scanTokenUsage(row rowScanner) (*Token, error) {
	token, err := scanTokenFields(row, true)
	if err != nil {
		return nil, err
	}
	return token, nil
}

func (s *Store) FindTokenByHash(hash string) (*Token, error) {
	row := s.queryRow(`SELECT id,name,token_hash,token_prefix,enabled,max_rps,allow_countries,expires_at,created_at,last_used_at,note,tenant_id,allow_protocols,daily_quota FROM tokens WHERE token_hash=?`, hash)
	return scanToken(row)
}

func (s *Store) FindTokensByPrefix(prefix string) ([]*Token, error) {
	rows, err := s.query(`SELECT id,name,token_hash,token_prefix,enabled,max_rps,allow_countries,expires_at,created_at,last_used_at,note,tenant_id,allow_protocols,daily_quota
 FROM tokens WHERE token_prefix=?`, prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tokens []*Token
	for rows.Next() {
		token, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	return tokens, rows.Err()
}

func scanToken(row rowScanner) (*Token, error) {
	return scanTokenFields(row, false)
}

func scanTokenFields(row rowScanner, withUsage bool) (*Token, error) {
	var t Token
	var en int
	var maxRPS sql.NullFloat64
	var countries, protocols, note, last, exp sql.NullString
	fields := []any{&t.ID, &t.Name, &t.TokenHash, &t.TokenPrefix, &en, &maxRPS, &countries,
		&exp, &t.CreatedAt, &last, &note, &t.TenantID, &protocols, &t.DailyQuota}
	if withUsage {
		fields = append(fields, &t.RequestsToday, &t.BytesToday)
	}
	if err := row.Scan(fields...); err != nil {
		return nil, err
	}
	t.Enabled = en == 1
	if maxRPS.Valid {
		t.MaxRPS = maxRPS.Float64
	}
	if countries.Valid {
		_ = json.Unmarshal([]byte(countries.String), &t.AllowCountries)
	}
	if protocols.Valid {
		_ = json.Unmarshal([]byte(protocols.String), &t.AllowProtocols)
	}
	if exp.Valid {
		t.ExpiresAt = exp.String
	}
	if last.Valid {
		t.LastUsedAt = last.String
	}
	if note.Valid {
		t.Note = note.String
	}
	return &t, nil
}

func (s *Store) DeleteToken(id string) error {
	_, err := s.exec(`DELETE FROM tokens WHERE id=?`, id)
	return err
}

func (s *Store) DeleteTokenTenant(id, tenant string) error {
	result, err := s.exec(`DELETE FROM tokens WHERE id=? AND tenant_id=?`, id, tenantOrDefault(tenant))
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) SetTokenEnabled(id string, enabled bool) error {
	en := 0
	if enabled {
		en = 1
	}
	_, err := s.exec(`UPDATE tokens SET enabled=? WHERE id=?`, en, id)
	return err
}

func (s *Store) SetTokenEnabledTenant(id, tenant string, enabled bool) error {
	en := 0
	if enabled {
		en = 1
	}
	result, err := s.exec(`UPDATE tokens SET enabled=? WHERE id=? AND tenant_id=?`, en, id, tenantOrDefault(tenant))
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) TouchToken(id string) error {
	_, err := s.exec(`UPDATE tokens SET last_used_at=? WHERE id=?`, timex.NowRFC3339(), id)
	return err
}

func (s *Store) ConsumeTokenQuota(id string, dailyQuota int64) (int64, bool, error) {
	if id == "" {
		return 0, true, nil
	}
	day := timex.Now().Format("2006-01-02")
	query := `INSERT INTO token_usage(token_id,day,requests,bytes) VALUES(?,?,1,0)
 ON CONFLICT(token_id,day) DO UPDATE SET requests=token_usage.requests+1`
	args := []any{id, day}
	if dailyQuota > 0 {
		query += ` WHERE token_usage.requests<?`
		args = append(args, dailyQuota)
	}
	query += ` RETURNING requests`
	var used int64
	if err := s.queryRow(query, args...).Scan(&used); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, err
	}
	remaining := int64(-1)
	if dailyQuota > 0 {
		remaining = dailyQuota - used
	}
	return remaining, true, nil
}

func (s *Store) AddTokenBytes(id string, bytes int64) error {
	if id == "" || bytes <= 0 {
		return nil
	}
	_, err := s.exec(`UPDATE token_usage SET bytes=bytes+? WHERE token_id=? AND day=?`,
		bytes, id, timex.Now().Format("2006-01-02"))
	return err
}

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(timex.Location()).Format("2006-01-02T15:04:05.000000000Z07:00")
}
func fmtTimePtr(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return fmtTime(*t)
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

func tenantOrDefault(tenant string) string {
	if tenant == "" {
		return "default"
	}
	return tenant
}

// Ping 检查 DB
func (s *Store) Ping() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("db nil")
	}
	return s.db.Ping()
}

func (s *Store) Check() error {
	if err := s.Ping(); err != nil {
		return err
	}
	if s.driver != "sqlite" {
		return nil
	}
	var result string
	if err := s.queryRow(`PRAGMA quick_check`).Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return fmt.Errorf("sqlite quick_check: %s", result)
	}
	return nil
}

func (s *Store) PruneHistory(jobDays, auditDays int, metricDays ...int) error {
	if s == nil {
		return nil
	}
	if jobDays <= 0 {
		jobDays = 30
	}
	if auditDays <= 0 {
		auditDays = 90
	}
	metrics := 30
	if len(metricDays) > 0 && metricDays[0] > 0 {
		metrics = metricDays[0]
	}
	jobCutoff := fmtTime(time.Now().AddDate(0, 0, -jobDays))
	auditCutoff := fmtTime(time.Now().AddDate(0, 0, -auditDays))
	metricCutoff := fmtTime(time.Now().AddDate(0, 0, -metrics))
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`DELETE FROM job_events WHERE job_id IN (SELECT id FROM jobs WHERE created_at < ?)`, []any{jobCutoff}},
		{`DELETE FROM jobs WHERE created_at < ?`, []any{jobCutoff}},
		{`DELETE FROM audit_logs WHERE at < ?`, []any{auditCutoff}},
		{`DELETE FROM node_metrics WHERE measured_at < ?`, []any{metricCutoff}},
		{`DELETE FROM task_queue WHERE status IN ('completed','dead','canceled') AND updated_at < ?`, []any{jobCutoff}},
		{`DELETE FROM alerts WHERE active=0 AND resolved_at < ?`, []any{auditCutoff}},
	} {
		if _, execErr := tx.Exec(s.rebind(statement.query), statement.args...); execErr != nil {
			_ = tx.Rollback()
			return execErr
		}
	}
	return tx.Commit()
}
