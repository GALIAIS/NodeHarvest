BEGIN;

CREATE TABLE IF NOT EXISTS jobs (
  id TEXT PRIMARY KEY,
  type TEXT NOT NULL,
  status TEXT NOT NULL,
  progress DOUBLE PRECISION,
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
CREATE INDEX IF NOT EXISTS idx_jobs_tenant_created ON jobs(tenant_id,created_at);

CREATE TABLE IF NOT EXISTS tokens (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  token_prefix TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  tenant_id TEXT NOT NULL DEFAULT 'default',
  max_rps DOUBLE PRECISION DEFAULT 0,
  allow_countries TEXT,
  allow_protocols TEXT,
  daily_quota BIGINT NOT NULL DEFAULT 0,
  expires_at TEXT,
  created_at TEXT NOT NULL,
  last_used_at TEXT,
  note TEXT
);
CREATE INDEX IF NOT EXISTS idx_tokens_prefix ON tokens(token_prefix);
CREATE INDEX IF NOT EXISTS idx_tokens_tenant ON tokens(tenant_id);

CREATE TABLE IF NOT EXISTS audit_logs (
  id BIGSERIAL PRIMARY KEY,
  at TEXT NOT NULL,
  actor TEXT,
  action TEXT NOT NULL,
  detail TEXT
);

CREATE TABLE IF NOT EXISTS job_events (
  id BIGSERIAL PRIMARY KEY,
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
  fetch_count BIGINT NOT NULL DEFAULT 0,
  success_count BIGINT NOT NULL DEFAULT 0,
  latency_ms BIGINT NOT NULL DEFAULT 0,
  bytes INTEGER NOT NULL DEFAULT 0,
  status_code INTEGER NOT NULL DEFAULT 0,
  attempts INTEGER NOT NULL DEFAULT 0,
  disabled_until TEXT,
  manually_disabled INTEGER NOT NULL DEFAULT 0,
  contribution_total INTEGER NOT NULL DEFAULT 0,
  contribution_hq INTEGER NOT NULL DEFAULT 0,
  health_score DOUBLE PRECISION NOT NULL DEFAULT 100
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
  score DOUBLE PRECISION NOT NULL DEFAULT 0,
  alive INTEGER NOT NULL DEFAULT 0,
  first_seen_at TEXT,
  last_seen_at TEXT,
  payload_json TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_nodes_last_seen ON nodes(last_seen_at);
CREATE INDEX IF NOT EXISTS idx_nodes_country_score ON nodes(country,score);

CREATE TABLE IF NOT EXISTS node_metrics (
  id BIGSERIAL PRIMARY KEY,
  node_id TEXT NOT NULL,
  measured_at TEXT NOT NULL,
  success INTEGER NOT NULL,
  latency_ms BIGINT NOT NULL DEFAULT 0,
  jitter_ms BIGINT NOT NULL DEFAULT 0,
  tls_ms BIGINT NOT NULL DEFAULT 0,
  http_latency_ms BIGINT NOT NULL DEFAULT 0,
  throughput_bps BIGINT NOT NULL DEFAULT 0,
  score DOUBLE PRECISION NOT NULL DEFAULT 0
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
  oidc_issuer TEXT,
  oidc_subject TEXT,
  created_at TEXT NOT NULL,
  last_login_at TEXT,
  UNIQUE(tenant_id,username)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_oidc ON users(oidc_issuer,oidc_subject);

CREATE TABLE IF NOT EXISTS token_usage (
  token_id TEXT NOT NULL,
  day TEXT NOT NULL,
  requests BIGINT NOT NULL DEFAULT 0,
  bytes BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY(token_id,day)
);

CREATE TABLE IF NOT EXISTS config_versions (
  id TEXT PRIMARY KEY,
  actor TEXT NOT NULL,
  checksum TEXT NOT NULL,
  config_yaml TEXT NOT NULL,
  created_at TEXT NOT NULL
);

COMMIT;
