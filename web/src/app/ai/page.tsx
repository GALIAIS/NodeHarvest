"use client";

import { useCallback, useEffect, useState } from "react";
import { AlertTriangle, Bot, Info } from "lucide-react";
import {
  api,
  type AIProbeResult,
  type AITarget,
  type DashboardStats,
  type NodeItem,
} from "@/lib/api";
import { JobActions } from "@/components/job-actions";
import { PageHeader } from "@/components/page-header";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableEmpty,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { formatMs } from "@/lib/utils";

export default function AIPage() {
  const [targets, setTargets] = useState<AITarget[]>([]);
  const [hostAI, setHostAI] = useState<Record<string, AIProbeResult>>({});
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [nodes, setNodes] = useState<NodeItem[]>([]);
  const [err, setErr] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const [t, h, st, n] = await Promise.all([
        api.aiTargets(),
        api.hostAI(),
        api.stats(),
        api.nodes({ limit: 50, alive: true, min_score: 55 }),
      ]);
      setTargets(t);
      setHostAI(h);
      setStats(st);
      setNodes(n.nodes.filter((x) => x.ai_access && Object.keys(x.ai_access).length > 0));
      setErr(null);
    } catch (e) {
      setErr(e instanceof Error ? e.message : "加载失败");
    }
  }, []);

  useEffect(() => {
    const initial = setTimeout(load, 0);
    const t = setInterval(load, 5000);
    return () => {
      clearTimeout(initial);
      clearInterval(t);
    };
  }, [load]);

  return (
    <div className="flex flex-1 flex-col">
      <PageHeader
        eyebrow="AI reachability matrix"
        title="AI 站点可达"
        description="ChatGPT、Gemini、Claude、Grok 等目标的本机边缘状态与节点代理可达矩阵。"
      />

      <div className="reveal space-y-4 p-4 sm:p-6 lg:p-8">
        <Alert variant="warn" role="note">
          <Info />
          <AlertDescription>
            <p>
              <strong>真实代理测 AI</strong>：把节点导入 xray/sing-box 本地 SOCKS5，然后设置环境变量{" "}
              <code className="border border-border bg-popover px-1 font-mono">
                NODE_HARVEST_SOCKS5=127.0.0.1:1080
              </code>{" "}
              或在任务 body 传{" "}
              <code className="border border-border bg-popover px-1 font-mono">socks5</code>。
            </p>
            <p className="opacity-70">
              无 SOCKS5 时使用<strong>启发模式</strong>：节点质量 + 本机到 AI
              边缘连通性，用于筛选候选，不保证经节点可访问。
            </p>
          </AlertDescription>
        </Alert>

        <Card>
          <CardHeader>
            <CardTitle>运行 AI 探测</CardTitle>
            <CardDescription>建议先测速再 AI 探测，或直接一键全流程</CardDescription>
          </CardHeader>
          <CardContent>
            <JobActions onStarted={() => load()} />
          </CardContent>
        </Card>

        {err && (
          <Alert variant="danger">
            <AlertTriangle />
            <AlertDescription>{err}</AlertDescription>
          </Alert>
        )}

        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
          {targets.map((t) => {
            const host = hostAI[t.key];
            const pass = stats?.ai_pass_rate?.[t.key];
            return (
              <Card key={t.key}>
                <CardHeader className="pb-2">
                  <CardTitle className="flex items-center gap-2 text-sm">
                    <Bot className="size-4 text-primary" />
                    {t.name}
                  </CardTitle>
                  <CardDescription className="truncate font-mono text-[10px]">
                    {t.host}
                  </CardDescription>
                </CardHeader>
                <CardContent className="space-y-2">
                  <div className="flex items-center justify-between text-sm">
                    <span className="text-muted-foreground">本机直连</span>
                    <Badge variant={host?.ok ? "success" : "danger"}>
                      {host ? (host.ok ? "OK" : "FAIL") : "未测"}
                    </Badge>
                  </div>
                  {host && (
                    <div className="font-mono text-xs text-muted-foreground">
                      {formatMs(host.latency_ms)}
                      {host.status_code ? ` · HTTP ${host.status_code}` : ""}
                      {host.error ? ` · ${host.error}` : ""}
                    </div>
                  )}
                  <div className="flex items-center justify-between text-sm">
                    <span className="text-muted-foreground">节点通过率</span>
                    <span className="font-mono tabular-nums text-accent">
                      {pass != null ? `${Math.round(pass * 100)}%` : "—"}
                    </span>
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>

        <Card>
          <CardHeader>
            <CardTitle>节点 AI 矩阵（样本）</CardTitle>
            <CardDescription>已做过 AI 探测的存活节点</CardDescription>
          </CardHeader>
          <CardContent className="px-0 pb-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>节点</TableHead>
                  <TableHead>分数</TableHead>
                  {targets.map((t) => (
                    <TableHead key={t.key}>{t.key}</TableHead>
                  ))}
                </TableRow>
              </TableHeader>
              <TableBody>
                {nodes.slice(0, 40).map((n) => (
                  <TableRow key={n.id}>
                    <TableCell className="max-w-40 truncate">{n.name}</TableCell>
                    <TableCell className="font-mono tabular-nums text-accent">
                      {n.score?.toFixed?.(1)}
                    </TableCell>
                    {targets.map((t) => {
                      const r = n.ai_access?.[t.key];
                      return (
                        <TableCell key={t.key}>
                          {r ? (
                            <Badge variant={r.ok ? "success" : "secondary"}>
                              {r.ok ? "✓" : "×"}
                            </Badge>
                          ) : (
                            <span className="text-muted-foreground">·</span>
                          )}
                        </TableCell>
                      );
                    })}
                  </TableRow>
                ))}
                {nodes.length === 0 && (
                  <TableEmpty colSpan={2 + targets.length} icon={Bot}>
                    暂无 AI 探测数据，请运行 AI 探测任务
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
