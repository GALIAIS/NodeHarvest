"use client";

import { useCallback, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { Copy, Filter, Globe2, RefreshCw, Search } from "lucide-react";
import { api, type CountryRow, type NodeItem } from "@/lib/api";
import { PageHeader } from "@/components/page-header";
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
  const [country, setCountry] = useState("");
  const [minScore, setMinScore] = useState(sp.get("hq") === "1" ? "70" : "");
  const [hq, setHq] = useState(sp.get("hq") === "1");
  const [alive, setAlive] = useState(true);
  const [verified, setVerified] = useState(false);
  const [ai, setAi] = useState("");
  const [countries, setCountries] = useState<CountryRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [copied, setCopied] = useState<string | null>(null);
  const [nextCursor, setNextCursor] = useState("");

  const load = useCallback(async (cursor = "", append = false) => {
    setLoading(true);
    try {
      const res = await api.nodes({
        limit: 200,
        q: q || undefined,
        protocol: protocol || undefined,
        grade: grade || undefined,
        country: country || undefined,
        hq: hq || undefined,
        alive: alive || undefined,
        verified: verified || undefined,
        ai: ai || undefined,
        min_score: minScore || (hq ? 70 : undefined),
        cursor: cursor || undefined,
      });
      setNodes((current) => (append ? [...current, ...res.nodes] : res.nodes));
      setTotal(res.total);
      setNextCursor(res.next_cursor || "");
      setErr(null);
    } catch (e) {
      setErr(e instanceof Error ? e.message : "加载失败");
    } finally {
      setLoading(false);
    }
  }, [q, protocol, grade, country, minScore, hq, alive, verified, ai]);

  useEffect(() => {
    const initial = setTimeout(load, 0);
    return () => clearTimeout(initial);
  }, [load]);

  useEffect(() => {
    api.countries({ alive: true }).then((result) => setCountries(result.countries)).catch(() => {});
  }, []);

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
      <PageHeader
        eyebrow="Node inventory"
        title="节点资产库"
        description={`按国家、协议、评分、真实拨测与 AI 可达性筛选 · 当前 ${total} 条`}
        actions={
          <Button variant="secondary" size="sm" onClick={() => load()} disabled={loading}>
            <RefreshCw className={cn("h-3.5 w-3.5", loading && "animate-spin")} />
            刷新
          </Button>
        }
      />

      <div className="reveal space-y-4 p-4 sm:p-6 lg:p-8">
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
              className="h-10 min-w-36 px-3 text-sm"
              value={country}
              onChange={(e) => setCountry(e.target.value)}
              aria-label="国家筛选"
            >
              <option value="">全部国家</option>
              {countries.map((item) => (
                <option key={item.code} value={item.code}>
                  {item.flag} {item.name || item.code} ({item.count})
                </option>
              ))}
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
            <label className="relative w-32">
              <span className="sr-only">最低评分</span>
              <Input
                type="number"
                min="0"
                max="100"
                value={minScore}
                onChange={(e) => setMinScore(e.target.value)}
                placeholder="最低评分"
              />
            </label>
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
            <label className="flex items-center gap-2 text-sm text-slate-400">
              <input
                type="checkbox"
                checked={verified}
                onChange={(e) => setVerified(e.target.checked)}
                className="accent-emerald-400"
              />
              真实拨测
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
                    <th>国家</th>
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
                        <td className="font-mono text-xs text-slate-500">
                          <span className="inline-flex items-center gap-1">
                            <Globe2 className="h-3 w-3" /> {n.country || "ZZ"}
                          </span>
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
                            onClick={() => n.raw_uri && copyURI(n.raw_uri, n.id)}
                            disabled={!n.raw_uri}
                            title={n.raw_uri ? "复制 URI" : "输入管理 Token 后可复制"}
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
                      <td colSpan={12} className="py-12 text-center text-slate-500">
                        无匹配节点
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </CardContent>
        </Card>
        {nextCursor && (
          <div className="flex justify-center">
            <Button
              variant="secondary"
              disabled={loading}
              onClick={() => load(nextCursor, true)}
            >
              {loading ? "加载中…" : `继续加载（已显示 ${nodes.length}/${total}）`}
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}
