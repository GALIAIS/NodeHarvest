"use client";

import { useCallback, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { Copy, Filter, RefreshCw, Search } from "lucide-react";
import { api, type NodeItem } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { cn, formatMs, gradeColor } from "@/lib/utils";

export default function NodesPage() {
  const sp = useSearchParams();
  const [nodes, setNodes] = useState<NodeItem[]>([]);
  const [total, setTotal] = useState(0);
  const [q, setQ] = useState("");
  const [protocol, setProtocol] = useState("");
  const [grade, setGrade] = useState("");
  const [hq, setHq] = useState(sp.get("hq") === "1");
  const [alive, setAlive] = useState(true);
  const [ai, setAi] = useState("");
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [copied, setCopied] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await api.nodes({
        limit: 200,
        q: q || undefined,
        protocol: protocol || undefined,
        grade: grade || undefined,
        hq: hq || undefined,
        alive: alive || undefined,
        ai: ai || undefined,
        min_score: hq ? 70 : undefined,
      });
      setNodes(res.nodes);
      setTotal(res.total);
      setErr(null);
    } catch (e) {
      setErr(e instanceof Error ? e.message : "加载失败");
    } finally {
      setLoading(false);
    }
  }, [q, protocol, grade, hq, alive, ai]);

  useEffect(() => {
    load();
  }, [load]);

  async function copyURI(uri: string, id: string) {
    try {
      await navigator.clipboard.writeText(uri);
      setCopied(id);
      setTimeout(() => setCopied(null), 1500);
    } catch {
      /* ignore */
    }
  }

  return (
    <div className="flex flex-1 flex-col">
      <header className="flex items-center justify-between border-b border-slate-800/80 px-8 py-5">
        <div>
          <h1 className="font-[family-name:var(--font-display)] text-xl font-semibold">
            节点库
          </h1>
          <p className="mt-1 text-sm text-slate-500">
            筛选高质量节点 · 当前 {total} 条
          </p>
        </div>
        <Button variant="secondary" size="sm" onClick={load} disabled={loading}>
          <RefreshCw className={cn("h-3.5 w-3.5", loading && "animate-spin")} />
          刷新
        </Button>
      </header>

      <div className="space-y-4 p-8">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="flex items-center gap-2 text-sm">
              <Filter className="h-4 w-4 text-cyan-300" />
              过滤器
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-wrap items-center gap-3">
            <div className="relative min-w-[200px] flex-1">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500" />
              <Input
                className="pl-9"
                placeholder="搜索名称 / 服务器 / 来源"
                value={q}
                onChange={(e) => setQ(e.target.value)}
              />
            </div>
            <select
              className="h-10 rounded-md border border-slate-700 bg-slate-950 px-3 text-sm"
              value={protocol}
              onChange={(e) => setProtocol(e.target.value)}
            >
              <option value="">全部协议</option>
              {["vmess", "vless", "trojan", "ss", "ssr", "hysteria2", "tuic"].map(
                (p) => (
                  <option key={p} value={p}>
                    {p}
                  </option>
                )
              )}
            </select>
            <select
              className="h-10 rounded-md border border-slate-700 bg-slate-950 px-3 text-sm"
              value={grade}
              onChange={(e) => setGrade(e.target.value)}
            >
              <option value="">全部等级</option>
              {["S", "A", "B", "C", "D", "F"].map((g) => (
                <option key={g} value={g}>
                  {g}
                </option>
              ))}
            </select>
            <select
              className="h-10 rounded-md border border-slate-700 bg-slate-950 px-3 text-sm"
              value={ai}
              onChange={(e) => setAi(e.target.value)}
            >
              <option value="">AI 过滤</option>
              {["chatgpt", "gemini", "claude", "grok", "openai", "copilot"].map(
                (k) => (
                  <option key={k} value={k}>
                    可通过 {k}
                  </option>
                )
              )}
            </select>
            <label className="flex items-center gap-2 text-sm text-slate-400">
              <input
                type="checkbox"
                checked={hq}
                onChange={(e) => setHq(e.target.checked)}
                className="accent-amber-400"
              />
              仅高质量
            </label>
            <label className="flex items-center gap-2 text-sm text-slate-400">
              <input
                type="checkbox"
                checked={alive}
                onChange={(e) => setAlive(e.target.checked)}
                className="accent-cyan-400"
              />
              仅存活
            </label>
          </CardContent>
        </Card>

        {err && (
          <div className="rounded-lg border border-rose-500/30 bg-rose-500/10 px-4 py-3 text-sm text-rose-300">
            {err}
          </div>
        )}

        <Card>
          <CardContent className="p-0">
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>等级</th>
                    <th>分数</th>
                    <th>名称</th>
                    <th>协议</th>
                    <th>地址</th>
                    <th>延迟</th>
                    <th>成功率</th>
                    <th>抖动</th>
                    <th>AI</th>
                    <th>来源</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {nodes.map((n) => {
                    const aiOk = Object.entries(n.ai_access || {})
                      .filter(([, r]) => r?.ok)
                      .map(([k]) => k);
                    return (
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
                        <td className="tabular-nums text-amber-200">
                          {typeof n.score === "number" ? n.score.toFixed(1) : "—"}
                        </td>
                        <td className="max-w-[180px] truncate" title={n.name}>
                          {n.name}
                        </td>
                        <td>
                          <Badge variant="secondary" className="font-mono">
                            {n.protocol}
                          </Badge>
                        </td>
                        <td className="font-mono text-xs text-slate-400">
                          {n.server}:{n.port}
                        </td>
                        <td className="tabular-nums">{formatMs(n.latency_ms)}</td>
                        <td className="tabular-nums text-xs">
                          {n.quality
                            ? `${Math.round(n.quality.success_rate * 100)}%`
                            : "—"}
                        </td>
                        <td className="tabular-nums text-xs">
                          {n.quality ? formatMs(n.quality.jitter_ms) : "—"}
                        </td>
                        <td>
                          <div className="flex max-w-[140px] flex-wrap gap-1">
                            {aiOk.length === 0 && (
                              <span className="text-xs text-slate-600">—</span>
                            )}
                            {aiOk.slice(0, 3).map((k) => (
                              <Badge key={k} variant="success" className="text-[10px]">
                                {k}
                              </Badge>
                            ))}
                          </div>
                        </td>
                        <td className="max-w-[100px] truncate text-xs text-slate-500">
                          {n.source}
                        </td>
                        <td>
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => copyURI(n.raw_uri, n.id)}
                            title="复制 URI"
                          >
                            <Copy className="h-3.5 w-3.5" />
                            {copied === n.id ? "OK" : ""}
                          </Button>
                        </td>
                      </tr>
                    );
                  })}
                  {nodes.length === 0 && (
                    <tr>
                      <td colSpan={11} className="py-12 text-center text-slate-500">
                        无匹配节点
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
