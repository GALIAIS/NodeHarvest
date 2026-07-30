"use client";

import { useCallback, useEffect, useState } from "react";
import { AuthRequired } from "@/components/auth-required";
import { JobActions } from "@/components/job-actions";
import { useLiveRefresh } from "@/components/live-provider";
import { PageHeader } from "@/components/page-header";
import { useSession } from "@/components/session-provider";
import { StatusBadge } from "@/components/status-badge";
import { TableEmpty } from "@/components/table-empty";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
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
  type AIProbeResult,
  type AITarget,
  type DashboardStats,
  type NodeItem,
} from "@/lib/api";
import { formatMs, formatNumber, formatPercent } from "@/lib/utils";

export default function AIPage() {
  const { authenticated, canOperate } = useSession();
  const [targets, setTargets] = useState<AITarget[]>([]);
  const [hostAI, setHostAI] = useState<Record<string, AIProbeResult>>({});
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [nodes, setNodes] = useState<NodeItem[]>([]);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    try {
      const [nextTargets, nextHostAI, nextStats, page] = await Promise.all([
        api.aiTargets(),
        api.hostAI(),
        api.stats(),
        api.nodes({ limit: 100, alive: true, min_score: 55 }),
      ]);
      setTargets(nextTargets);
      setHostAI(nextHostAI);
      setStats(nextStats);
      setNodes(page.nodes.filter((node) => Object.keys(node.ai_access ?? {}).length > 0));
      setError("");
    } catch (cause) {
      setError(errorMessage(cause, "加载 AI 可达性失败"));
    }
  }, []);

  useEffect(() => {
    const initial = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(initial);
  }, [load]);

  useLiveRefresh(load, authenticated);

  return (
    <>
      <PageHeader
        title="AI 可达"
        description="查看目标站点的本机状态与节点级探测结果。"
      />
      {error && (
        <Alert variant="destructive">
          <AlertTitle>加载失败</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <Card>
        <CardHeader>
          <CardTitle>运行探测</CardTitle>
          <CardDescription>需要 operator 或 admin 权限。</CardDescription>
        </CardHeader>
        <CardContent>
          {authenticated && canOperate ? (
            <JobActions onStarted={() => void load()} />
          ) : (
            <AuthRequired
              reason={authenticated ? "forbidden" : "anonymous"}
              title={authenticated ? "当前角色无法运行任务" : "登录后可运行探测"}
            />
          )}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>目标状态</CardTitle>
          <CardDescription>本机探测状态与节点总体通过率。</CardDescription>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>目标</TableHead>
                <TableHead>主机</TableHead>
                <TableHead>本机状态</TableHead>
                <TableHead className="text-right">本机延迟</TableHead>
                <TableHead className="text-right">节点通过率</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {targets.map((target) => {
                const result = hostAI[target.key];
                return (
                  <TableRow key={target.key}>
                    <TableCell className="font-medium">{target.name}</TableCell>
                    <TableCell className="font-mono text-xs">{target.host}</TableCell>
                    <TableCell>
                      <Badge variant={result?.ok ? "default" : "secondary"}>
                        {result ? (result.ok ? "可达" : "不可达") : "未测试"}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatMs(result?.latency_ms)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatPercent(stats?.ai_pass_rate[target.key])}
                    </TableCell>
                  </TableRow>
                );
              })}
              {!targets.length && (
                <TableEmpty colSpan={5}>暂无 AI 目标。</TableEmpty>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>节点探测结果</CardTitle>
          <CardDescription>按节点查看每个 AI 目标的可达性。</CardDescription>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>节点</TableHead>
                <TableHead className="text-right">评分</TableHead>
                {targets.map((target) => (
                  <TableHead key={target.key}>{target.key}</TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {nodes.map((node) => (
                <TableRow key={node.id}>
                  <TableCell className="font-medium">{node.name || node.server}</TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatNumber(node.score)}
                  </TableCell>
                  {targets.map((target) => {
                    const result = node.ai_access?.[target.key];
                    return (
                      <TableCell key={target.key}>
                        {result ? (
                          <StatusBadge status={result.ok ? "passed" : "failed"}>
                            {result.ok ? "通过" : "失败"}
                          </StatusBadge>
                        ) : "—"}
                      </TableCell>
                    );
                  })}
                </TableRow>
              ))}
              {!nodes.length && (
                <TableEmpty colSpan={targets.length + 2}>暂无 AI 探测结果。</TableEmpty>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </>
  );
}
