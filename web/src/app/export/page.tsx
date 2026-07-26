"use client";

import { useEffect, useState } from "react";
import { Copy, Download, FileJson, Link2, Radio } from "lucide-react";
import { api, exportBase64Url, exportRawUrl, getAdminToken, type Pool } from "@/lib/api";
import { PageHeader } from "@/components/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";

export default function ExportPage() {
  const [limit, setLimit] = useState("300");
  const [minScore, setMinScore] = useState("70");
  const [ai, setAi] = useState("");
  const [origin] = useState(() => (typeof window === "undefined" ? "" : window.location.origin));
  const [pub, setPub] = useState<{
    enabled?: boolean;
    path_prefix?: string;
    token_set?: boolean;
    public_url?: string;
    min_score?: number;
    max_nodes?: number;
  } | null>(null);
  const [schedule, setSchedule] = useState<Record<string, unknown> | null>(null);
  const [pools, setPools] = useState<Pool[]>([]);
  const [allowQueryToken, setAllowQueryToken] = useState(false);
  const [copied, setCopied] = useState<string | null>(null);
  const [downloadError, setDownloadError] = useState<string | null>(null);

  useEffect(() => {
    api.config().then((c) => {
      const p = (c.publish || {}) as typeof pub;
      setPub(p);
      const security = (c.security || {}) as { allow_query_token?: boolean };
      setAllowQueryToken(Boolean(security.allow_query_token));
    }).catch(() => {});
    api.schedule().then(setSchedule).catch(() => {});
    api.pools().then(setPools).catch(() => {});
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
  const tokenHint = pub?.token_set && allowQueryToken ? "?token=YOUR_TOKEN" : "";
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

  async function download(url: string, filename: string) {
    try {
      setDownloadError(null);
      const token = getAdminToken();
      const res = await fetch(url, {
        headers: token ? { "X-Admin-Token": token } : {},
        credentials: "same-origin",
        cache: "no-store",
      });
      if (!res.ok) {
        throw new Error((await res.text()) || res.statusText);
      }
      const href = URL.createObjectURL(await res.blob());
      const link = document.createElement("a");
      link.href = href;
      link.download = filename;
      link.click();
      URL.revokeObjectURL(href);
    } catch (error) {
      setDownloadError(error instanceof Error ? error.message : "下载失败");
    }
  }

  return (
    <div className="flex flex-1 flex-col">
      <PageHeader
        eyebrow="Distribution plane"
        title="订阅池与导出"
        description="按用途分发独立智能池，或临时组合筛选条件导出。发布缓存支持 ETag、CDN 与对象存储快照。"
      />

      <div className="reveal space-y-6 p-4 sm:p-6 lg:p-8">
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
            {pub?.token_set && !allowQueryToken && (
              <p className="text-xs text-amber-300">
                查询参数 Token 已关闭；客户端需发送 Authorization: Bearer TOKEN 或
                X-Sub-Token。若客户端只支持 URL Token，请显式开启 allow_query_token。
              </p>
            )}
          </CardContent>
        </Card>

        <div className="grid gap-3 lg:grid-cols-2 xl:grid-cols-3">
          {pools.map((pool) => (
            <Card key={pool.key}>
              <CardHeader>
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <CardTitle>{pool.title}</CardTitle>
                    <CardDescription className="mt-1">{pool.description}</CardDescription>
                  </div>
                  <Badge variant={pool.count ? "success" : "secondary"}>{pool.count} nodes</Badge>
                </div>
              </CardHeader>
              <CardContent className="space-y-2">
                {(["base64", "clash", "raw"] as const).map((format) => {
                  const url = `${base}${pool.urls[format]}${tokenHint}`;
                  const key = `${pool.key}:${format}`;
                  return (
                    <div key={format} className="flex items-center gap-2">
                      <span className="w-12 font-mono text-[9px] uppercase text-slate-600">{format}</span>
                      <code className="min-w-0 flex-1 truncate rounded bg-slate-950 px-2 py-1.5 text-[10px] text-slate-500">{url}</code>
                      <Button size="sm" variant="ghost" onClick={() => copy(url, key)}>
                        <Copy className="h-3.5 w-3.5" />{copied === key ? "OK" : ""}
                      </Button>
                    </div>
                  );
                })}
                <p className="pt-2 font-mono text-[9px] text-slate-700">
                  score ≥ {pool.min_score} · max {pool.max_nodes} · refresh {pool.refresh_sec}s
                </p>
              </CardContent>
            </Card>
          ))}
        </div>

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
              <Button onClick={() => download(raw, "nodes.txt")}>
                <Download className="h-4 w-4" />
                下载 nodes.txt
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
              <Button onClick={() => download(b64, "nodes.base64.txt")} variant="amber">
                <Download className="h-4 w-4" />
                下载 base64
              </Button>
            </CardContent>
          </Card>
        </div>
        {downloadError && <p className="text-sm text-rose-400">{downloadError}</p>}

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
