"use client";

import { useEffect, useState } from "react";
import { Copy, Download, FileJson, Link2, Radio } from "lucide-react";
import { api, exportBase64Url, exportRawUrl } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";

export default function ExportPage() {
  const [limit, setLimit] = useState("300");
  const [minScore, setMinScore] = useState("70");
  const [ai, setAi] = useState("");
  const [origin, setOrigin] = useState("");
  const [pub, setPub] = useState<{
    enabled?: boolean;
    path_prefix?: string;
    token_set?: boolean;
    public_url?: string;
    min_score?: number;
    max_nodes?: number;
  } | null>(null);
  const [schedule, setSchedule] = useState<Record<string, unknown> | null>(null);
  const [copied, setCopied] = useState<string | null>(null);

  useEffect(() => {
    setOrigin(window.location.origin);
    api.config().then((c) => {
      const p = (c.publish || {}) as typeof pub;
      setPub(p);
    }).catch(() => {});
    api.schedule().then(setSchedule).catch(() => {});
  }, []);

  const params: Record<string, string> = {
    limit,
    min_score: minScore,
    alive: "1",
    hq: Number(minScore) >= 70 ? "1" : "0",
  };
  if (ai) params.ai = ai;

  const raw = exportRawUrl(params);
  const b64 = exportBase64Url(params);

  const prefix = pub?.path_prefix || "/sub";
  const base = (pub?.public_url as string) || origin || "";
  const tokenHint = pub?.token_set ? "?token=YOUR_TOKEN" : "";
  const subLinks = {
    index: `${base}${prefix}${tokenHint}`,
    raw: `${base}${prefix}/raw${tokenHint}`,
    base64: `${base}${prefix}/base64${tokenHint}`,
    clash: `${base}${prefix}/clash${tokenHint}`,
  };

  async function copy(text: string, key: string) {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(key);
      setTimeout(() => setCopied(null), 1500);
    } catch {
      /* ignore */
    }
  }

  return (
    <div className="flex flex-1 flex-col">
      <header className="border-b border-slate-800/80 px-8 py-5">
        <h1 className="font-[family-name:var(--font-display)] text-xl font-semibold">
          导出 / 远程订阅
        </h1>
        <p className="mt-1 text-sm text-slate-500">
          高质量节点筛选后导出；VPS 上可把 /sub 链接分发给其他用户
        </p>
      </header>

      <div className="space-y-6 p-8">
        <Card className="border-cyan-500/20">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Radio className="h-4 w-4 text-cyan-300" />
              公开订阅端点
            </CardTitle>
            <CardDescription>
              客户端直接填这些 URL（定时任务会持续刷新高质量池）
              {pub?.token_set ? " · 已启用 Token" : " · 当前无 Token（生产环境请设置）"}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3 text-sm">
            {(["base64", "clash", "raw", "index"] as const).map((k) => (
              <div key={k} className="flex flex-col gap-1 sm:flex-row sm:items-center sm:gap-3">
                <span className="w-16 shrink-0 font-mono text-xs uppercase text-slate-500">{k}</span>
                <code className="min-w-0 flex-1 break-all rounded-lg bg-slate-950 px-3 py-2 text-xs text-cyan-100/90">
                  {subLinks[k] || `${prefix}/${k === "index" ? "" : k}`}
                </code>
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={() => copy(subLinks[k], k)}
                  disabled={!subLinks[k]}
                >
                  <Copy className="h-3.5 w-3.5" />
                  {copied === k ? "已复制" : "复制"}
                </Button>
              </div>
            ))}
            <p className="text-xs text-slate-500">
              筛选：score ≥ {pub?.min_score ?? 70} · 最多 {pub?.max_nodes ?? 500} · 仅存活
              {schedule?.enabled ? (
                <> · 定时 {String(schedule.interval_min)} 分钟 / {String(schedule.job)}</>
              ) : (
                <> · 定时未启用</>
              )}
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>临时导出条件</CardTitle>
            <CardDescription>走 /api/export/*，可带 AI 过滤</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-4 sm:grid-cols-3">
            <label className="space-y-1.5 text-sm">
              <span className="text-slate-400">数量上限</span>
              <Input value={limit} onChange={(e) => setLimit(e.target.value)} />
            </label>
            <label className="space-y-1.5 text-sm">
              <span className="text-slate-400">最低分数</span>
              <Input value={minScore} onChange={(e) => setMinScore(e.target.value)} />
            </label>
            <label className="space-y-1.5 text-sm">
              <span className="text-slate-400">AI 过滤（可选）</span>
              <select
                className="flex h-10 w-full rounded-md border border-slate-700 bg-slate-950 px-3 text-sm"
                value={ai}
                onChange={(e) => setAi(e.target.value)}
              >
                <option value="">不限</option>
                {["chatgpt", "gemini", "claude", "grok", "openai"].map((k) => (
                  <option key={k} value={k}>
                    {k}
                  </option>
                ))}
              </select>
            </label>
          </CardContent>
        </Card>

        <div className="grid gap-4 md:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Link2 className="h-4 w-4 text-cyan-300" />
                原始 URI 列表
              </CardTitle>
              <CardDescription>每行一个节点链接</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              <code className="block break-all rounded-lg bg-slate-950 p-3 text-xs text-slate-400">
                {raw}
              </code>
              <Button asChild>
                <a href={raw}>
                  <Download className="h-4 w-4" />
                  下载 nodes.txt
                </a>
              </Button>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <FileJson className="h-4 w-4 text-amber-300" />
                Base64 订阅
              </CardTitle>
              <CardDescription>客户端订阅导入</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              <code className="block break-all rounded-lg bg-slate-950 p-3 text-xs text-slate-400">
                {b64}
              </code>
              <Button asChild variant="amber">
                <a href={b64}>
                  <Download className="h-4 w-4" />
                  下载 base64
                </a>
              </Button>
            </CardContent>
          </Card>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>磁盘导出（任务完成后）</CardTitle>
            <CardDescription>
              写入 <code className="text-cyan-300">output/</code>，可用 Nginx 静态托管
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-1 text-sm text-slate-400">
            <p>· sub.txt / sub.base64 / clash.yaml（稳定文件名，适合远程拉取）</p>
            <p>· nodes-latest.txt / .base64.txt / .json / .clash.yaml</p>
            <p>· nodes-ai-friendly.txt（任一 AI 启发通过）</p>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
