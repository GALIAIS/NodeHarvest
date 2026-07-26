"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { Activity, Database, GitCommitHorizontal, RefreshCw, Save, ServerCog } from "lucide-react";
import { api, type ConfigVersion, type Health } from "@/lib/api";
import { PageHeader } from "@/components/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { formatDuration, formatTime } from "@/lib/utils";

type RuntimeForm = {
  publish_min_score: string;
  publish_max_nodes: string;
  publish_alive_only: boolean;
  publish_cache_sec: string;
  publish_max_node_age_hours: string;
  governance_disable_after_failures: string;
  governance_cooldown_hours: string;
  governance_hq_drop_percent: string;
  governance_country_share_percent: string;
  dial_after_quality: boolean;
  dial_after_quality_max: string;
};

const empty: RuntimeForm = {
  publish_min_score: "",
  publish_max_nodes: "",
  publish_alive_only: true,
  publish_cache_sec: "",
  publish_max_node_age_hours: "",
  governance_disable_after_failures: "",
  governance_cooldown_hours: "",
  governance_hq_drop_percent: "",
  governance_country_share_percent: "",
  dial_after_quality: false,
  dial_after_quality_max: "",
};

export default function SystemPage() {
  const [config, setConfig] = useState<Record<string, unknown>>({});
  const [health, setHealth] = useState<Health | null>(null);
  const [ready, setReady] = useState<{ ready: boolean; reasons: string[] } | null>(null);
  const [versions, setVersions] = useState<ConfigVersion[]>([]);
  const [form, setForm] = useState<RuntimeForm>(empty);
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    try {
      const [cfg, healthState, readyState, versionList] = await Promise.all([
        api.config(),
        api.health(),
        api.ready(),
        api.configVersions(),
      ]);
      const publish = (cfg.publish || {}) as Record<string, unknown>;
      const governance = (cfg.governance || {}) as Record<string, unknown>;
      const dial = (cfg.dial || {}) as Record<string, unknown>;
      setConfig(cfg);
      setHealth(healthState);
      setReady(readyState);
      setVersions(versionList);
      setForm({
        publish_min_score: String(publish.min_score ?? ""),
        publish_max_nodes: String(publish.max_nodes ?? ""),
        publish_alive_only: Boolean(publish.alive_only),
        publish_cache_sec: String(publish.cache_sec ?? ""),
        publish_max_node_age_hours: String(publish.max_node_age_hours ?? ""),
        governance_disable_after_failures: String(governance.disable_after_failures ?? ""),
        governance_cooldown_hours: String(governance.cooldown_hours ?? ""),
        governance_hq_drop_percent: String(governance.hq_drop_percent ?? ""),
        governance_country_share_percent: String(governance.country_share_percent ?? ""),
        dial_after_quality: Boolean(dial.after_quality),
        dial_after_quality_max: String(dial.after_quality_max ?? ""),
      });
      setError("");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "加载失败");
    }
  }, []);

  useEffect(() => {
    const initial = setTimeout(load, 0);
    return () => clearTimeout(initial);
  }, [load]);

  function field<K extends keyof RuntimeForm>(key: K, value: RuntimeForm[K]) {
    setForm((current) => ({ ...current, [key]: value }));
  }

  async function save(event: FormEvent) {
    event.preventDefault();
    if (confirm !== "APPLY") {
      setError("请输入 APPLY 确认运行时配置变更");
      return;
    }
    setBusy(true);
    setError("");
    setMessage("");
    const patch = Object.fromEntries(
      Object.entries(form).map(([key, value]) => [
        key,
        typeof value === "boolean" ? value : Number(value),
      ]),
    );
    try {
      const result = await api.updateConfig(patch);
      setMessage(`配置版本 ${result.version.id} 已应用并写入审计`);
      setConfirm("");
      await load();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "保存失败");
    } finally {
      setBusy(false);
    }
  }

  const database = (config.database || {}) as Record<string, unknown>;
  const redis = (config.redis || {}) as Record<string, unknown>;
  const queue = (config.queue || {}) as Record<string, unknown>;
  const auth = (config.auth || {}) as Record<string, unknown>;
  const observability = (config.observability || {}) as Record<string, unknown>;

  return (
    <div className="flex flex-1 flex-col">
      <PageHeader
        eyebrow="Runtime control plane"
        title="系统与热配置"
        description="查看依赖、就绪状态、版本与运行时安全项。监听地址、数据库、凭证和拓扑变更仍需滚动重启。"
        actions={<Button size="sm" variant="secondary" onClick={load}><RefreshCw className="h-3.5 w-3.5" />刷新</Button>}
      />
      <div className="reveal space-y-5 p-4 sm:p-6 lg:p-8">
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <Card><CardContent className="p-4"><Database className="mb-3 h-4 w-4 text-cyan-300" /><p className="text-xs text-slate-600">Database</p><p className="mt-1 text-sm text-slate-200">{String(database.driver || "—")} · {health?.database.ok ? "healthy" : "unavailable"}</p></CardContent></Card>
          <Card><CardContent className="p-4"><ServerCog className="mb-3 h-4 w-4 text-amber-300" /><p className="text-xs text-slate-600">Redis / Queue</p><p className="mt-1 text-sm text-slate-200">{redis.enabled ? "Redis on" : "Redis off"} · {queue.enabled ? `${queue.embedded_workers || 0} embedded` : "queue off"}</p></CardContent></Card>
          <Card><CardContent className="p-4"><Activity className="mb-3 h-4 w-4 text-emerald-300" /><p className="text-xs text-slate-600">Readiness</p><p className="mt-1 text-sm text-slate-200">{ready?.ready ? "Ready to serve" : "Not ready"}</p></CardContent></Card>
          <Card><CardContent className="p-4"><GitCommitHorizontal className="mb-3 h-4 w-4 text-rose-300" /><p className="text-xs text-slate-600">Version / Uptime</p><p className="mt-1 text-sm text-slate-200">{health?.version || "—"} · {formatDuration(health?.uptime_sec)}</p></CardContent></Card>
        </div>

        {ready && !ready.ready && (
          <div className="rounded-md border border-rose-500/30 bg-rose-500/10 px-4 py-3 text-sm text-rose-300">
            {ready.reasons.join(" · ")}
          </div>
        )}
        {error && <div role="alert" className="rounded-md border border-rose-500/30 bg-rose-500/10 px-4 py-3 text-sm text-rose-300">{error}</div>}
        {message && <div role="status" className="rounded-md border border-emerald-500/30 bg-emerald-500/10 px-4 py-3 text-sm text-emerald-300">{message}</div>}

        <div className="grid gap-4 xl:grid-cols-[1.25fr_.75fr]">
          <Card>
            <CardHeader>
              <CardTitle>运行时配置</CardTitle>
              <CardDescription>保存后立即生效、写入数据库版本并刷新订阅缓存。</CardDescription>
            </CardHeader>
            <CardContent>
              <form onSubmit={save} className="space-y-6">
                <fieldset>
                  <legend className="mb-3 font-mono text-[10px] uppercase tracking-[0.2em] text-cyan-400">Publish policy</legend>
                  <div className="control-grid">
                    <label className="text-xs text-slate-500">最低评分<Input className="mt-1.5" type="number" min="0" max="100" value={form.publish_min_score} onChange={(e) => field("publish_min_score", e.target.value)} /></label>
                    <label className="text-xs text-slate-500">最多节点<Input className="mt-1.5" type="number" min="1" max="100000" value={form.publish_max_nodes} onChange={(e) => field("publish_max_nodes", e.target.value)} /></label>
                    <label className="text-xs text-slate-500">缓存秒数<Input className="mt-1.5" type="number" min="0" max="86400" value={form.publish_cache_sec} onChange={(e) => field("publish_cache_sec", e.target.value)} /></label>
                    <label className="text-xs text-slate-500">最大节点年龄（小时）<Input className="mt-1.5" type="number" min="1" max="8760" value={form.publish_max_node_age_hours} onChange={(e) => field("publish_max_node_age_hours", e.target.value)} /></label>
                  </div>
                  <label className="mt-3 flex items-center gap-2 text-xs text-slate-400"><input type="checkbox" className="accent-cyan-400" checked={form.publish_alive_only} onChange={(e) => field("publish_alive_only", e.target.checked)} />仅发布存活节点</label>
                </fieldset>

                <fieldset>
                  <legend className="mb-3 font-mono text-[10px] uppercase tracking-[0.2em] text-amber-400">Governance thresholds</legend>
                  <div className="control-grid">
                    <label className="text-xs text-slate-500">连续失败停源<Input className="mt-1.5" type="number" min="1" max="100" value={form.governance_disable_after_failures} onChange={(e) => field("governance_disable_after_failures", e.target.value)} /></label>
                    <label className="text-xs text-slate-500">冷却小时<Input className="mt-1.5" type="number" min="1" max="720" value={form.governance_cooldown_hours} onChange={(e) => field("governance_cooldown_hours", e.target.value)} /></label>
                    <label className="text-xs text-slate-500">HQ 骤降阈值 %<Input className="mt-1.5" type="number" min="1" max="100" value={form.governance_hq_drop_percent} onChange={(e) => field("governance_hq_drop_percent", e.target.value)} /></label>
                    <label className="text-xs text-slate-500">单国家占比阈值 %<Input className="mt-1.5" type="number" min="1" max="100" value={form.governance_country_share_percent} onChange={(e) => field("governance_country_share_percent", e.target.value)} /></label>
                  </div>
                </fieldset>

                <fieldset>
                  <legend className="mb-3 font-mono text-[10px] uppercase tracking-[0.2em] text-emerald-400">Real protocol verification</legend>
                  <div className="flex flex-wrap items-end gap-4">
                    <label className="flex items-center gap-2 pb-3 text-xs text-slate-400"><input type="checkbox" className="accent-emerald-400" checked={form.dial_after_quality} onChange={(e) => field("dial_after_quality", e.target.checked)} />质量任务后抽样真实拨测</label>
                    <label className="min-w-48 flex-1 text-xs text-slate-500">每次最多节点<Input className="mt-1.5" type="number" min="0" max="100000" value={form.dial_after_quality_max} onChange={(e) => field("dial_after_quality_max", e.target.value)} /></label>
                  </div>
                </fieldset>

                <div className="flex flex-col gap-3 border-t border-slate-800 pt-5 sm:flex-row sm:items-end">
                  <label className="min-w-0 flex-1 text-xs text-rose-300">
                    输入 APPLY 确认变更
                    <Input className="mt-1.5 border-rose-500/30" value={confirm} onChange={(e) => setConfirm(e.target.value)} autoComplete="off" />
                  </label>
                  <Button type="submit" variant="danger" disabled={busy || confirm !== "APPLY"}>
                    <Save className="h-4 w-4" /> {busy ? "应用中…" : "应用配置"}
                  </Button>
                </div>
              </form>
            </CardContent>
          </Card>

          <div className="space-y-4">
            <Card>
              <CardHeader><CardTitle>运行能力</CardTitle><CardDescription>仅显示状态，不返回密钥或 DSN。</CardDescription></CardHeader>
              <CardContent className="space-y-2 text-xs">
                {[
                  ["本地登录", auth.local_enabled],
                  ["OIDC", auth.oidc_enabled],
                  ["OpenTelemetry", observability.otel_enabled],
                  ["GeoIP", health?.geo_mmdb],
                  ["发布缓存", health?.publish_fresh],
                ].map(([label, value]) => (
                  <div key={String(label)} className="flex items-center justify-between border-b border-slate-800/60 py-2 last:border-0">
                    <span className="text-slate-500">{String(label)}</span>
                    <Badge variant={value ? "success" : "secondary"}>{value ? "ON" : "OFF"}</Badge>
                  </div>
                ))}
              </CardContent>
            </Card>
            <Card>
              <CardHeader><CardTitle>配置版本</CardTitle><CardDescription>最近 50 次确认后的热更新。</CardDescription></CardHeader>
              <CardContent className="max-h-[34rem] space-y-3 overflow-y-auto">
                {versions.map((version) => (
                  <div key={version.id} className="border-l border-slate-700 pl-3">
                    <div className="flex items-center justify-between gap-2">
                      <code className="text-[10px] text-cyan-300">{version.checksum.slice(0, 12)}</code>
                      <span className="font-mono text-[9px] text-slate-700">{formatTime(version.created_at)}</span>
                    </div>
                    <p className="mt-1 text-xs text-slate-500">{version.actor}</p>
                    <details className="mt-1">
                      <summary className="cursor-pointer text-[10px] text-slate-700">查看 patch</summary>
                      <pre className="mt-2 overflow-auto rounded bg-slate-950 p-2 font-mono text-[9px] text-slate-600">{version.patch_json}</pre>
                    </details>
                  </div>
                ))}
                {versions.length === 0 && <p className="py-8 text-center text-xs text-slate-600">暂无热更新版本</p>}
              </CardContent>
            </Card>
          </div>
        </div>
      </div>
    </div>
  );
}
