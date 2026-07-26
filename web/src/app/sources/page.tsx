"use client";

import { useCallback, useEffect, useState } from "react";
import {
  Activity,
  AlertTriangle,
  Ban,
  ExternalLink,
  Play,
  Power,
  RefreshCw,
  ShieldAlert,
} from "lucide-react";
import { api, errorMessage, isAuthError, type Source } from "@/lib/api";
import { PageHeader } from "@/components/page-header";
import { useSession } from "@/components/session-provider";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardEmpty } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn, formatBytes, formatMs, formatPercent, formatTime } from "@/lib/utils";

export default function SourcesPage() {
  const { canOperate } = useSession();
  const [sources, setSources] = useState<Source[]>([]);
  const [sort, setSort] = useState("priority");
  const [busy, setBusy] = useState("");
  const [err, setErr] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      setSources(await api.sources(sort));
      setErr(null);
    } catch (cause) {
      setErr(errorMessage(cause, "加载失败"));
    }
  }, [sort]);

  useEffect(() => {
    const initial = setTimeout(load, 0);
    return () => clearTimeout(initial);
  }, [load]);

  async function act(source: Source, action: "enable" | "disable" | "probe") {
    setBusy(`${source.name}:${action}`);
    setErr(null);
    try {
      if (action === "probe") await api.probeSource(source.name);
      else await api.setSourceEnabled(source.name, action === "enable");
      await load();
    } catch (cause) {
      setErr(isAuthError(cause) ? "需要登录后才能执行此操作" : errorMessage(cause, "操作失败"));
    } finally {
      setBusy("");
    }
  }

  const enabled = sources.filter((source) => source.effective_enabled).length;
  const unhealthy = sources.filter((source) => source.health_score < 60).length;
  const hq = sources.reduce((sum, source) => sum + source.contribution_hq, 0);

  return (
    <div className="flex flex-1 flex-col">
      <PageHeader
        eyebrow="Supply governance"
        title="采集源治理"
        description="在线控制、单源探测、自动冷却与 HQ 贡献排名。高风险源会在连续失败后自动退出调度。"
        actions={
          <>
            <Select value={sort} onValueChange={setSort}>
              <SelectTrigger size="sm" className="w-36" aria-label="源排序">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="priority">按优先级</SelectItem>
                <SelectItem value="health">按健康分</SelectItem>
                <SelectItem value="contribution">按 HQ 贡献</SelectItem>
                <SelectItem value="success">按成功率</SelectItem>
              </SelectContent>
            </Select>
            <Button variant="secondary" size="sm" onClick={load}>
              <RefreshCw className="size-3.5" /> 刷新
            </Button>
          </>
        }
      />

      <div className="reveal space-y-4 p-4 sm:p-6 lg:p-8">
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
          {[
            ["源总数", sources.length, "catalog"],
            ["有效源", enabled, "scheduled"],
            ["风险源", unhealthy, "health < 60"],
            ["HQ 贡献", hq, "provenance sum"],
          ].map(([label, value, hint]) => (
            <Card key={label}>
              <CardContent className="p-4">
                <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-muted-foreground">
                  {label}
                </p>
                <p className="mt-1 font-display text-2xl tabular-nums text-foreground">{value}</p>
                <p className="text-[10px] text-muted-foreground">{hint}</p>
              </CardContent>
            </Card>
          ))}
        </div>

        {err && (
          <Alert variant="danger">
            <AlertTriangle />
            <AlertDescription>{err}</AlertDescription>
          </Alert>
        )}

        <div className="grid gap-3">
          {sources.map((source, index) => {
            const cooling = Boolean(source.disabled_until && new Date(source.disabled_until) > new Date());
            const active = source.effective_enabled;
            const loading = busy.startsWith(`${source.name}:`);
            return (
              <Card
                key={source.name}
                className={cn(
                  "overflow-hidden",
                  !active && "opacity-75",
                  source.health_score < 50 && "border-l-2 border-l-destructive/70",
                )}
                style={{ animationDelay: `${Math.min(index, 10) * 25}ms` }}
              >
                <CardContent className="grid gap-4 p-4 lg:grid-cols-[minmax(0,1.5fr)_minmax(260px,.8fr)_auto] lg:items-center">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-mono text-[10px] text-muted-foreground">
                        #{String(index + 1).padStart(3, "0")}
                      </span>
                      <h2 className="truncate text-sm font-semibold text-foreground">{source.name}</h2>
                      <Badge variant={active ? "success" : "secondary"}>{active ? "ACTIVE" : "PAUSED"}</Badge>
                      <Badge variant="secondary">P{source.priority}</Badge>
                      {cooling && <Badge variant="warn">COOLDOWN</Badge>}
                      {source.manually_disabled && <Badge variant="danger">MANUAL</Badge>}
                    </div>
                    <div className="mt-2 flex items-center gap-2">
                      <p className="truncate font-mono text-[10px] text-muted-foreground">{source.url}</p>
                      <a
                        href={source.url}
                        target="_blank"
                        rel="noreferrer"
                        aria-label={`打开 ${source.name}`}
                        className="shrink-0 text-muted-foreground hover:text-primary"
                      >
                        <ExternalLink className="size-3.5" />
                      </a>
                    </div>
                    {source.last_error && (
                      <p className="mt-2 flex items-start gap-2 text-xs text-destructive">
                        <ShieldAlert className="mt-0.5 size-3.5 shrink-0" />
                        <span className="line-clamp-2">{source.last_error}</span>
                      </p>
                    )}
                  </div>

                  <div className="grid grid-cols-3 gap-2">
                    <div className="border border-border bg-muted/40 p-2.5">
                      <p className="text-[10px] text-muted-foreground">健康分</p>
                      <p
                        className={cn(
                          "mt-1 font-mono text-lg tabular-nums",
                          source.health_score >= 80
                            ? "text-success"
                            : source.health_score >= 50
                              ? "text-accent"
                              : "text-destructive",
                        )}
                      >
                        {source.health_score.toFixed(0)}
                      </p>
                    </div>
                    <div className="border border-border bg-muted/40 p-2.5">
                      <p className="text-[10px] text-muted-foreground">HQ / 总贡献</p>
                      <p className="mt-1 font-mono text-sm tabular-nums text-primary">
                        {source.contribution_hq}
                        <span className="text-muted-foreground">/{source.contribution_total}</span>
                      </p>
                    </div>
                    <div className="border border-border bg-muted/40 p-2.5">
                      <p className="text-[10px] text-muted-foreground">成功率</p>
                      <p className="mt-1 font-mono text-sm tabular-nums text-foreground">
                        {source.fetch_count ? formatPercent(source.success_rate) : "—"}
                      </p>
                    </div>
                    <p className="col-span-3 font-mono text-[10px] text-muted-foreground">
                      {formatMs(source.latency_ms)} · {formatBytes(source.bytes)} · HTTP {source.status_code || "—"} ·
                      成功 {formatTime(source.last_success_at)}
                    </p>
                  </div>

                  <div className="flex gap-2 lg:justify-end">
                    <Tooltip>
                      {/* wrapper span keeps the tooltip reachable when the
                          button is disabled (pointer-events-none) */}
                      <TooltipTrigger asChild>
                        <span tabIndex={canOperate ? -1 : 0} className="inline-flex">
                          <Button
                            size="sm"
                            variant="outline"
                            disabled={loading || !canOperate}
                            onClick={() => act(source, "probe")}
                          >
                            <Activity className={cn("size-3.5", busy === `${source.name}:probe` && "animate-pulse")} />
                            探测
                          </Button>
                        </span>
                      </TooltipTrigger>
                      {!canOperate && <TooltipContent side="top">需要 operator 权限</TooltipContent>}
                    </Tooltip>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <span tabIndex={canOperate ? -1 : 0} className="inline-flex">
                          <Button
                            size="sm"
                            variant={source.manually_disabled ? "secondary" : "destructive"}
                            disabled={loading || !canOperate}
                            onClick={() => act(source, source.manually_disabled ? "enable" : "disable")}
                          >
                            {source.manually_disabled ? <Power className="size-3.5" /> : <Ban className="size-3.5" />}
                            {source.manually_disabled ? "启用" : "停用"}
                          </Button>
                        </span>
                      </TooltipTrigger>
                      {!canOperate && <TooltipContent side="top">需要 operator 权限</TooltipContent>}
                    </Tooltip>
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>
        {sources.length === 0 && (
          <Card>
            <CardEmpty icon={Play}>暂无采集源</CardEmpty>
          </Card>
        )}
      </div>
    </div>
  );
}
