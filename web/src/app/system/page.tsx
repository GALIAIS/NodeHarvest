"use client";

import type { FormEvent } from "react";
import { useCallback, useEffect, useState } from "react";
import { RefreshCw, Save } from "lucide-react";
import { AuthRequired } from "@/components/auth-required";
import { useLiveRefresh } from "@/components/live-provider";
import { PageHeader } from "@/components/page-header";
import { useSession } from "@/components/session-provider";
import { StatusBadge } from "@/components/status-badge";
import { TableEmpty } from "@/components/table-empty";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { Switch } from "@/components/ui/switch";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  api,
  errorMessage,
  type ConfigVersion,
  type Health,
} from "@/lib/api";
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

const emptyForm: RuntimeForm = {
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

function record(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" ? (value as Record<string, unknown>) : {};
}

export default function SystemPage() {
  const { authenticated, loading, canAdmin } = useSession();
  const [config, setConfig] = useState<Record<string, unknown>>({});
  const [health, setHealth] = useState<Health | null>(null);
  const [ready, setReady] = useState<{ ready: boolean; reasons: string[] } | null>(null);
  const [versions, setVersions] = useState<ConfigVersion[]>([]);
  const [form, setForm] = useState<RuntimeForm>(emptyForm);
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    try {
      const [nextConfig, nextHealth, nextReady] = await Promise.all([api.config(), api.health(), api.ready()]);
      const publish = record(nextConfig.publish);
      const governance = record(nextConfig.governance);
      const dial = record(nextConfig.dial);
      setConfig(nextConfig);
      setHealth(nextHealth);
      setReady(nextReady);
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
      setError(errorMessage(cause, "加载系统状态失败"));
    }

    if (!canAdmin) {
      setVersions([]);
      return;
    }
    try {
      setVersions(await api.configVersions());
    } catch (cause) {
      setError(errorMessage(cause, "加载配置版本失败"));
    }
  }, [canAdmin]);

  useEffect(() => {
    const initial = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(initial);
  }, [load]);

  useLiveRefresh(load, authenticated);

  function update<K extends keyof RuntimeForm>(key: K, value: RuntimeForm[K]) {
    setForm((current) => ({ ...current, [key]: value }));
  }

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (confirm !== "APPLY") {
      setError("请输入 APPLY 确认变更。");
      return;
    }
    setBusy(true);
    try {
      const patch = Object.fromEntries(
        Object.entries(form).map(([key, value]) => [key, typeof value === "boolean" ? value : Number(value)]),
      );
      const result = await api.updateConfig(patch);
      setMessage("配置版本 " + result.version.id + " 已应用。");
      setConfirm("");
      await load();
    } catch (cause) {
      setError(errorMessage(cause, "保存配置失败"));
    } finally {
      setBusy(false);
    }
  }

  const database = record(config.database);
  const redis = record(config.redis);
  const queue = record(config.queue);

  return (
    <>
      <PageHeader
        title="系统"
        description="查看服务健康状态，并管理可热更新的运行时配置。"
        actions={
          <Button type="button" variant="outline" onClick={() => void load()}>
            <RefreshCw />
            刷新
          </Button>
        }
      />
      {error && (
        <Alert variant="destructive">
          <AlertTitle>系统操作失败</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      {message && (
        <Alert>
          <AlertTitle>配置已应用</AlertTitle>
          <AlertDescription>{message}</AlertDescription>
        </Alert>
      )}
      <Card>
        <CardHeader>
          <CardTitle>运行状态</CardTitle>
          <CardDescription>服务依赖与当前就绪状态。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <div className="space-y-1">
              <dt className="text-sm text-muted-foreground">版本</dt>
              <dd className="font-medium">{health?.version ?? "—"}</dd>
            </div>
            <div className="space-y-1">
              <dt className="text-sm text-muted-foreground">运行时间</dt>
              <dd className="font-medium">{formatDuration(health?.uptime_sec)}</dd>
            </div>
            <div className="space-y-1">
              <dt className="text-sm text-muted-foreground">数据库</dt>
              <dd className="font-medium">{String(database.driver ?? "—") + " · " + (health?.database.ok ? "正常" : "不可用")}</dd>
            </div>
            <div className="space-y-1">
              <dt className="text-sm text-muted-foreground">Redis</dt>
              <dd className="font-medium">{redis.enabled ? (health?.redis.ok ? "正常" : "异常") : "未启用"}</dd>
            </div>
            <div className="space-y-1">
              <dt className="text-sm text-muted-foreground">队列</dt>
              <dd className="font-medium">{queue.enabled ? "已启用" : "未启用"}</dd>
            </div>
            <div className="space-y-1">
              <dt className="text-sm text-muted-foreground">就绪</dt>
              <dd>
                <StatusBadge status={ready?.ready ? "ready" : "unhealthy"}>
                  {ready ? (ready.ready ? "就绪" : "未就绪") : "加载中"}
                </StatusBadge>
              </dd>
            </div>
          </dl>
          {ready && !ready.ready && <p>{ready.reasons.join("；")}</p>}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>运行时配置</CardTitle>
          <CardDescription>保存后立即生效并写入配置版本。</CardDescription>
        </CardHeader>
        <CardContent>
          {!authenticated && !loading ? (
            <AuthRequired title="需要登录后修改配置" />
          ) : authenticated && !canAdmin ? (
            <AuthRequired reason="forbidden" title="需要 admin 角色" />
          ) : (
            <form className="space-y-6" onSubmit={save}>
              <section className="space-y-4" aria-labelledby="publish-settings">
                <div>
                  <h3 id="publish-settings" className="font-medium">发布策略</h3>
                  <p className="text-sm text-muted-foreground">控制订阅输出的质量、数量与缓存。</p>
                </div>
                <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
                  <div className="space-y-2">
                    <Label htmlFor="publish-score">发布最低评分</Label>
                    <Input
                      id="publish-score"
                      type="number"
                      min="0"
                      max="100"
                      value={form.publish_min_score}
                      onChange={(event) => update("publish_min_score", event.target.value)}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="publish-max-nodes">发布最多节点</Label>
                    <Input
                      id="publish-max-nodes"
                      type="number"
                      min="1"
                      max="100000"
                      value={form.publish_max_nodes}
                      onChange={(event) => update("publish_max_nodes", event.target.value)}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="publish-cache">发布缓存秒数</Label>
                    <Input
                      id="publish-cache"
                      type="number"
                      min="0"
                      max="86400"
                      value={form.publish_cache_sec}
                      onChange={(event) => update("publish_cache_sec", event.target.value)}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="publish-age">最大节点年龄（小时）</Label>
                    <Input
                      id="publish-age"
                      type="number"
                      min="1"
                      max="8760"
                      value={form.publish_max_node_age_hours}
                      onChange={(event) => update("publish_max_node_age_hours", event.target.value)}
                    />
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Switch
                    id="publish-alive-only"
                    checked={form.publish_alive_only}
                    onCheckedChange={(checked) => update("publish_alive_only", checked)}
                  />
                  <Label htmlFor="publish-alive-only">仅发布存活节点</Label>
                </div>
              </section>
              <Separator />
              <section className="space-y-4" aria-labelledby="governance-settings">
                <div>
                  <h3 id="governance-settings" className="font-medium">来源治理</h3>
                  <p className="text-sm text-muted-foreground">设置停源、冷却与质量异常阈值。</p>
                </div>
                <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
                  <div className="space-y-2">
                    <Label htmlFor="governance-failures">连续失败停源阈值</Label>
                    <Input
                      id="governance-failures"
                      type="number"
                      min="1"
                      max="100"
                      value={form.governance_disable_after_failures}
                      onChange={(event) => update("governance_disable_after_failures", event.target.value)}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="governance-cooldown">冷却小时</Label>
                    <Input
                      id="governance-cooldown"
                      type="number"
                      min="1"
                      max="720"
                      value={form.governance_cooldown_hours}
                      onChange={(event) => update("governance_cooldown_hours", event.target.value)}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="governance-drop">HQ 骤降阈值（%）</Label>
                    <Input
                      id="governance-drop"
                      type="number"
                      min="1"
                      max="100"
                      value={form.governance_hq_drop_percent}
                      onChange={(event) => update("governance_hq_drop_percent", event.target.value)}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="governance-country">单国家占比阈值（%）</Label>
                    <Input
                      id="governance-country"
                      type="number"
                      min="1"
                      max="100"
                      value={form.governance_country_share_percent}
                      onChange={(event) => update("governance_country_share_percent", event.target.value)}
                    />
                  </div>
                </div>
              </section>
              <Separator />
              <section className="space-y-4" aria-labelledby="dial-settings">
                <div>
                  <h3 id="dial-settings" className="font-medium">真实拨测</h3>
                  <p className="text-sm text-muted-foreground">控制质量任务后的真实网络验证。</p>
                </div>
                <div className="grid gap-4 md:grid-cols-2">
                  <div className="flex items-center gap-2">
                    <Switch
                      id="dial-after-quality"
                      checked={form.dial_after_quality}
                      onCheckedChange={(checked) => update("dial_after_quality", checked)}
                    />
                    <Label htmlFor="dial-after-quality">质量任务后真实拨测</Label>
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="dial-max">每次最多拨测节点</Label>
                    <Input
                      id="dial-max"
                      type="number"
                      min="0"
                      max="100000"
                      value={form.dial_after_quality_max}
                      onChange={(event) => update("dial_after_quality_max", event.target.value)}
                    />
                  </div>
                </div>
              </section>
              <Separator />
              <section className="space-y-4" aria-labelledby="apply-settings">
                <div>
                  <h3 id="apply-settings" className="font-medium">确认应用</h3>
                  <p className="text-sm text-muted-foreground">输入 APPLY 后保存，配置会立即生效。</p>
                </div>
                <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
                  <div className="w-full max-w-sm space-y-2">
                    <Label htmlFor="config-confirm">确认文本</Label>
                    <Input
                      id="config-confirm"
                      value={confirm}
                      placeholder="APPLY"
                      autoComplete="off"
                      onChange={(event) => setConfirm(event.target.value)}
                    />
                  </div>
                  <Button type="submit" variant="destructive" disabled={busy || confirm !== "APPLY"}>
                    <Save />
                    {busy ? "应用中…" : "应用配置"}
                  </Button>
                </div>
              </section>
            </form>
          )}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>配置版本</CardTitle>
          <CardDescription>最近的运行时配置变更。</CardDescription>
        </CardHeader>
        <CardContent>
          {authenticated && canAdmin ? (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>版本</TableHead>
                  <TableHead>操作人</TableHead>
                  <TableHead>校验和</TableHead>
                  <TableHead>时间</TableHead>
                  <TableHead>变更</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {versions.map((version) => (
                  <TableRow key={version.id}>
                    <TableCell className="font-mono text-xs">{version.id}</TableCell>
                    <TableCell className="font-mono text-xs">{version.actor}</TableCell>
                    <TableCell className="max-w-64 truncate font-mono text-xs" title={version.checksum}>
                      {version.checksum}
                    </TableCell>
                    <TableCell>{formatTime(version.created_at)}</TableCell>
                    <TableCell>
                      <details>
                        <summary className="cursor-pointer text-sm font-medium">查看</summary>
                        <pre className="mt-2 max-w-xl overflow-x-auto rounded-md bg-muted p-4 text-xs">
                          {version.patch_json}
                        </pre>
                      </details>
                    </TableCell>
                  </TableRow>
                ))}
                {!versions.length && (
                  <TableEmpty colSpan={5}>暂无配置版本。</TableEmpty>
                )}
              </TableBody>
            </Table>
          ) : (
            <AuthRequired reason={authenticated ? "forbidden" : "anonymous"} title="配置版本需要 admin 权限" />
          )}
        </CardContent>
      </Card>
    </>
  );
}
