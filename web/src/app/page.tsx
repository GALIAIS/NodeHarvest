"use client";

import { useCallback, useEffect, useState } from "react";
import { JobActions } from "@/components/job-actions";
import { useLive, useLiveRefresh } from "@/components/live-provider";
import { useSession } from "@/components/session-provider";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
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
  type AlertRecord,
  type DashboardSnapshot,
  type Job,
} from "@/lib/api";
import { formatMs, formatTime } from "@/lib/utils";

export default function DashboardPage() {
  const { authenticated, canOperate } = useSession();
  const { dashboard: liveDashboard } = useLive();
  const [dashboard, setDashboard] = useState<DashboardSnapshot | null>(null);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [alerts, setAlerts] = useState<AlertRecord[]>([]);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    try {
      setDashboard(await api.dashboard());
      setError("");
    } catch (cause) {
      setError(errorMessage(cause, "加载仪表盘失败"));
    }

    if (!authenticated) {
      setJobs([]);
      setAlerts([]);
      return;
    }

    const privateResults = await Promise.allSettled([api.jobs({ limit: 8 }), api.alerts(true)]);
    if (privateResults[0].status === "fulfilled") setJobs(privateResults[0].value.jobs);
    if (privateResults[1].status === "fulfilled") setAlerts(privateResults[1].value);
  }, [authenticated]);

  useEffect(() => {
    const initial = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(initial);
  }, [load]);

  useLiveRefresh(load, authenticated);

  const snapshot = liveDashboard ?? dashboard;
  const stats = snapshot?.stats;

  return (
    <>
      <header className="space-y-1">
        <h1 className="text-3xl font-bold tracking-tight">仪表盘</h1>
        <p className="text-muted-foreground">节点、采集源与任务的当前状态。</p>
      </header>
      {error && (
        <Alert variant="destructive">
          <AlertTitle>数据加载失败</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <Card>
        <CardHeader>
          <CardTitle>概览</CardTitle>
          <CardDescription>{snapshot?.updated_at ? "更新时间 " + formatTime(snapshot.updated_at) : "正在加载…"}</CardDescription>
        </CardHeader>
        <CardContent>
          <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <div className="space-y-1">
              <dt className="text-sm text-muted-foreground">节点总数</dt>
              <dd className="text-2xl font-bold">{stats?.total_nodes ?? "—"}</dd>
            </div>
            <div className="space-y-1">
              <dt className="text-sm text-muted-foreground">存活节点</dt>
              <dd className="text-2xl font-bold">{stats?.alive_nodes ?? "—"}</dd>
            </div>
            <div className="space-y-1">
              <dt className="text-sm text-muted-foreground">高质量节点</dt>
              <dd className="text-2xl font-bold">{stats?.high_quality ?? "—"}</dd>
            </div>
            <div className="space-y-1">
              <dt className="text-sm text-muted-foreground">平均延迟</dt>
              <dd className="text-2xl font-bold">{formatMs(stats?.avg_latency_ms)}</dd>
            </div>
            <div className="space-y-1">
              <dt className="text-sm text-muted-foreground">启用采集源</dt>
              <dd className="text-2xl font-bold">{stats?.sources_enabled ?? "—"}</dd>
            </div>
            <div className="space-y-1">
              <dt className="text-sm text-muted-foreground">发布状态</dt>
              <dd className="text-2xl font-bold">{snapshot?.health.publish_fresh ? "最新" : "待更新"}</dd>
            </div>
          </dl>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>质量趋势</CardTitle>
          <CardDescription>最近 30 天的质量采样。</CardDescription>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>日期</TableHead>
                <TableHead>样本</TableHead>
                <TableHead>成功率</TableHead>
                <TableHead>平均延迟</TableHead>
                <TableHead>平均评分</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(snapshot?.trends ?? []).map((trend) => (
                <TableRow key={trend.day}>
                  <TableCell>{trend.day}</TableCell>
                  <TableCell>{trend.samples}</TableCell>
                  <TableCell>{Math.round(trend.success_rate * 100) + "%"}</TableCell>
                  <TableCell>{formatMs(trend.p50_latency_ms)}</TableCell>
                  <TableCell>{trend.avg_score}</TableCell>
                </TableRow>
              ))}
              {!snapshot?.trends.length && (
                <TableRow>
                  <TableCell colSpan={5}>暂无趋势数据。</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      {canOperate && (
        <Card>
          <CardHeader>
            <CardTitle>任务操作</CardTitle>
            <CardDescription>启动采集、质量测试或 AI 探测。</CardDescription>
          </CardHeader>
          <CardContent>
            <JobActions onStarted={() => void load()} />
          </CardContent>
        </Card>
      )}
      <Card>
        <CardHeader>
          <CardTitle>高质量节点</CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>名称</TableHead>
                <TableHead>协议</TableHead>
                <TableHead>位置</TableHead>
                <TableHead>评分</TableHead>
                <TableHead>延迟</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(snapshot?.top ?? []).map((node) => (
                <TableRow key={node.id}>
                  <TableCell>{node.name || node.source}</TableCell>
                  <TableCell>{node.protocol}</TableCell>
                  <TableCell>{node.country || "—"}</TableCell>
                  <TableCell>{node.score}</TableCell>
                  <TableCell>{formatMs(node.latency_ms)}</TableCell>
                </TableRow>
              ))}
              {!snapshot?.top.length && (
                <TableRow>
                  <TableCell colSpan={5}>暂无节点。</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>采集源</CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>名称</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>健康度</TableHead>
                <TableHead>贡献</TableHead>
                <TableHead>延迟</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(snapshot?.sources ?? []).map((source) => (
                <TableRow key={source.name}>
                  <TableCell>{source.name}</TableCell>
                  <TableCell>{source.effective_enabled ? "启用" : "停用"}</TableCell>
                  <TableCell>{source.health_score}</TableCell>
                  <TableCell>{source.contribution_hq + " / " + source.contribution_total}</TableCell>
                  <TableCell>{formatMs(source.latency_ms)}</TableCell>
                </TableRow>
              ))}
              {!snapshot?.sources.length && (
                <TableRow>
                  <TableCell colSpan={5}>暂无采集源。</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      {authenticated && (
        <>
          <Card>
            <CardHeader>
              <CardTitle>最近任务</CardTitle>
            </CardHeader>
            <CardContent>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>类型</TableHead>
                    <TableHead>状态</TableHead>
                    <TableHead>进度</TableHead>
                    <TableHead>更新时间</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {jobs.map((job) => (
                    <TableRow key={job.id}>
                      <TableCell>{job.type}</TableCell>
                      <TableCell>{job.status}</TableCell>
                      <TableCell>{job.progress + "%"}</TableCell>
                      <TableCell>{formatTime(job.updated_at)}</TableCell>
                    </TableRow>
                  ))}
                  {!jobs.length && (
                    <TableRow>
                      <TableCell colSpan={4}>暂无任务。</TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>活动告警</CardTitle>
            </CardHeader>
            <CardContent>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>严重程度</TableHead>
                    <TableHead>类型</TableHead>
                    <TableHead>内容</TableHead>
                    <TableHead>时间</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {alerts.map((alert) => (
                    <TableRow key={alert.id}>
                      <TableCell>{alert.severity}</TableCell>
                      <TableCell>{alert.kind}</TableCell>
                      <TableCell>{alert.message}</TableCell>
                      <TableCell>{formatTime(alert.created_at)}</TableCell>
                    </TableRow>
                  ))}
                  {!alerts.length && (
                    <TableRow>
                      <TableCell colSpan={4}>暂无活动告警。</TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </>
      )}
    </>
  );
}
