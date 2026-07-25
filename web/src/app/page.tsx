"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { ArrowRight, RefreshCw } from "lucide-react";
import { api, type DashboardStats, type Job, type NodeItem } from "@/lib/api";
import { JobActions } from "@/components/job-actions";
import { StatCard } from "@/components/stat-card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { formatMs, formatTime, gradeColor, cn } from "@/lib/utils";

export default function DashboardPage() {
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [top, setTop] = useState<NodeItem[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [online, setOnline] = useState(false);

  const load = useCallback(async () => {
    try {
      await api.health();
      setOnline(true);
      const [st, j, n] = await Promise.all([
        api.stats(),
        api.jobs(),
        api.nodes({ limit: 8, hq: true, alive: true }),
      ]);
      setStats(st);
      setJobs(j);
      setTop(n.nodes);
      setErr(null);
    } catch (e) {
      setOnline(false);
      setErr(e instanceof Error ? e.message : "无法连接后端");
    }
  }, []);

  useEffect(() => {
    load();
    const t = setInterval(load, 4000);
    return () => clearInterval(t);
  }, [load]);

  const activeJob = jobs.find((j) => j.status === "running" || j.status === "pending");

  return (
    <div className="flex flex-1 flex-col">
      <header className="flex items-center justify-between border-b border-slate-800/80 px-8 py-5">
        <div>
          <h1 className="font-[family-name:var(--font-display)] text-xl font-semibold tracking-tight">
            仪表盘
          </h1>
          <p className="mt-1 text-sm text-slate-500">
            采集 · 智能测速 · AI 可达 · 高质量筛选
          </p>
        </div>
        <div className="flex items-center gap-3">
          <Badge variant={online ? "success" : "danger"}>
            {online ? "API 在线" : "API 离线"}
          </Badge>
          <Button variant="secondary" size="sm" onClick={load}>
            <RefreshCw className="h-3.5 w-3.5" />
            刷新
          </Button>
        </div>
      </header>

      <div className="space-y-6 p-8">
        {err && (
          <Card className="border-rose-500/30 bg-rose-500/5">
            <CardContent className="p-4 text-sm text-rose-300">
              {err}
              <div className="mt-1 text-xs text-rose-400/80">
                请先启动 Go 后端：
                <code className="ml-1 rounded bg-slate-950 px-1.5 py-0.5">
                  ./bin/node-hunter-server -addr :8080
                </code>
                {" "}
                或执行 <code className="rounded bg-slate-950 px-1.5 py-0.5">./start.sh</code>
              </div>
            </CardContent>
          </Card>
        )}

        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
          <StatCard
            label="节点总数"
            value={stats?.total_nodes ?? 0}
            hint={`启用源 ${stats?.sources_enabled ?? 0}`}
            accent="cyan"
          />
          <StatCard
            label="存活"
            value={stats?.alive_nodes ?? 0}
            hint={`均延 ${formatMs(stats?.avg_latency_ms)}`}
            accent="emerald"
          />
          <StatCard
            label="高质量 ≥70"
            value={stats?.high_quality ?? 0}
            hint="推荐导出"
            accent="amber"
          />
          <StatCard
            label="最近测速"
            value={stats?.last_quality_at ? "已完成" : "未测"}
            hint={formatTime(stats?.last_quality_at)}
            accent="rose"
          />
        </div>

        <div className="grid gap-4 lg:grid-cols-5">
          <Card className="lg:col-span-3">
            <CardHeader>
              <CardTitle>任务控制</CardTitle>
              <CardDescription>
                全流程 = 拉取订阅 → 多轮质量测速 → AI 站点启发探测 → 导出
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <JobActions onStarted={() => load()} />
              {activeJob && (
                <div className="rounded-lg border border-cyan-500/20 bg-cyan-500/5 p-4">
                  <div className="mb-2 flex items-center justify-between text-sm">
                    <span className="text-cyan-200">
                      运行中 · {activeJob.type} · {activeJob.message}
                    </span>
                    <span className="tabular-nums text-slate-400">
                      {Math.round(activeJob.progress)}%
                    </span>
                  </div>
                  <Progress value={activeJob.progress} />
                </div>
              )}
              <div className="grid gap-2 sm:grid-cols-3">
                {Object.entries(stats?.by_grade || {}).map(([g, n]) => (
                  <div
                    key={g}
                    className={cn(
                      "rounded-lg border px-3 py-2 text-center",
                      gradeColor(g)
                    )}
                  >
                    <div className="text-xs opacity-70">Grade {g}</div>
                    <div className="text-lg font-semibold tabular-nums">{n}</div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>

          <Card className="lg:col-span-2">
            <CardHeader>
              <CardTitle>协议分布</CardTitle>
              <CardDescription>当前库内协议占比</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              {Object.entries(stats?.by_protocol || {})
                .sort((a, b) => b[1] - a[1])
                .slice(0, 8)
                .map(([proto, count]) => {
                  const total = stats?.total_nodes || 1;
                  const pct = Math.round((count / total) * 100);
                  return (
                    <div key={proto}>
                      <div className="mb-1 flex justify-between text-xs">
                        <span className="font-mono text-slate-300">{proto}</span>
                        <span className="text-slate-500">
                          {count} · {pct}%
                        </span>
                      </div>
                      <div className="h-1.5 overflow-hidden rounded-full bg-slate-800">
                        <div
                          className="h-full rounded-full bg-gradient-to-r from-cyan-500 to-cyan-300/70"
                          style={{ width: `${pct}%` }}
                        />
                      </div>
                    </div>
                  );
                })}
              {!stats?.by_protocol && (
                <p className="text-sm text-slate-500">暂无数据，请先运行采集。</p>
              )}
            </CardContent>
          </Card>
        </div>

        <Card>
          <CardHeader className="flex-row items-center justify-between space-y-0">
            <div>
              <CardTitle>高质量 Top</CardTitle>
              <CardDescription>score ≥ 70 且存活</CardDescription>
            </div>
            <Button asChild variant="ghost" size="sm">
              <Link href="/nodes?hq=1">
                查看全部 <ArrowRight className="h-3.5 w-3.5" />
              </Link>
            </Button>
          </CardHeader>
          <CardContent>
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>等级</th>
                    <th>名称</th>
                    <th>协议</th>
                    <th>延迟</th>
                    <th>分数</th>
                    <th>来源</th>
                  </tr>
                </thead>
                <tbody>
                  {top.map((n) => (
                    <tr key={n.id}>
                      <td>
                        <span
                          className={cn(
                            "inline-flex h-6 w-6 items-center justify-center rounded border text-xs font-semibold",
                            gradeColor(n.grade)
                          )}
                        >
                          {n.grade || "—"}
                        </span>
                      </td>
                      <td className="max-w-[220px] truncate">{n.name}</td>
                      <td className="font-mono text-xs text-cyan-200/90">{n.protocol}</td>
                      <td className="tabular-nums">{formatMs(n.latency_ms)}</td>
                      <td className="tabular-nums text-amber-200">{n.score?.toFixed?.(1) ?? n.score}</td>
                      <td className="text-xs text-slate-500">{n.source}</td>
                    </tr>
                  ))}
                  {top.length === 0 && (
                    <tr>
                      <td colSpan={6} className="py-8 text-center text-slate-500">
                        暂无高质量节点，请运行「一键全流程」
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
