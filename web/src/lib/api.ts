export type Quality = {
  score_version: string;
  rounds: number;
  success_rate: number;
  avg_latency_ms: number;
  min_latency_ms: number;
  max_latency_ms: number;
  jitter_ms: number;
  tls_ok: boolean;
  tls_ms?: number;
  http_ms?: number;
  throughput_bps?: number;
  edge_score: number;
  score: number;
  notes?: string[];
  breakdown?: Record<string, number>;
};

export type AIProbeResult = {
  target: string;
  url: string;
  ok: boolean;
  status_code?: number;
  latency_ms: number;
  error?: string;
  mode: string;
};

export type NodeItem = {
  id: string;
  protocol: string;
  name: string;
  server: string;
  port: number;
  tls: boolean;
  raw_uri?: string;
  source: string;
  sources?: string[];
  alive: boolean;
  latency_ms?: number;
  score: number;
  grade: string;
  country?: string;
  city?: string;
  asn?: string;
  entry_type?: string;
  tags?: string[];
  quality?: Quality;
  ai_access?: Record<string, AIProbeResult>;
  error?: string;
  tested_at?: string;
  first_seen_at?: string;
  last_seen_at?: string;
  quality_failures?: number;
  next_test_at?: string;
};

export type DashboardStats = {
  total_nodes: number;
  alive_nodes: number;
  high_quality: number;
  by_protocol: Record<string, number>;
  by_grade: Record<string, number>;
  by_source: Record<string, number>;
  by_country?: Record<string, number>;
  by_country_hq?: Record<string, number>;
  avg_latency_ms: number;
  ai_pass_rate: Record<string, number>;
  last_fetch_at?: string;
  last_quality_at?: string;
  sources_enabled: number;
};

export type CountryRow = {
  code: string;
  name: string;
  flag: string;
  count: number;
  display: string;
};

export type Job = {
  id: string;
  type: string;
  status: string;
  progress: number;
  message: string;
  error?: string;
  stats?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
  started_at?: string;
  ended_at?: string;
};

export type JobEvent = {
  id: number;
  job_id: string;
  at: string;
  level: string;
  message: string;
};

export type QueuedTask = {
  id: string;
  type: string;
  priority: number;
  status: string;
  attempts: number;
  max_attempts: number;
  available_at: string;
  lease_until?: string;
  worker_id?: string;
  last_error?: string;
  created_at: string;
  updated_at: string;
};

export type Source = {
  name: string;
  type: string;
  url: string;
  enabled: boolean;
  effective_enabled: boolean;
  manually_disabled: boolean;
  disabled_until?: string;
  priority: number;
  max_bytes: number;
  last_attempt_at?: string;
  last_success_at?: string;
  last_error?: string;
  consecutive_failures: number;
  fetch_count: number;
  success_rate: number;
  latency_ms: number;
  bytes: number;
  status_code: number;
  contribution_total: number;
  contribution_hq: number;
  health_score: number;
};

export type AITarget = {
  key: string;
  name: string;
  url: string;
  host: string;
};

export type Principal = {
  kind: string;
  token_id?: string;
  name: string;
  role: "viewer" | "operator" | "admin";
  tenant_id: string;
  subject?: string;
  email?: string;
  authenticated: boolean;
};

export type SessionInfo = {
  authenticated: boolean;
  principal: Principal;
  local_enabled: boolean;
};

export type DashboardSource = Pick<
  Source,
  "name" | "effective_enabled" | "contribution_total" | "contribution_hq" | "health_score" | "latency_ms"
>;

export type DashboardNode = Pick<
  NodeItem,
  "id" | "protocol" | "name" | "source" | "country" | "latency_ms" | "score" | "grade"
>;

export type DashboardSnapshot = {
  updated_at: string;
  stats: DashboardStats;
  health: Pick<Health, "ok" | "nodes" | "running_job" | "publish_count" | "publish_fresh">;
  trends: DailyMetric[];
  top: DashboardNode[];
  countries: CountryRow[];
  sources: DashboardSource[];
};

export type CursorPage = {
  total?: number;
  count: number;
  next_cursor: string;
  has_more: boolean;
};

export type TokenRecord = {
  id: string;
  name: string;
  token_prefix: string;
  enabled: boolean;
  max_rps: number;
  allow_countries?: string[];
  allow_protocols?: string[];
  tenant_id: string;
  daily_quota: number;
  requests_today: number;
  bytes_today: number;
  expires_at?: string;
  created_at: string;
  last_used_at?: string;
  note?: string;
  token?: string;
};

