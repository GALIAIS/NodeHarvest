"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  AlertTriangle,
  ArrowRight,
  Clock3,
  Database,
  Gauge,
  Network,
  RefreshCw,
  ServerCog,
  ShieldCheck,
  Waves,
} from "lucide-react";
import {
  api,
  errorMessage,
  isAuthError,
  type AlertRecord,
  type DashboardSnapshot,
  type Job,
} from "@/lib/api";
import { AuthRequired } from "@/components/auth-required";
import { JobActions } from "@/components/job-actions";
import { PageHeader } from "@/components/page-header";
import { useSession } from "@/components/session-provider";
import { useLive, useLiveRefresh } from "@/components/live-provider";
import { StatCard } from "@/components/stat-card";
import { TrendChart } from "@/components/trend-chart";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardEmpty,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { Separator } from "@/components/ui/separator";
import {
  Table,
  TableBody,
  TableCell,
  TableEmpty,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  cn,
  formatDuration,
  formatMs,
  formatTime,
  gradeColor,
  healthVariant,
} from "@/lib/utils";

type Schedule = {
  enabled?: boolean;
  running?: boolean;
  next_run_at?: string;
  last_run_at?: string;
  job?: string;
  last_error?: string;
};

const GRADES = ["S", "A", "B", "C", "D", "F"] as const;

