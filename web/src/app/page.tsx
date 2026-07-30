"use client";

import { useCallback, useEffect, useState } from "react";
import { JobActions } from "@/components/job-actions";
import { useLive, useLiveRefresh } from "@/components/live-provider";
import { PageHeader } from "@/components/page-header";
import { useSession } from "@/components/session-provider";
import { StatusBadge } from "@/components/status-badge";
import { TableEmpty } from "@/components/table-empty";
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
import { formatMs, formatNumber, formatTime } from "@/lib/utils";

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
      <PageHeader
        title="仪表盘"
        description={
          snapshot?.updated_at
            ? "平台运行概览 · 更新于 " + formatTime(snapshot.updated_at)
            : "正在加载平台运行概览…"
        }
      />
      {error && (
        <Alert variant="destructive">
          <AlertTitle>数据加载失败</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        <Card className="min-w-0">
          <CardHeader>
            <CardDescription>节点总数</CardDescription>
            <CardTitle className="text-2xl tabular-nums">
              {formatNumber(stats?.total_nodes, 0)}
            </CardTitle>
          </CardHeader>
        </Card>
        <Card className="min-w-0">
          <CardHeader>
            <CardDescription>存活节点</CardDescription>
            <CardTitle className="text-2xl tabular-nums">
              {formatNumber(stats?.alive_nodes, 0)}
            </CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader>
            <CardDescription>高质量节点</CardDescription>
            <CardTitle className="text-2xl tabular-nums">
              {formatNumber(stats?.high_quality, 0)}
            </CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader>
            <CardDescription>平均延迟</CardDescription>
            <CardTitle className="text-2xl tabular-nums">
              {formatMs(stats?.avg_latency_ms)}
            </CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader>
            <CardDescription>启用采集源</CardDescription>
            <CardTitle className="text-2xl tabular-nums">
              {formatNumber(stats?.sources_enabled, 0)}
            </CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader>
            <CardDescription>发布状态</CardDescription>
            <CardTitle>
              <StatusBadge status={snapshot?.health.publish_fresh ? "ready" : "pending"}>
                {snapshot ? (snapshot.health.publish_fresh ? "已更新" : "待更新") : "加载中"}
              </StatusBadge>
            </CardTitle>
          </CardHeader>
        </Card>
      </div>
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
                <TableHead className="text-right">样本</TableHead>
                <TableHead className="text-right">成功率</TableHead>
                <TableHead className="text-right">平均延迟</TableHead>
                <TableHead className="text-right">平均评分</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(snapshot?.trends ?? []).map((trend) => (
                <TableRow key={trend.day}>
                  <TableCell>{trend.day}</TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatNumber(trend.samples, 0)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {Math.round(trend.success_rate * 100) + "%"}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatMs(trend.p50_latency_ms)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatNumber(trend.avg_score)}
                  </TableCell>
                </TableRow>
              ))}
              {!snapshot?.trends.length && (
                <TableEmpty colSpan={5}>暂无趋势数据。</TableEmpty>
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
      <div className="grid gap-6 xl:grid-cols-2">
        <Card className="min-w-0">
          <CardHeader>
            <CardTitle>高质量节点</CardTitle>
            <CardDescription>按当前质量评分排序。</CardDescription>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>名称</TableHead>
                  <TableHead>协议</TableHead>
                  <TableHead>位置</TableHead>
                  <TableHead className="text-right">评分</TableHead>
                  <TableHead className="text-right">延迟</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(snapshot?.top ?? []).map((node) => (
                  <TableRow key={node.id}>
                    <TableCell className="font-medium">{node.name || node.source}</TableCell>
                    <TableCell>
                      <StatusBadge status={node.protocol}>{node.protocol}</StatusBadge>
                    </TableCell>
                    <TableCell>{node.country || "—"}</TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatNumber(node.score)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatMs(node.latency_ms)}
                    </TableCell>
                  </TableRow>
                ))}
                {!snapshot?.top.length && (
                  <TableEmpty colSpan={5}>暂无节点。</TableEmpty>
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
        <Card className="min-w-0">
          <CardHeader>
            <CardTitle>采集源</CardTitle>
            <CardDescription>当前来源健康与贡献情况。</CardDescription>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>名称</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead className="text-right">健康度</TableHead>
                  <TableHead className="text-right">贡献</TableHead>
                  <TableHead className="text-right">延迟</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(snapshot?.sources ?? []).map((source) => (
                  <TableRow key={source.name}>
                    <TableCell className="font-medium">{source.name}</TableCell>
                    <TableCell>
                      <StatusBadge status={source.effective_enabled ? "enabled" : "disabled"}>
                        {source.effective_enabled ? "启用" : "停用"}
                      </StatusBadge>
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatNumber(source.health_score)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {source.contribution_hq + " / " + source.contribution_total}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatMs(source.latency_ms)}
                    </TableCell>
                  </TableRow>
                ))}
                {!snapshot?.sources.length && (
                  <TableEmpty colSpan={5}>暂无采集源。</TableEmpty>
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </div>
      {authenticated && (
        <div className="grid gap-6 xl:grid-cols-2">
          <Card className="min-w-0">
            <CardHeader>
              <CardTitle>最近任务</CardTitle>
              <CardDescription>最近更新的任务记录。</CardDescription>
            </CardHeader>
            <CardContent>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>类型</TableHead>
                    <TableHead>状态</TableHead>
                    <TableHead className="text-right">进度</TableHead>
                    <TableHead>更新时间</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {jobs.map((job) => (
                    <TableRow key={job.id}>
                      <TableCell className="font-medium">{job.type}</TableCell>
                      <TableCell><StatusBadge status={job.status} /></TableCell>
                      <TableCell className="text-right tabular-nums">{job.progress + "%"}</TableCell>
                      <TableCell>{formatTime(job.updated_at)}</TableCell>
                    </TableRow>
                  ))}
                  {!jobs.length && (
                    <TableEmpty colSpan={4}>暂无任务。</TableEmpty>
                  )}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
          <Card className="min-w-0">
            <CardHeader>
              <CardTitle>活动告警</CardTitle>
              <CardDescription>尚未解决的异常。</CardDescription>
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
                      <TableCell><StatusBadge status={alert.severity} /></TableCell>
                      <TableCell className="font-medium">{alert.kind}</TableCell>
                      <TableCell className="max-w-80 truncate">{alert.message}</TableCell>
                      <TableCell>{formatTime(alert.created_at)}</TableCell>
                    </TableRow>
                  ))}
                  {!alerts.length && (
                    <TableEmpty colSpan={4}>暂无活动告警。</TableEmpty>
                  )}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </div>
      )}
    </>
  );
}