export type UserRecord = {
  id: string;
  tenant_id: string;
  username: string;
  email?: string;
  role: "viewer" | "operator" | "admin";
  enabled: boolean;
  created_at: string;
  last_login_at?: string;
};

export type AuditEntry = {
  id: number;
  at: string;
  actor: string;
  action: string;
  detail: string;
};

export type AlertRecord = {
  id: string;
  kind: string;
  severity: string;
  message: string;
  details?: Record<string, unknown>;
  active: boolean;
  created_at: string;
  resolved_at?: string;
  acknowledged_at?: string;
  acknowledged_by?: string;
};

export type DailyMetric = {
  day: string;
  samples: number;
  success_rate: number;
  p50_latency_ms: number;
  p95_latency_ms: number;
  avg_score: number;
  avg_throughput_bps: number;
};

export type Pool = {
  key: string;
  title: string;
  description: string;
  count: number;
  refresh_sec: number;
  min_score: number;
  max_nodes: number;
  urls: Record<"raw" | "base64" | "clash", string>;
};

export type Health = {
  ok: boolean;
  time: string;
  version: string;
  uptime_sec: number;
  nodes: number;
  running_job: boolean;
  geo_mmdb: boolean;
  database: { driver: string; ok: boolean };
  redis: { enabled: boolean; ok: boolean };
  publish_count: number;
  publish_updated_at?: string;
  publish_fresh: boolean;
  schedule: boolean;
  sources_unhealthy: number;
};

export type ConfigVersion = {
  id: string;
  actor: string;
  checksum: string;
  patch_json: string;
  created_at: string;
};

export type Terms = {
  title: string;
  terms_url?: string;
  notice: string;
  restrictions: string[];
};

function params(values: Record<string, string | number | boolean | undefined>) {
  const query = new URLSearchParams();
  Object.entries(values).forEach(([key, value]) => {
    if (value === undefined || value === "") return;
    query.set(key, String(value));
  });
  const encoded = query.toString();
  return encoded ? `?${encoded}` : "";
}

/**
 * Carries the HTTP status so callers can tell "you are not signed in" apart
 * from a genuine backend failure — the two need very different UI.
 */
export class ApiError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

/** 401 — the caller is not signed in at all. */
export function isUnauthenticated(cause: unknown): boolean {
  return cause instanceof ApiError && cause.status === 401;
}

/** 403 — signed in, but the role is not allowed to do this. */
export function isForbidden(cause: unknown): boolean {
  return cause instanceof ApiError && cause.status === 403;
}

/** True when the request failed only because the caller lacks credentials. */
export function isAuthError(cause: unknown): boolean {
  return isUnauthenticated(cause) || isForbidden(cause);
}

/**
 * Human-readable message for a failed request. 401 and 403 need different
 * wording — telling a signed-in operator to "log in" sends them in circles.
 */
export function errorMessage(cause: unknown, fallback = "请求失败"): string {
  if (isForbidden(cause)) return "当前账号的角色无权执行此操作";
  if (isUnauthenticated(cause)) return "需要登录后才能访问";
  if (cause instanceof Error && cause.message.trim()) return cause.message;
  return fallback;
}

/**
 * Go marshals an empty slice as JSON `null`, so several list endpoints answer
 * `null` instead of `[]` when they have nothing to return. Callers render these
 * with `.map()`, so normalise here rather than guarding at every call site.
 */
async function requestList<T>(path: string, init?: RequestInit): Promise<T[]> {
  return (await request<T[] | null>(path, init)) ?? [];
}

async function request<T>(path: string, init?: RequestInit, acceptErrorStatus = false): Promise<T> {
  const res = await fetch(path, {
    ...init,
    credentials: "same-origin",
    headers: {
      ...(init?.body ? { "Content-Type": "application/json" } : {}),
      ...(init?.headers || {}),
    },
    cache: "no-store",
  });
  if (!res.ok && !acceptErrorStatus) {
    const message = (await res.text()).trim();
    throw new ApiError(res.status, message || `${res.status} ${res.statusText}`);
  }
  return res.json() as Promise<T>;
}

