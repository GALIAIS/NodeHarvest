export type Quality = {
  rounds: number;
  success_rate: number;
  avg_latency_ms: number;
  min_latency_ms: number;
  max_latency_ms: number;
  jitter_ms: number;
  tls_ok: boolean;
  tls_ms?: number;
  edge_score: number;
  score: number;
  notes?: string[];
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
  raw_uri: string;
  source: string;
  alive: boolean;
  latency_ms?: number;
  score: number;
  grade: string;
  country?: string;
  city?: string;
  tags?: string[];
  quality?: Quality;
  ai_access?: Record<string, AIProbeResult>;
  error?: string;
  tested_at?: string;
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

export type Source = {
  name: string;
  type: string;
  url: string;
  enabled: boolean;
};

export type AITarget = {
  key: string;
  name: string;
  url: string;
  host: string;
};

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers || {}),
    },
    cache: "no-store",
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || res.statusText);
  }
  return res.json() as Promise<T>;
}

export const api = {
  health: () => request<{ ok: boolean }>("/api/health"),
  stats: () => request<DashboardStats>("/api/stats"),
  nodes: (params: Record<string, string | number | boolean | undefined> = {}) => {
    const q = new URLSearchParams();
    Object.entries(params).forEach(([k, v]) => {
      if (v === undefined || v === "" || v === false) return;
      q.set(k, String(v));
    });
    return request<{ total: number; nodes: NodeItem[] }>(`/api/nodes?${q}`);
  },
  node: (id: string) => request<NodeItem>(`/api/nodes/${id}`),
  jobs: () => request<Job[]>("/api/jobs"),
  job: (id: string) => request<Job>(`/api/jobs/${id}`),
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
  sources: () => request<Source[]>("/api/sources"),
  countries: (params: Record<string, string | number | boolean | undefined> = {}) => {
    const q = new URLSearchParams();
    Object.entries(params).forEach(([k, v]) => {
      if (v === undefined || v === "" || v === false) return;
      q.set(k, String(v));
    });
    return request<{
      total_countries: number;
      total_nodes: number;
      countries: CountryRow[];
    }>(`/api/countries?${q}`);
  },
  aiTargets: () => request<AITarget[]>("/api/ai/targets"),
  hostAI: () => request<Record<string, AIProbeResult>>("/api/ai/host"),
  config: () => request<Record<string, unknown>>("/api/config"),
  schedule: () => request<Record<string, unknown>>("/api/schedule"),
};

export function exportRawUrl(params: Record<string, string> = {}) {
  const q = new URLSearchParams({ hq: "1", alive: "1", limit: "500", ...params });
  return `/api/export/raw?${q}`;
}

export function exportBase64Url(params: Record<string, string> = {}) {
  const q = new URLSearchParams({ hq: "1", alive: "1", limit: "500", ...params });
  return `/api/export/base64?${q}`;
}

export function subUrl(kind: "raw" | "base64" | "clash", params: Record<string, string> = {}) {
  const q = new URLSearchParams(params);
  const qs = q.toString();
  return `/sub/${kind}${qs ? `?${qs}` : ""}`;
}