export default function DashboardPage() {
  const { authenticated, canOperate, loading: sessionLoading } = useSession();
  const { dashboard: liveDashboard, connected } = useLive();
  const [fallbackSnapshot, setSnapshot] = useState<DashboardSnapshot | null>(null);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [alerts, setAlerts] = useState<AlertRecord[]>([]);
  // 告警接口属于管理面：只有本轮拉取真正成功时才把 0 条视为“无异常”
  const [alertsVisible, setAlertsVisible] = useState(false);
  const [schedule, setSchedule] = useState<Schedule>({});
  const [err, setErr] = useState<string | null>(null);
  const [now, setNow] = useState(0);

  const loadDashboard = useCallback(async () => {
    try {
      setSnapshot(await api.dashboard());
      setErr(null);
    } catch (cause) {
      setErr(errorMessage(cause, "无法连接后端"));
    }
  }, []);

  const loadManagement = useCallback(async () => {
    if (!authenticated) return;
    const [jobResult, alertResult, scheduleResult] = await Promise.allSettled([
      api.jobs({ limit: 20 }),
      api.alerts(true),
      api.schedule(),
    ]);
    if (jobResult.status === "fulfilled") setJobs(jobResult.value?.jobs ?? []);
    if (scheduleResult.status === "fulfilled") setSchedule(scheduleResult.value as Schedule);
    // 判定依据是"这次请求是否被允许"，而不是返回值真假：无告警时后端
    // 返回 JSON null，若按真值判断会把"已登录且零告警"误判成"未登录"。
    if (!authenticated) {
      setAlerts([]);
      setAlertsVisible(false);
    } else if (alertResult.status === "fulfilled") {
      setAlerts(alertResult.value ?? []);
      setAlertsVisible(true);
    } else if (isAuthError(alertResult.reason)) {
      // 会话中途失效导致的 401/403 才收起管理面板块
      setAlerts([]);
      setAlertsVisible(false);
    } else {
      // 后端故障或网络抖动：保留上一轮告警与可见状态，仅提示错误，
      // 避免把服务端异常误判成"未登录"。
      setErr(errorMessage(alertResult.reason, "告警数据拉取失败"));
    }
  }, [authenticated]);

  useEffect(() => {
    const initial = setTimeout(loadDashboard, 0);
    return () => clearTimeout(initial);
  }, [loadDashboard]);

  useEffect(() => {
    const initial = setTimeout(() => {
      if (!authenticated) {
        setJobs([]);
        setAlerts([]);
        setAlertsVisible(false);
        setSchedule({});
        return;
      }
      void loadManagement();
    }, 0);
    return () => clearTimeout(initial);
  }, [authenticated, loadManagement]);

  useLiveRefresh(loadManagement, authenticated);

  useEffect(() => {
    const clock = setInterval(() => setNow(Date.now()), 1000);
    return () => {
      clearInterval(clock);
    };
  }, []);

  const activeJob = jobs.find((job) => job.status === "running" || job.status === "pending");
  const snapshot = liveDashboard ?? fallbackSnapshot;
  const nextRunSeconds = useMemo(() => {
    if (!schedule.next_run_at || !now) return null;
    return Math.max(0, Math.floor((new Date(schedule.next_run_at).getTime() - now) / 1000));
  }, [now, schedule.next_run_at]);
  const countries = snapshot?.countries ?? [];
  const sources = snapshot?.sources ?? [];
  const top = snapshot?.top ?? [];
  const maxCountry = Math.max(1, ...countries.map((country) => country.count));

  return (
    <div className="flex flex-1 flex-col">
      <PageHeader
        eyebrow="Fleet overview / live"
        title="网络质量作战台"
        description="从采集源、任务队列到高质量订阅池，一屏掌握当前节点供给与系统风险。"
        actions={
          <>
            <Badge variant={snapshot?.health.ok ? "success" : "danger"} className="h-8 px-2.5">
              <span className={cn("size-1.5", snapshot?.health.ok ? "bg-success" : "bg-destructive")} />
              {snapshot?.health.ok ? "系统在线" : "系统异常"}
            </Badge>
            {authenticated && (
              <Button variant="secondary" size="sm" onClick={() => { void loadDashboard(); void loadManagement(); }}>
                <RefreshCw className="size-3.5" /> 刷新
              </Button>
            )}
          </>
        }
      />

      <div className="reveal space-y-4 p-4 sm:p-6 lg:p-8">
        {err && (
          <Alert variant="danger">
            <AlertTriangle />
            <AlertDescription>{err}</AlertDescription>
          </Alert>
        )}

        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <StatCard
            label="全量节点"
            value={snapshot?.stats.total_nodes ?? 0}
            hint={`启用源 ${snapshot?.stats.sources_enabled ?? 0}`}
            accent="primary"
            icon={Network}
          />
          <StatCard
            label="可用节点"
            value={snapshot?.stats.alive_nodes ?? 0}
            hint={`均延 ${formatMs(snapshot?.stats.avg_latency_ms)}`}
            accent="success"
            icon={Gauge}
          />
          <StatCard
            label="HQ 供给"
            value={snapshot?.stats.high_quality ?? 0}
            hint={`订阅缓存 ${snapshot?.health.publish_count ?? 0}`}
            accent="accent"
            icon={ShieldCheck}
          />
          <StatCard
            label="活跃告警"
            value={alertsVisible ? alerts.length : "—"}
            hint={
              alertsVisible
                ? alerts.some((alert) => alert.severity === "critical")
                  ? "存在关键异常"
                  : "无关键异常"
                : sessionLoading
                  ? "—"
                  : "登录后查看"
            }
            accent="destructive"
            icon={AlertTriangle}
          />
        </div>

        <div className="grid gap-3 xl:grid-cols-[1.45fr_.55fr]">
          <Card>
            <CardHeader className="flex-row items-start justify-between gap-4">
              <div>
                <CardTitle className="flex items-center gap-2">
                  <Waves className="size-4 text-primary" /> 30 日质量脉冲
                </CardTitle>
                <CardDescription>平均评分与 P95 延迟按日聚合，避免被单次尖峰误导</CardDescription>
              </div>
              <CardAction>
                <Badge variant="secondary">
                  {(snapshot?.trends ?? []).reduce((sum, row) => sum + row.samples, 0)} samples
                </Badge>
              </CardAction>
            </CardHeader>
            <CardContent>
              <TrendChart data={snapshot?.trends ?? []} />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Clock3 className="size-4 text-accent" /> 调度窗口
              </CardTitle>
              <CardDescription>Asia/Shanghai · {schedule.job || "未配置"} 流程</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="corner-ticks relative border border-border bg-muted/40 p-4">
                <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-muted-foreground">
                  Next execution
                </p>
                <p className="mt-2 font-display text-3xl leading-none font-semibold tabular-nums text-accent">
                  {schedule.enabled ? formatDuration(nextRunSeconds) : "已停用"}
                </p>
                <p className="mt-2 font-mono text-[10px] text-muted-foreground">
                  {formatTime(schedule.next_run_at)}
                </p>
              </div>
              <div className="grid grid-cols-2 gap-2">
                <div className="border border-border bg-muted/40 p-3">
                  <Database className="mb-2 size-4 text-primary" />
                  <p className="text-[10px] uppercase tracking-wider text-muted-foreground">仪表盘快照</p>
                  <p className="mt-1 text-xs text-foreground">
                    {snapshot?.health.nodes ?? 0} 节点 · {snapshot?.health.running_job ? "任务运行中" : "无运行任务"}
                  </p>
                </div>
                <div className="border border-border bg-muted/40 p-3">
                  <ServerCog className="mb-2 size-4 text-accent" />
                  <p className="text-[10px] uppercase tracking-wider text-muted-foreground">实时通道</p>
                  <p className="mt-1 text-xs text-foreground">
                    {connected ? "SSE 已连接" : "正在重连"}
                  </p>
                </div>
              </div>
              <Separator />
              <p className="font-mono text-[10px] text-muted-foreground">
                最近运行 {formatTime(schedule.last_run_at)}
              </p>
            </CardContent>
          </Card>
        </div>

        <div className="grid gap-3 xl:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle>任务控制</CardTitle>
              <CardDescription>手动任务优先于定时任务；重任务进入持久化队列执行</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {sessionLoading || canOperate ? (
                <JobActions onStarted={() => { void loadManagement(); }} />
              ) : authenticated ? (
                <AuthRequired
                  compact
                  reason="forbidden"
                  title="当前角色无法运行任务"
                  description="采集、测速与 AI 探测任务需要 operator 及以上权限。"
                />
              ) : (
                <AuthRequired
                  compact
                  title="需要登录后运行任务"
                  description="采集、测速与 AI 探测任务需要 operator 及以上权限。"
                />
              )}
              {activeJob ? (
                <div className="border border-primary/30 bg-primary/5 p-4">
                  <div className="mb-2 flex items-center justify-between gap-3 text-sm">
                    <span className="truncate text-primary">
                      {activeJob.type} · {activeJob.message}
                    </span>
                    <span className="font-mono text-xs tabular-nums text-muted-foreground">
                      {Math.round(activeJob.progress)}%
                    </span>
                  </div>
                  <Progress value={activeJob.progress} />
                </div>
              ) : (
                <p className="border border-dashed border-border px-4 py-5 text-center text-xs text-muted-foreground">
                  当前没有运行中的任务
                </p>
              )}
              <div className="grid grid-cols-3 gap-2 sm:grid-cols-6">
                {GRADES.map((grade) => (
                  <div key={grade} className={cn("border p-2 text-center", gradeColor(grade))}>
                    <span className="font-mono text-[10px] uppercase tracking-wider opacity-70">
                      {grade}
                    </span>
                    <p className="font-mono text-base tabular-nums">
                      {snapshot?.stats.by_grade?.[grade] ?? 0}
                    </p>
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
              {authenticated && (
                <CardAction>
                  <Button variant="ghost" size="sm" asChild>
                    <Link href="/nodes?hq=1">
                      查看节点 <ArrowRight className="size-3" />
                    </Link>
                  </Button>
                </CardAction>
              )}
            </CardHeader>
            <CardContent className="space-y-2.5">
              {countries.map((country) => (
                <div
                  key={country.code}
                  className="grid grid-cols-[2.2rem_1fr_3rem] items-center gap-3 text-xs"
                >
                  <span className="font-mono text-muted-foreground">
                    {country.flag || country.code}
                  </span>
                  <div>
                    <p className="mb-1 text-muted-foreground">{country.name || country.code}</p>
                    <div className="h-1.5 border border-border bg-muted">
                      <div
                        className="h-full bg-primary"
                        style={{ width: `${Math.max(3, (country.count / maxCountry) * 100)}%` }}
                      />
                    </div>
                  </div>
                  <span className="text-right font-mono tabular-nums text-foreground">
                    {country.count}
                  </span>
                </div>
              ))}
              {countries.length === 0 && <CardEmpty>暂无国家数据</CardEmpty>}
            </CardContent>
          </Card>
        </div>

        <div className="grid gap-3 xl:grid-cols-[1.1fr_.9fr]">
          <Card>
            <CardHeader className="flex-row items-start justify-between">
              <div>
                <CardTitle>源健康雷达</CardTitle>
                <CardDescription>按健康分从低到高优先处理风险源</CardDescription>
              </div>
              {authenticated && (
                <CardAction>
                  <Button variant="ghost" size="sm" asChild>
                    <Link href="/sources">治理全部</Link>
                  </Button>
                </CardAction>
              )}
            </CardHeader>
            <CardContent className="space-y-0">
              {sources.map((source) => (
                <div
                  key={source.name}
                  className="grid grid-cols-[minmax(0,1fr)_3.5rem_5rem] items-center gap-3 border-b border-border/60 py-2.5 last:border-0"
                >
                  <div className="min-w-0">
                    <p className="truncate text-sm text-foreground">{source.name}</p>
                    <p className="font-mono text-[10px] text-muted-foreground">
                      HQ {source.contribution_hq} / {source.contribution_total}
                    </p>
                  </div>
                  <Badge variant={healthVariant(source.health_score)}>
                    {source.health_score.toFixed(0)}
                  </Badge>
                  <span className="text-right font-mono text-xs tabular-nums text-muted-foreground">
                    {formatMs(source.latency_ms)}
                  </span>
                </div>
              ))}
              {sources.length === 0 && <CardEmpty>暂无采集源</CardEmpty>}
            </CardContent>
          </Card>

          <Card className={alerts.length ? "border-destructive/30" : undefined}>
            <CardHeader className="flex-row items-start justify-between">
              <div>
                <CardTitle className="flex items-center gap-2">
                  <AlertTriangle className="size-4 text-destructive" /> 异常队列
                </CardTitle>
                <CardDescription>主动异常检测与值班处理入口</CardDescription>
              </div>
              {authenticated && (
                <CardAction>
                  <Button variant="ghost" size="sm" asChild>
                    <Link href="/alerts">打开告警台</Link>
                  </Button>
                </CardAction>
              )}
            </CardHeader>
            <CardContent className="space-y-2">
              {alerts.slice(0, 4).map((alert) => (
                <div
                  key={alert.id}
                  className="border border-border border-l-2 border-l-destructive/70 bg-muted/40 p-3"
                >
                  <div className="flex items-center justify-between gap-3">
                    <p className="truncate text-sm text-foreground">{alert.message}</p>
                    <Badge variant={alert.severity === "critical" ? "danger" : "warn"}>
                      {alert.severity}
                    </Badge>
                  </div>
                  <p className="mt-1 font-mono text-[10px] text-muted-foreground">
                    {formatTime(alert.created_at)}
                  </p>
                </div>
              ))}
              {alertsVisible && alerts.length === 0 && (
                <CardEmpty className="text-success/80">当前没有活跃异常</CardEmpty>
              )}
              {!alertsVisible && !sessionLoading && (
                <AuthRequired compact title="需要登录" description="活跃告警与值班处理属于管理面。" />
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
            {authenticated && (
              <CardAction>
                <Button variant="ghost" size="sm" asChild>
                  <Link href="/nodes?hq=1">
                    全部 <ArrowRight className="size-3" />
                  </Link>
                </Button>
              </CardAction>
            )}
          </CardHeader>
          <CardContent className="px-0 pb-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>等级</TableHead>
                  <TableHead>节点</TableHead>
                  <TableHead>国家</TableHead>
                  <TableHead>协议</TableHead>
                  <TableHead>评分</TableHead>
                  <TableHead>延迟</TableHead>
                  <TableHead>来源</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {top.map((node) => (
                  <TableRow key={node.id}>
                    <TableCell>
                      <span
                        className={cn(
                          "inline-flex size-6 items-center justify-center border font-mono text-xs font-semibold",
                          gradeColor(node.grade),
                        )}
                      >
                        {node.grade || "—"}
                      </span>
                    </TableCell>
                    <TableCell className="max-w-60 truncate text-foreground">{node.name}</TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">
                      {node.country || "ZZ"}
                    </TableCell>
                    <TableCell>
                      <Badge variant="secondary">{node.protocol}</Badge>
                    </TableCell>
                    <TableCell className="font-mono tabular-nums text-accent">
                      {node.score.toFixed(1)}
                    </TableCell>
                    <TableCell className="font-mono text-xs tabular-nums">
                      {formatMs(node.latency_ms)}
                    </TableCell>
                    <TableCell className="max-w-44 truncate text-xs text-muted-foreground">
                      {node.source}
                    </TableCell>
                  </TableRow>
                ))}
                {top.length === 0 && (
                  <TableEmpty colSpan={7} icon={Network}>
                    暂无高质量节点，先运行一次全流程任务
                  </TableEmpty>
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