export const api = {
  health: () => request<Health>("/api/health"),
  ready: () =>
    request<{ ready: boolean; reasons: string[]; time: string }>("/api/ready", undefined, true),
  version: () => request<Record<string, unknown>>("/api/version"),
  terms: () => request<Terms>("/api/terms"),
  me: () => request<SessionInfo>("/api/v1/auth/me"),
  login: (body: { tenant: string; username: string; password: string }) =>
    request<{ authenticated: boolean; principal: Principal }>("/api/v1/auth/login", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  logout: () => request<{ authenticated: boolean }>("/api/v1/auth/logout", { method: "POST" }),
  dashboard: async () => {
    const snapshot = await request<DashboardSnapshot>("/api/public/dashboard");
    return {
      ...snapshot,
      trends: snapshot.trends ?? [],
      top: snapshot.top ?? [],
      countries: snapshot.countries ?? [],
      sources: snapshot.sources ?? [],
    };
  },
  stats: () => request<DashboardStats>("/api/stats"),
  trends: (days = 30) => requestList<DailyMetric>(`/api/stats/trends?days=${days}`),
  nodes: async (values: Record<string, string | number | boolean | undefined> = {}) => {
    const page = await request<{
      total: number;
      count: number;
      nodes: NodeItem[] | null;
      next_cursor: string;
      has_more: boolean;
    }>(`/api/nodes${params(values)}`);
    return { ...page, nodes: page.nodes ?? [] };
  },
  node: (id: string) => request<NodeItem>(`/api/nodes/${encodeURIComponent(id)}`),
  nodeMetrics: (id: string, days = 30) =>
    requestList<DailyMetric>(`/api/nodes/${encodeURIComponent(id)}/metrics?days=${days}`),
  // `jobs` is a nil slice server-side until the first job runs, so a fresh
  // install answers {"jobs": null} and every caller indexing it would throw.
  jobs: async (values: Record<string, string | number | boolean | undefined> = {}) => {
    const page = await request<{ jobs: Job[] | null; next_cursor: string; has_more: boolean }>(
      `/api/jobs${params(values)}`,
    );
    return { ...page, jobs: page.jobs ?? [] };
  },
  job: (id: string) => request<Job>(`/api/jobs/${encodeURIComponent(id)}`),
  jobEvents: (id: string, after = 0) =>
    requestList<JobEvent>(`/api/jobs/${encodeURIComponent(id)}/events?after=${after}`),
  startFetch: (opts: Record<string, unknown> = {}) =>
    request<Job>("/api/jobs/fetch", { method: "POST", body: JSON.stringify(opts) }),
  startQuality: (opts: Record<string, unknown> = {}) =>
    request<Job>("/api/jobs/quality", { method: "POST", body: JSON.stringify(opts) }),
  startAI: (opts: Record<string, unknown> = {}) =>
    request<Job>("/api/jobs/ai", { method: "POST", body: JSON.stringify(opts) }),
  startFull: (opts: Record<string, unknown> = {}) =>
    request<Job>("/api/jobs/full", { method: "POST", body: JSON.stringify(opts) }),
  startGeo: (opts: Record<string, unknown> = {}) =>
    request<Job>("/api/jobs/geo", { method: "POST", body: JSON.stringify(opts) }),
  cancelTask: (id: string) =>
    request<{ id: string; canceled: boolean }>(`/api/admin/tasks/${encodeURIComponent(id)}/cancel`, {
      method: "POST",
    }),
  tasks: (status = "") =>
    requestList<QueuedTask>(`/api/admin/tasks${params({ limit: 200, status })}`),
  tasksPage: async (values: Record<string, string | number | boolean | undefined> = {}) => {
    const page = await request<CursorPage & { tasks: QueuedTask[] | null }>(
      `/api/admin/tasks${params({ page: 1, limit: 30, ...values })}`,
    );
    return { ...page, tasks: page.tasks ?? [] };
  },
  queue: () =>
    request<{ enabled: boolean; workers?: number; tasks?: Record<string, number> }>(
      "/api/admin/queue",
    ),
  sources: (sort = "priority") =>
    requestList<Source>(`/api/sources${params({ sort })}`),
  sourcesPage: async (sort = "priority", values: Record<string, string | number | boolean | undefined> = {}) => {
    const page = await request<CursorPage & { sources: Source[] | null }>(
      `/api/sources${params({ page: 1, limit: 25, sort, ...values })}`,
    );
    return { ...page, sources: page.sources ?? [] };
  },
  setSourceEnabled: (name: string, enabled: boolean) =>
    request<{ name: string; enabled: boolean }>(
      `/api/admin/sources/${encodeURIComponent(name)}/${enabled ? "enable" : "disable"}`,
      { method: "POST" },
    ),
  probeSource: (name: string) =>
    request<{ state: Source }>(`/api/admin/sources/${encodeURIComponent(name)}/probe`, {
      method: "POST",
    }),
  countries: async (values: Record<string, string | number | boolean | undefined> = {}) => {
    const page = await request<{
      total_countries: number;
      total_nodes: number;
      countries: CountryRow[] | null;
    }>(`/api/countries${params(values)}`);
    return { ...page, countries: page.countries ?? [] };
  },
  aiTargets: () => requestList<AITarget>("/api/ai/targets"),
  hostAI: () => request<Record<string, AIProbeResult>>("/api/ai/host"),
  config: () => request<Record<string, unknown>>("/api/config"),
  updateConfig: (patch: Record<string, unknown>) =>
    request<{ version: ConfigVersion; config: Record<string, unknown> }>("/api/admin/config", {
      method: "PATCH",
      body: JSON.stringify({ ...patch, confirm: true }),
    }),
  configVersions: () => requestList<ConfigVersion>("/api/admin/config/versions?limit=50"),
  schedule: () => request<Record<string, unknown>>("/api/schedule"),
  pools: () => requestList<Pool>("/api/pools"),
  tokens: () => requestList<TokenRecord>("/api/admin/tokens"),
  tokensPage: async (values: Record<string, string | number | boolean | undefined> = {}) => {
    const page = await request<CursorPage & { tokens: TokenRecord[] | null }>(
      `/api/admin/tokens${params({ page: 1, limit: 25, ...values })}`,
    );
    return { ...page, tokens: page.tokens ?? [] };
  },
  createToken: (body: Record<string, unknown>) =>
    request<TokenRecord>("/api/admin/tokens", { method: "POST", body: JSON.stringify(body) }),
  setTokenEnabled: (id: string, enabled: boolean) =>
    request<{ id: string; enabled: boolean }>(
      `/api/admin/tokens/${encodeURIComponent(id)}/${enabled ? "enable" : "disable"}`,
      { method: "POST" },
    ),
  deleteToken: (id: string) =>
    request<{ deleted: string }>(`/api/admin/tokens/${encodeURIComponent(id)}`, {
      method: "DELETE",
    }),
  users: () => requestList<UserRecord>("/api/admin/users"),
  usersPage: async (values: Record<string, string | number | boolean | undefined> = {}) => {
    const page = await request<CursorPage & { users: UserRecord[] | null }>(
      `/api/admin/users${params({ page: 1, limit: 25, ...values })}`,
    );
    return { ...page, users: page.users ?? [] };
  },
  createUser: (body: Record<string, unknown>) =>
    request<UserRecord>("/api/admin/users", { method: "POST", body: JSON.stringify(body) }),
  setUserEnabled: (id: string, enabled: boolean) =>
    request<{ id: string; enabled: boolean }>(
      `/api/admin/users/${encodeURIComponent(id)}/${enabled ? "enable" : "disable"}`,
      { method: "POST" },
    ),
  audit: (values: Record<string, string | number | undefined> = {}) =>
    requestList<AuditEntry>(`/api/admin/audit${params({ limit: 200, ...values })}`),
  auditPage: async (values: Record<string, string | number | boolean | undefined> = {}) => {
    const page = await request<CursorPage & { entries: AuditEntry[] | null }>(
      `/api/admin/audit${params({ page: 1, limit: 50, ...values })}`,
    );
    return { ...page, entries: page.entries ?? [] };
  },
  alerts: (active = true) =>
    requestList<AlertRecord>(`/api/admin/alerts${params({ active, limit: 200 })}`),
  alertsPage: async (values: Record<string, string | number | boolean | undefined> = {}) => {
    const page = await request<CursorPage & { alerts: AlertRecord[] | null }>(
      `/api/admin/alerts${params({ page: 1, limit: 30, ...values })}`,
    );
    return { ...page, alerts: page.alerts ?? [] };
  },
  changeAlert: (id: string, action: "acknowledge" | "resolve") =>
    request<Record<string, unknown>>(
      `/api/admin/alerts/${encodeURIComponent(id)}/${action}`,
      { method: "POST" },
    ),
};

export function exportRawUrl(values: Record<string, string> = {}) {
  return `/api/export/raw${params({ hq: 1, alive: 1, limit: 500, ...values })}`;
}

export function exportBase64Url(values: Record<string, string> = {}) {
  return `/api/export/base64${params({ hq: 1, alive: 1, limit: 500, ...values })}`;
}

export function subUrl(kind: "raw" | "base64" | "clash", values: Record<string, string> = {}) {
  return `/sub/${kind}${params(values)}`;
}
