"use client";

import { useCallback, useEffect, useState } from "react";
import { Bot, Info } from "lucide-react";
import {
  api,
  type AIProbeResult,
  type AITarget,
  type DashboardStats,
  type NodeItem,
} from "@/lib/api";
import { JobActions } from "@/components/job-actions";
import { PageHeader } from "@/components/page-header";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
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

      <div className="reveal space-y-6 p-4 sm:p-6 lg:p-8">
        <Card className="border-amber-500/20 bg-amber-500/5">
          <CardContent className="flex gap-3 p-4 text-sm text-amber-100/90">
            <Info className="mt-0.5 h-4 w-4 shrink-0 text-amber-300" />
            <div className="space-y-1">
              <p>
                <strong>真实代理测 AI</strong>：把节点导入 xray/sing-box 本地 SOCKS5，然后设置环境变量{" "}
                <code className="rounded bg-slate-950 px-1">NODE_HARVEST_SOCKS5=127.0.0.1:1080</code>{" "}
                或在任务 body 传 <code className="rounded bg-slate-950 px-1">socks5</code>。
              </p>
              <p className="text-amber-200/70">
                无 SOCKS5 时使用<strong>启发模式</strong>：节点质量 + 本机到 AI 边缘连通性，用于筛选候选，不保证经节点可访问。
              </p>
            </div>
          </CardContent>
        </Card>

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
          <div className="rounded-lg border border-rose-500/30 bg-rose-500/10 px-4 py-3 text-sm text-rose-300">
            {err}
          </div>
        )}

        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          {targets.map((t) => {
            const host = hostAI[t.key];
            const pass = stats?.ai_pass_rate?.[t.key];
            return (
              <Card key={t.key}>
                <CardHeader className="pb-2">
                  <CardTitle className="flex items-center gap-2 text-sm">
                    <Bot className="h-4 w-4 text-cyan-300" />
                    {t.name}
                  </CardTitle>
                  <CardDescription className="truncate font-mono text-[10px]">
                    {t.host}
                  </CardDescription>
                </CardHeader>
                <CardContent className="space-y-2">
                  <div className="flex items-center justify-between text-sm">
                    <span className="text-slate-500">本机直连</span>
                    <Badge variant={host?.ok ? "success" : "danger"}>
                      {host ? (host.ok ? "OK" : "FAIL") : "未测"}
                    </Badge>
                  </div>
                  {host && (
                    <div className="text-xs text-slate-500">
                      {formatMs(host.latency_ms)}
                      {host.status_code ? ` · HTTP ${host.status_code}` : ""}
                      {host.error ? ` · ${host.error}` : ""}
                    </div>
                  )}
                  <div className="flex items-center justify-between text-sm">
                    <span className="text-slate-500">节点通过率</span>
                    <span className="tabular-nums text-amber-200">
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
          <CardContent className="p-0">
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>节点</th>
                    <th>分数</th>
                    {targets.map((t) => (
                      <th key={t.key}>{t.key}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {nodes.slice(0, 40).map((n) => (
                    <tr key={n.id}>
                      <td className="max-w-[160px] truncate">{n.name}</td>
                      <td className="tabular-nums text-amber-200">
                        {n.score?.toFixed?.(1)}
                      </td>
                      {targets.map((t) => {
                        const r = n.ai_access?.[t.key];
                        return (
                          <td key={t.key}>
                            {r ? (
                              <Badge variant={r.ok ? "success" : "secondary"}>
                                {r.ok ? "✓" : "×"}
                              </Badge>
                            ) : (
                              <span className="text-slate-600">·</span>
                            )}
                          </td>
                        );
                      })}
                    </tr>
                  ))}
                  {nodes.length === 0 && (
                    <tr>
                      <td colSpan={2 + targets.length} className="py-10 text-center text-slate-500">
                        暂无 AI 探测数据，请运行 AI 探测任务
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
