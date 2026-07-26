"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  Database,
  GitCommitHorizontal,
  RefreshCw,
  Save,
  ServerCog,
} from "lucide-react";
import { api, errorMessage, isAuthError, type ConfigVersion, type Health } from "@/lib/api";
import { AuthRequired } from "@/components/auth-required";
import { PageHeader } from "@/components/page-header";
import { useSession } from "@/components/session-provider";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardEmpty,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Field, Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
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
  const { authenticated, loading: sessionLoading } = useSession();
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
      const [cfg, healthState, readyState] = await Promise.all([
        api.config(),
        api.health(),
        api.ready(),
      ]);
      const publish = (cfg.publish || {}) as Record<string, unknown>;
      const governance = (cfg.governance || {}) as Record<string, unknown>;
      const dial = (cfg.dial || {}) as Record<string, unknown>;
      setConfig(cfg);
      setHealth(healthState);
      setReady(readyState);
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
      if (!isAuthError(cause)) setError(errorMessage(cause, "加载失败"));
    }
    // Version history is admin-only; fetch it separately so its failure
    // never clears the public config/health/ready state above.
    if (!authenticated) {
      setVersions([]);
      return;
    }
    try {
      setVersions(await api.configVersions());
    } catch (cause) {
      if (!isAuthError(cause)) setError(errorMessage(cause, "加载失败"));
    }
  }, [authenticated]);

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
      if (!isAuthError(cause)) setError(errorMessage(cause, "保存失败"));
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
        actions={
          <Button size="sm" variant="secondary" onClick={load}>
            <RefreshCw className="size-3.5" />
            刷新
          </Button>
        }
      />
      <div className="reveal space-y-4 p-4 sm:p-6 lg:p-8">
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <Card>
            <CardContent className="p-4">
              <Database className="mb-3 size-4 text-primary" />
              <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-muted-foreground">
                Database
              </p>
              <p className="mt-1 text-sm text-foreground">
                {String(database.driver || "—")} · {health?.database.ok ? "healthy" : "unavailable"}
              </p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-4">
              <ServerCog className="mb-3 size-4 text-accent" />
              <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-muted-foreground">
                Redis / Queue
              </p>
              <p className="mt-1 text-sm text-foreground">
                {redis.enabled ? "Redis on" : "Redis off"} ·{" "}
                {queue.enabled ? `${queue.embedded_workers || 0} embedded` : "queue off"}
              </p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-4">
              <Activity className="mb-3 size-4 text-success" />
              <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-muted-foreground">
                Readiness
              </p>
              <p className="mt-1 text-sm text-foreground">
                {ready?.ready ? "Ready to serve" : "Not ready"}
              </p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-4">
              <GitCommitHorizontal className="mb-3 size-4 text-destructive" />
              <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-muted-foreground">
                Version / Uptime
              </p>
              <p className="mt-1 text-sm text-foreground">
                {health?.version || "—"} · {formatDuration(health?.uptime_sec)}
              </p>
            </CardContent>
          </Card>
        </div>

        {ready && !ready.ready && (
          <Alert variant="danger">
            <AlertTriangle />
            <AlertDescription>{ready.reasons.join(" · ")}</AlertDescription>
          </Alert>
        )}
        {error && (
          <Alert variant="danger">
            <AlertTriangle />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        {message && (
          <Alert variant="success" role="status">
            <CheckCircle2 />
            <AlertDescription>{message}</AlertDescription>
          </Alert>
        )}

        <div className="grid gap-4 xl:grid-cols-[1.25fr_.75fr]">
          <Card>
            <CardHeader>
              <CardTitle>运行时配置</CardTitle>
              <CardDescription>保存后立即生效、写入数据库版本并刷新订阅缓存。</CardDescription>
            </CardHeader>
            <CardContent>
              {!authenticated && !sessionLoading ? (
                <AuthRequired
                  compact
                  title="需要登录后修改"
                  description="运行时配置的变更会写入审计与配置版本。"
                />
              ) : (
              <form onSubmit={save} className="space-y-6">
                <fieldset>
                  <legend className="mb-3 font-mono text-[10px] uppercase tracking-[0.18em] text-primary">
                    Publish policy
                  </legend>
                  <div className="control-grid">
                    <Field label="最低评分" htmlFor="publish_min_score">
                      <Input
                        id="publish_min_score"
                        type="number"
                        min="0"
                        max="100"
                        value={form.publish_min_score}
                        onChange={(e) => field("publish_min_score", e.target.value)}
                        disabled={!authenticated}
                      />
                    </Field>
                    <Field label="最多节点" htmlFor="publish_max_nodes">
                      <Input
                        id="publish_max_nodes"
                        type="number"
                        min="1"
                        max="100000"
                        value={form.publish_max_nodes}
                        onChange={(e) => field("publish_max_nodes", e.target.value)}
                        disabled={!authenticated}
                      />
                    </Field>
                    <Field label="缓存秒数" htmlFor="publish_cache_sec">
                      <Input
                        id="publish_cache_sec"
                        type="number"
                        min="0"
                        max="86400"
                        value={form.publish_cache_sec}
                        onChange={(e) => field("publish_cache_sec", e.target.value)}
                        disabled={!authenticated}
                      />
                    </Field>
                    <Field label="最大节点年龄（小时）" htmlFor="publish_max_node_age_hours">
                      <Input
                        id="publish_max_node_age_hours"
                        type="number"
                        min="1"
                        max="8760"
                        value={form.publish_max_node_age_hours}
                        onChange={(e) => field("publish_max_node_age_hours", e.target.value)}
                        disabled={!authenticated}
                      />
                    </Field>
                  </div>
                  <div className="mt-3 flex items-center gap-2">
                    <Switch
                      id="publish_alive_only"
                      checked={form.publish_alive_only}
                      onCheckedChange={(state) => field("publish_alive_only", state)}
                      disabled={!authenticated}
                    />
                    <Label htmlFor="publish_alive_only">仅发布存活节点</Label>
                  </div>
                </fieldset>

                <fieldset>
                  <legend className="mb-3 font-mono text-[10px] uppercase tracking-[0.18em] text-accent">
                    Governance thresholds
                  </legend>
                  <div className="control-grid">
                    <Field label="连续失败停源" htmlFor="governance_disable_after_failures">
                      <Input
                        id="governance_disable_after_failures"
                        type="number"
                        min="1"
                        max="100"
                        value={form.governance_disable_after_failures}
                        onChange={(e) => field("governance_disable_after_failures", e.target.value)}
                        disabled={!authenticated}
                      />
                    </Field>
                    <Field label="冷却小时" htmlFor="governance_cooldown_hours">
                      <Input
                        id="governance_cooldown_hours"
                        type="number"
                        min="1"
                        max="720"
                        value={form.governance_cooldown_hours}
                        onChange={(e) => field("governance_cooldown_hours", e.target.value)}
                        disabled={!authenticated}
                      />
                    </Field>
                    <Field label="HQ 骤降阈值 %" htmlFor="governance_hq_drop_percent">
                      <Input
                        id="governance_hq_drop_percent"
                        type="number"
                        min="1"
                        max="100"
                        value={form.governance_hq_drop_percent}
                        onChange={(e) => field("governance_hq_drop_percent", e.target.value)}
                        disabled={!authenticated}
                      />
                    </Field>
                    <Field label="单国家占比阈值 %" htmlFor="governance_country_share_percent">
                      <Input
                        id="governance_country_share_percent"
                        type="number"
                        min="1"
                        max="100"
                        value={form.governance_country_share_percent}
                        onChange={(e) => field("governance_country_share_percent", e.target.value)}
                        disabled={!authenticated}
                      />
                    </Field>
                  </div>
                </fieldset>

                <fieldset>
                  <legend className="mb-3 font-mono text-[10px] uppercase tracking-[0.18em] text-success">
                    Real protocol verification
                  </legend>
                  <div className="flex flex-wrap items-end gap-4">
                    <div className="flex items-center gap-2 pb-3">
                      <Switch
                        id="dial_after_quality"
                        checked={form.dial_after_quality}
                        onCheckedChange={(state) => field("dial_after_quality", state)}
                        disabled={!authenticated}
                      />
                      <Label htmlFor="dial_after_quality">质量任务后抽样真实拨测</Label>
                    </div>
                    <Field
                      label="每次最多节点"
                      htmlFor="dial_after_quality_max"
                      className="min-w-48 flex-1"
                    >
                      <Input
                        id="dial_after_quality_max"
                        type="number"
                        min="0"
                        max="100000"
                        value={form.dial_after_quality_max}
                        onChange={(e) => field("dial_after_quality_max", e.target.value)}
                        disabled={!authenticated}
                      />
                    </Field>
                  </div>
                </fieldset>

                <div className="flex flex-col gap-3 border-t border-border pt-5 sm:flex-row sm:items-end">
                  <Field
                    label={<span className="text-destructive">输入 APPLY 确认变更</span>}
                    htmlFor="confirm-apply"
                    className="min-w-0 flex-1"
                  >
                    <Input
                      id="confirm-apply"
                      className="border-destructive/40"
                      value={confirm}
                      onChange={(e) => setConfirm(e.target.value)}
                      autoComplete="off"
                      disabled={!authenticated}
                    />
                  </Field>
                  <Button
                    type="submit"
                    variant="destructive"
                    disabled={busy || confirm !== "APPLY" || !authenticated}
                    title={!authenticated ? "需要登录后才能应用配置" : undefined}
                  >
                    <Save className="size-4" /> {busy ? "应用中…" : "应用配置"}
                  </Button>
                </div>
              </form>
              )}
            </CardContent>
          </Card>

          <div className="space-y-4">
            <Card>
              <CardHeader>
                <CardTitle>运行能力</CardTitle>
                <CardDescription>仅显示状态，不返回密钥或 DSN。</CardDescription>
              </CardHeader>
              <CardContent className="space-y-2 text-xs">
                {[
                  ["本地登录", auth.local_enabled],
                  ["OIDC", auth.oidc_enabled],
                  ["OpenTelemetry", observability.otel_enabled],
                  ["GeoIP", health?.geo_mmdb],
                  ["发布缓存", health?.publish_fresh],
                ].map(([label, value]) => (
                  <div
                    key={String(label)}
                    className="flex items-center justify-between border-b border-border/60 py-2 last:border-0"
                  >
                    <span className="text-muted-foreground">{String(label)}</span>
                    <Badge variant={value ? "success" : "secondary"}>{value ? "ON" : "OFF"}</Badge>
                  </div>
                ))}
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle>配置版本</CardTitle>
                <CardDescription>最近 50 次确认后的热更新。</CardDescription>
              </CardHeader>
              <CardContent className="max-h-[34rem] space-y-3 overflow-y-auto">
                {!authenticated && !sessionLoading && (
                  <AuthRequired compact title="需要登录" description="配置版本历史属于管理面。" />
                )}
                {versions.map((version) => (
                  <div key={version.id} className="border-l-2 border-border pl-3">
                    <div className="flex items-center justify-between gap-2">
                      <code className="text-[10px] text-primary">{version.checksum.slice(0, 12)}</code>
                      <span className="font-mono text-[9px] text-muted-foreground">
                        {formatTime(version.created_at)}
                      </span>
                    </div>
                    <p className="mt-1 text-xs text-muted-foreground">{version.actor}</p>
                    <details className="mt-1">
                      <summary className="cursor-pointer text-[10px] text-muted-foreground">
                        查看 patch
                      </summary>
                      <pre className="mt-2 overflow-auto border border-border bg-popover p-2 font-mono text-[9px] text-muted-foreground">
                        {version.patch_json}
                      </pre>
                    </details>
                  </div>
                ))}
                {versions.length === 0 && authenticated && (
                  <CardEmpty>暂无热更新版本</CardEmpty>
                )}
              </CardContent>
            </Card>
          </div>
        </div>
      </div>
    </div>
  );
}
