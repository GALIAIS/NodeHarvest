"use client";

import { useCallback, useEffect, useState } from "react";
import { Activity, Ban, ExternalLink, Play, Power, RefreshCw, ShieldAlert } from "lucide-react";
import { api, type Source } from "@/lib/api";
import { PageHeader } from "@/components/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { cn, formatBytes, formatMs, formatPercent, formatTime } from "@/lib/utils";

export default function SourcesPage() {
  const [sources, setSources] = useState<Source[]>([]);
  const [sort, setSort] = useState("priority");
  const [busy, setBusy] = useState("");
  const [err, setErr] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      setSources(await api.sources(sort));
      setErr(null);
    } catch (cause) {
      setErr(cause instanceof Error ? cause.message : "加载失败");
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
      setErr(cause instanceof Error ? cause.message : "操作失败");
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
            <select
              aria-label="源排序"
              className="h-8 px-3 text-xs"
              value={sort}
              onChange={(event) => setSort(event.target.value)}
            >
              <option value="priority">按优先级</option>
              <option value="health">按健康分</option>
              <option value="contribution">按 HQ 贡献</option>
              <option value="success">按成功率</option>
            </select>
            <Button variant="secondary" size="sm" onClick={load}>
              <RefreshCw className="h-3.5 w-3.5" /> 刷新
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
                <p className="font-mono text-[9px] uppercase tracking-[0.2em] text-slate-600">{label}</p>
                <p className="mt-1 font-[family-name:var(--font-display)] text-2xl text-slate-100">{value}</p>
                <p className="text-[10px] text-slate-700">{hint}</p>
              </CardContent>
            </Card>
          ))}
        </div>

        {err && (
          <div role="alert" className="rounded-lg border border-rose-500/30 bg-rose-500/10 px-4 py-3 text-sm text-rose-300">
            {err}
          </div>
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
                  source.health_score < 50 && "border-rose-500/25",
                )}
                style={{ animationDelay: `${Math.min(index, 10) * 25}ms` }}
              >
                <CardContent className="grid gap-4 p-4 lg:grid-cols-[minmax(0,1.5fr)_minmax(260px,.8fr)_auto] lg:items-center">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-mono text-[10px] text-slate-700">
                        #{String(index + 1).padStart(3, "0")}
                      </span>
                      <h2 className="truncate text-sm font-semibold text-slate-200">{source.name}</h2>
                      <Badge variant={active ? "success" : "secondary"}>{active ? "ACTIVE" : "PAUSED"}</Badge>
                      <Badge variant="secondary">P{source.priority}</Badge>
                      {cooling && <Badge variant="warn">COOLDOWN</Badge>}
                      {source.manually_disabled && <Badge variant="danger">MANUAL</Badge>}
                    </div>
                    <div className="mt-2 flex items-center gap-2">
                      <p className="truncate font-mono text-[10px] text-slate-600">{source.url}</p>
                      <a
                        href={source.url}
                        target="_blank"
                        rel="noreferrer"
                        aria-label={`打开 ${source.name}`}
                        className="shrink-0 text-slate-700 hover:text-cyan-300"
                      >
                        <ExternalLink className="h-3.5 w-3.5" />
                      </a>
                    </div>
                    {source.last_error && (
                      <p className="mt-2 flex items-start gap-2 text-xs text-rose-400">
                        <ShieldAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                        <span className="line-clamp-2">{source.last_error}</span>
                      </p>
                    )}
                  </div>

                  <div className="grid grid-cols-3 gap-2">
                    <div className="rounded-md border border-slate-800 bg-slate-950/55 p-2.5">
                      <p className="text-[10px] text-slate-600">健康分</p>
                      <p className={cn("mt-1 font-mono text-lg", source.health_score >= 80 ? "text-emerald-300" : source.health_score >= 50 ? "text-amber-300" : "text-rose-300")}>
                        {source.health_score.toFixed(0)}
                      </p>
                    </div>
                    <div className="rounded-md border border-slate-800 bg-slate-950/55 p-2.5">
                      <p className="text-[10px] text-slate-600">HQ / 总贡献</p>
                      <p className="mt-1 font-mono text-sm text-cyan-200">
                        {source.contribution_hq}<span className="text-slate-700">/{source.contribution_total}</span>
                      </p>
                    </div>
                    <div className="rounded-md border border-slate-800 bg-slate-950/55 p-2.5">
                      <p className="text-[10px] text-slate-600">成功率</p>
                      <p className="mt-1 font-mono text-sm text-slate-300">
                        {source.fetch_count ? formatPercent(source.success_rate) : "—"}
                      </p>
                    </div>
                    <p className="col-span-3 font-mono text-[10px] text-slate-600">
                      {formatMs(source.latency_ms)} · {formatBytes(source.bytes)} · HTTP {source.status_code || "—"} ·
                      成功 {formatTime(source.last_success_at)}
                    </p>
                  </div>

                  <div className="flex gap-2 lg:justify-end">
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={loading}
                      onClick={() => act(source, "probe")}
                    >
                      <Activity className={cn("h-3.5 w-3.5", busy === `${source.name}:probe` && "animate-pulse")} />
                      探测
                    </Button>
                    <Button
                      size="sm"
                      variant={source.manually_disabled ? "secondary" : "danger"}
                      disabled={loading}
                      onClick={() => act(source, source.manually_disabled ? "enable" : "disable")}
                    >
                      {source.manually_disabled ? <Power className="h-3.5 w-3.5" /> : <Ban className="h-3.5 w-3.5" />}
                      {source.manually_disabled ? "启用" : "停用"}
                    </Button>
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>
        {sources.length === 0 && (
          <Card><CardContent className="py-16 text-center text-sm text-slate-600"><Play className="mx-auto mb-3 h-5 w-5" />暂无采集源</CardContent></Card>
        )}
      </div>
    </div>
  );
}
