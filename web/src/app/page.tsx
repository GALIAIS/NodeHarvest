"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  AlertTriangle,
  ArrowRight,
  Clock3,
  Database,
  RefreshCw,
  ServerCog,
  Waves,
} from "lucide-react";
import {
  api,
  type AlertRecord,
  type CountryRow,
  type DailyMetric,
  type DashboardStats,
  type Health,
  type Job,
  type NodeItem,
  type Source,
} from "@/lib/api";
import { JobActions } from "@/components/job-actions";
import { PageHeader } from "@/components/page-header";
import { StatCard } from "@/components/stat-card";
import { TrendChart } from "@/components/trend-chart";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { cn, formatDuration, formatMs, formatTime, gradeColor } from "@/lib/utils";

type Schedule = {
  enabled?: boolean;
  running?: boolean;
  next_run_at?: string;
  last_run_at?: string;
  job?: string;
  last_error?: string;
};

export default function DashboardPage() {
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [health, setHealth] = useState<Health | null>(null);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [top, setTop] = useState<NodeItem[]>([]);
  const [trends, setTrends] = useState<DailyMetric[]>([]);
  const [countries, setCountries] = useState<CountryRow[]>([]);
  const [sources, setSources] = useState<Source[]>([]);
  const [alerts, setAlerts] = useState<AlertRecord[]>([]);
  const [schedule, setSchedule] = useState<Schedule>({});
  const [err, setErr] = useState<string | null>(null);
  const [now, setNow] = useState(0);

  const load = useCallback(async () => {
    try {
      const [healthResult, statsResult, nodesResult, countriesResult, sourcesResult, scheduleResult] =
        await Promise.all([
          api.health(),
          api.stats(),
          api.nodes({ limit: 6, hq: true, alive: true }),
          api.countries({ hq: true, alive: true }),
          api.sources("health"),
          api.schedule(),
        ]);
      setHealth(healthResult);
      setStats(statsResult);
      setTop(nodesResult.nodes);
      setCountries(countriesResult.countries.slice(0, 8));
      setSources(sourcesResult.slice(0, 5));
      setSchedule(scheduleResult as Schedule);
      setErr(null);
    } catch (cause) {
      setErr(cause instanceof Error ? cause.message : "无法连接后端");
    }
    const [jobResult, trendResult, alertResult] = await Promise.allSettled([
      api.jobs({ limit: 20 }),
      api.trends(30),
      api.alerts(true),
    ]);
    if (jobResult.status === "fulfilled") setJobs(jobResult.value.jobs);
    if (trendResult.status === "fulfilled") setTrends(trendResult.value);
    if (alertResult.status === "fulfilled") setAlerts(alertResult.value);
  }, []);

  useEffect(() => {
    const initial = setTimeout(load, 0);
    const refresh = setInterval(load, 10000);
    const clock = setInterval(() => setNow(Date.now()), 1000);
    return () => {
      clearTimeout(initial);
      clearInterval(refresh);
      clearInterval(clock);
    };
  }, [load]);

  const activeJob = jobs.find((job) => job.status === "running" || job.status === "pending");
  const nextRunSeconds = useMemo(() => {
    if (!schedule.next_run_at || !now) return null;
    return Math.max(0, Math.floor((new Date(schedule.next_run_at).getTime() - now) / 1000));
  }, [now, schedule.next_run_at]);
  const maxCountry = Math.max(1, ...countries.map((country) => country.count));

  return (
    <div className="flex flex-1 flex-col">
      <PageHeader
        eyebrow="Fleet overview / live"
        title="网络质量作战台"
        description="从采集源、任务队列到高质量订阅池，一屏掌握当前节点供给与系统风险。"
        actions={
          <>
            <Badge variant={health?.ok ? "success" : "danger"}>
              <span className={cn("mr-1.5 h-1.5 w-1.5 rounded-full", health?.ok ? "bg-emerald-300" : "bg-rose-300")} />
              {health?.ok ? "系统在线" : "系统异常"}
            </Badge>
            <Button variant="secondary" size="sm" onClick={load}>
              <RefreshCw className="h-3.5 w-3.5" /> 刷新
            </Button>
          </>
        }
      />

      <div className="reveal space-y-5 p-4 sm:p-6 lg:p-8">
        {err && (
          <div role="alert" className="rounded-lg border border-rose-500/30 bg-rose-500/10 px-4 py-3 text-sm text-rose-300">
            {err}
          </div>
        )}

        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <StatCard label="全量节点" value={stats?.total_nodes ?? 0} hint={`启用源 ${stats?.sources_enabled ?? 0}`} accent="cyan" />
          <StatCard label="可用节点" value={stats?.alive_nodes ?? 0} hint={`均延 ${formatMs(stats?.avg_latency_ms)}`} accent="emerald" />
          <StatCard label="HQ 供给" value={stats?.high_quality ?? 0} hint={`订阅缓存 ${health?.publish_count ?? 0}`} accent="amber" />
          <StatCard
            label="活跃告警"
            value={alerts.length}
            hint={alerts.some((alert) => alert.severity === "critical") ? "存在关键异常" : "无关键异常"}
            accent="rose"
          />
        </div>

        <div className="grid gap-4 xl:grid-cols-[1.45fr_.55fr]">
          <Card>
            <CardHeader className="flex-row items-start justify-between gap-4">
              <div>
                <CardTitle className="flex items-center gap-2">
                  <Waves className="h-4 w-4 text-cyan-300" /> 30 日质量脉冲
                </CardTitle>
                <CardDescription>平均评分与 P95 延迟按日聚合，避免被单次尖峰误导</CardDescription>
              </div>
              <Badge variant="secondary">{trends.reduce((sum, row) => sum + row.samples, 0)} samples</Badge>
            </CardHeader>
            <CardContent>
              <TrendChart data={trends} />
            </CardContent>
          </Card>

          <Card className="overflow-hidden">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Clock3 className="h-4 w-4 text-amber-300" /> 调度窗口
              </CardTitle>
              <CardDescription>Asia/Shanghai · {schedule.job || "未配置"} 流程</CardDescription>
            </CardHeader>
            <CardContent className="space-y-5">
              <div>
                <p className="font-mono text-[10px] uppercase tracking-widest text-slate-600">Next execution</p>
                <p className="mt-2 font-[family-name:var(--font-display)] text-3xl font-semibold text-amber-200">
                  {schedule.enabled ? formatDuration(nextRunSeconds) : "已停用"}
                </p>
                <p className="mt-1 text-xs text-slate-600">{formatTime(schedule.next_run_at)}</p>
              </div>
              <div className="grid grid-cols-2 gap-2 text-xs">
                <div className="rounded-md border border-slate-800 bg-slate-950/60 p-3">
                  <Database className="mb-2 h-4 w-4 text-cyan-400" />
                  <p className="text-slate-600">数据库</p>
                  <p className="mt-1 text-slate-300">{health?.database.driver || "—"} · {health?.database.ok ? "OK" : "FAIL"}</p>
                </div>
                <div className="rounded-md border border-slate-800 bg-slate-950/60 p-3">
                  <ServerCog className="mb-2 h-4 w-4 text-amber-400" />
                  <p className="text-slate-600">Redis</p>
                  <p className="mt-1 text-slate-300">{health?.redis.enabled ? (health.redis.ok ? "OK" : "FAIL") : "OFF"}</p>
                </div>
              </div>
              <p className="border-t border-slate-800 pt-3 text-xs text-slate-500">
                最近运行：{formatTime(schedule.last_run_at)}
              </p>
            </CardContent>
          </Card>
        </div>

        <div className="grid gap-4 xl:grid-cols-[1fr_1fr]">
          <Card>
            <CardHeader>
              <CardTitle>任务控制</CardTitle>
              <CardDescription>手动任务优先于定时任务；重任务进入持久化队列执行</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <JobActions onStarted={load} />
              {activeJob ? (
                <div className="rounded-lg border border-cyan-500/20 bg-cyan-500/5 p-4">
                  <div className="mb-2 flex items-center justify-between gap-3 text-sm">
                    <span className="truncate text-cyan-200">{activeJob.type} · {activeJob.message}</span>
                    <span className="font-mono text-xs text-slate-400">{Math.round(activeJob.progress)}%</span>
                  </div>
                  <Progress value={activeJob.progress} />
                </div>
              ) : (
                <p className="rounded-lg border border-dashed border-slate-800 px-4 py-5 text-center text-xs text-slate-600">
                  当前没有运行中的任务
                </p>
              )}
              <div className="grid grid-cols-3 gap-2 sm:grid-cols-6">
                {["S", "A", "B", "C", "D", "F"].map((grade) => (
                  <div key={grade} className={cn("rounded-md border p-2 text-center", gradeColor(grade))}>
                    <span className="text-[10px] opacity-70">{grade}</span>
                    <p className="font-mono text-base">{stats?.by_grade?.[grade] ?? 0}</p>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex-row items-start justify-between">
              <div>
                <CardTitle>HQ 国家分布</CardTitle>
                <CardDescription>高质量池中节点最多的八个国家或地区</CardDescription>
              </div>
              <Link href="/nodes?hq=1" className="flex items-center gap-1 text-xs text-cyan-300 hover:text-cyan-200">
                查看节点 <ArrowRight className="h-3 w-3" />
              </Link>
            </CardHeader>
            <CardContent className="space-y-2.5">
              {countries.map((country) => (
                <div key={country.code} className="grid grid-cols-[2.4rem_1fr_3rem] items-center gap-3 text-xs">
                  <span className="font-mono text-slate-500">{country.flag || country.code}</span>
                  <div>
                    <div className="mb-1 flex justify-between text-slate-400">
                      <span>{country.name || country.code}</span>
                    </div>
                    <div className="h-1.5 overflow-hidden rounded-full bg-slate-800">
                      <div
                        className="h-full rounded-full bg-gradient-to-r from-cyan-500 to-cyan-300"
                        style={{ width: `${Math.max(4, (country.count / maxCountry) * 100)}%` }}
                      />
                    </div>
                  </div>
                  <span className="text-right font-mono text-slate-300">{country.count}</span>
                </div>
              ))}
              {countries.length === 0 && <p className="py-12 text-center text-sm text-slate-600">暂无国家数据</p>}
            </CardContent>
          </Card>
        </div>

        <div className="grid gap-4 xl:grid-cols-[1.1fr_.9fr]">
          <Card>
            <CardHeader className="flex-row items-start justify-between">
              <div>
                <CardTitle>源健康雷达</CardTitle>
                <CardDescription>按健康分从低到高优先处理风险源</CardDescription>
              </div>
              <Link href="/sources" className="text-xs text-cyan-300">治理全部</Link>
            </CardHeader>
            <CardContent className="space-y-2">
              {sources.map((source) => (
                <div key={source.name} className="grid grid-cols-[minmax(0,1fr)_4rem_5rem] items-center gap-3 border-b border-slate-800/60 py-2 last:border-0">
                  <div className="min-w-0">
                    <p className="truncate text-sm text-slate-300">{source.name}</p>
                    <p className="font-mono text-[10px] text-slate-600">HQ {source.contribution_hq} / {source.contribution_total}</p>
                  </div>
                  <Badge variant={source.health_score >= 80 ? "success" : source.health_score >= 50 ? "warn" : "danger"}>
                    {source.health_score.toFixed(0)}
                  </Badge>
                  <span className="text-right text-xs text-slate-600">{formatMs(source.latency_ms)}</span>
                </div>
              ))}
            </CardContent>
          </Card>

          <Card className={alerts.length ? "border-rose-500/20" : ""}>
            <CardHeader className="flex-row items-start justify-between">
              <div>
                <CardTitle className="flex items-center gap-2">
                  <AlertTriangle className="h-4 w-4 text-rose-300" /> 异常队列
                </CardTitle>
                <CardDescription>主动异常检测与值班处理入口</CardDescription>
              </div>
              <Link href="/alerts" className="text-xs text-cyan-300">打开告警台</Link>
            </CardHeader>
            <CardContent className="space-y-2">
              {alerts.slice(0, 4).map((alert) => (
                <div key={alert.id} className="rounded-md border border-slate-800 bg-slate-950/50 p-3">
                  <div className="flex items-center justify-between gap-3">
                    <p className="truncate text-sm text-slate-300">{alert.message}</p>
                    <Badge variant={alert.severity === "critical" ? "danger" : "warn"}>{alert.severity}</Badge>
                  </div>
                  <p className="mt-1 font-mono text-[10px] text-slate-600">{formatTime(alert.created_at)}</p>
                </div>
              ))}
              {alerts.length === 0 && (
                <p className="py-10 text-center text-sm text-emerald-400/70">当前没有活跃异常</p>
              )}
            </CardContent>
          </Card>
        </div>

        <Card>
          <CardHeader className="flex-row items-start justify-between">
            <div>
              <CardTitle>当前高分节点</CardTitle>
              <CardDescription>可用、HQ 且最近进入发布池的节点样本</CardDescription>
            </div>
            <Link href="/nodes?hq=1" className="flex items-center gap-1 text-xs text-cyan-300">
              全部 <ArrowRight className="h-3 w-3" />
            </Link>
          </CardHeader>
          <CardContent className="p-0">
            <div className="table-wrap">
              <table>
                <thead><tr><th>等级</th><th>节点</th><th>国家</th><th>协议</th><th>评分</th><th>延迟</th><th>来源</th></tr></thead>
                <tbody>
                  {top.map((node) => (
                    <tr key={node.id}>
                      <td><span className={cn("inline-flex h-6 w-6 items-center justify-center rounded border text-xs", gradeColor(node.grade))}>{node.grade || "—"}</span></td>
                      <td className="max-w-[240px] truncate text-slate-200">{node.name}</td>
                      <td className="font-mono text-xs text-slate-500">{node.country || "ZZ"}</td>
                      <td><Badge variant="secondary">{node.protocol}</Badge></td>
                      <td className="font-mono text-amber-200">{node.score.toFixed(1)}</td>
                      <td className="font-mono text-xs">{formatMs(node.latency_ms)}</td>
                      <td className="max-w-[180px] truncate text-xs text-slate-600">{node.source}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
