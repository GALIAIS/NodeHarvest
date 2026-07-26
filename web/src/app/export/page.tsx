"use client";

import { useEffect, useState } from "react";
import { AlertTriangle, Check, Copy, Download, FileJson, Link2, Radio } from "lucide-react";
import {
  api,
  ApiError,
  errorMessage,
  exportBase64Url,
  exportRawUrl,
  getAdminToken,
  type Pool,
} from "@/lib/api";
import { PageHeader } from "@/components/page-header";
import { useSession } from "@/components/session-provider";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Field } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

/** Radix Select reserves the empty string, so "all" stands in for "no filter". */
const ALL = "all";
const unset = (value: string) => (value === ALL ? "" : value);

/** `/api/export/*` is admin-gated, so the buttons need a reason when disabled. */
function DownloadHint() {
  return (
    <p className="text-[10px] leading-4 text-muted-foreground">
      导出接口需要登录；页首的公开订阅端点无需登录即可使用。
    </p>
  );
}

export default function ExportPage() {
  const { authenticated, loading: sessionLoading } = useSession();
  const [limit, setLimit] = useState("300");
  const [minScore, setMinScore] = useState("70");
  const [ai, setAi] = useState(ALL);
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
  // 会话未定时先放行，避免刷新瞬间按钮闪成禁用态
  const canDownload = authenticated || sessionLoading;

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
  const aiFilter = unset(ai);
  if (aiFilter) params.ai = aiFilter;

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
        // 保留状态码，errorMessage 才能把 401/403 翻译成友好提示
        throw new ApiError(res.status, (await res.text()) || res.statusText);
      }
      const href = URL.createObjectURL(await res.blob());
      const link = document.createElement("a");
      link.href = href;
      link.download = filename;
      link.click();
      URL.revokeObjectURL(href);
    } catch (error) {
      setDownloadError(errorMessage(error, "下载失败"));
    }
  }

  return (
    <div className="flex flex-1 flex-col">
      <PageHeader
        eyebrow="Distribution plane"
        title="订阅池与导出"
        description="按用途分发独立智能池，或临时组合筛选条件导出。发布缓存支持 ETag、CDN 与对象存储快照。"
      />

      <div className="reveal space-y-4 p-4 sm:p-6 lg:p-8">
        <Card className="border-primary/25">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Radio className="size-4 text-primary" />
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
                <span className="w-16 shrink-0 font-mono text-[10px] uppercase tracking-[0.16em] text-muted-foreground">
                  {k}
                </span>
                <code className="min-w-0 flex-1 break-all border border-border bg-popover px-3 py-2 font-mono text-xs text-primary/90">
                  {subLinks[k] || `${prefix}/${k === "index" ? "" : k}`}
                </code>
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={() => copy(subLinks[k], k)}
                  disabled={!subLinks[k]}
                >
                  <Copy className="size-3.5" />
                  {copied === k ? "已复制" : "复制"}
                </Button>
              </div>
            ))}
            <p className="text-xs text-muted-foreground">
              筛选：score ≥ {pub?.min_score ?? 70} · 最多 {pub?.max_nodes ?? 500} · 仅存活
              {schedule?.enabled ? (
                <> · 定时 {String(schedule.interval_min)} 分钟 / {String(schedule.job)}</>
              ) : (
                <> · 定时未启用</>
              )}
            </p>
            {pub?.token_set && !allowQueryToken && (
              <p className="text-xs text-accent">
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
                      <span className="w-16 shrink-0 font-mono text-[10px] uppercase text-muted-foreground">
                        {format}
                      </span>
                      <code className="min-w-0 flex-1 truncate border border-border bg-popover px-2 py-1.5 font-mono text-[10px] text-muted-foreground">
                        {url}
                      </code>
                      <Button
                        size="icon-sm"
                        variant="ghost"
                        aria-label={`复制 ${pool.title} ${format} 订阅链接`}
                        onClick={() => copy(url, key)}
                      >
                        {copied === key ? (
                          <Check className="size-3.5 text-success" />
                        ) : (
                          <Copy className="size-3.5" />
                        )}
                      </Button>
                    </div>
                  );
                })}
                <p className="pt-2 font-mono text-[10px] tabular-nums text-muted-foreground">
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
            <Field label="数量上限" htmlFor="export-limit">
              <Input id="export-limit" value={limit} onChange={(e) => setLimit(e.target.value)} />
            </Field>
            <Field label="最低分数" htmlFor="export-min-score">
              <Input
                id="export-min-score"
                value={minScore}
                onChange={(e) => setMinScore(e.target.value)}
              />
            </Field>
            <Field label="AI 过滤（可选）">
              <Select value={ai} onValueChange={setAi}>
                <SelectTrigger className="w-full" aria-label="AI 过滤">
                  <SelectValue placeholder="不限" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={ALL}>不限</SelectItem>
                  {["chatgpt", "gemini", "claude", "grok", "openai"].map((k) => (
                    <SelectItem key={k} value={k} className="font-mono">
                      {k}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
          </CardContent>
        </Card>

        <div className="grid gap-4 md:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Link2 className="size-4 text-primary" />
                原始 URI 列表
              </CardTitle>
              <CardDescription>每行一个节点链接</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              <code className="block break-all border border-border bg-popover p-3 font-mono text-xs text-muted-foreground">
                {raw}
              </code>
              <Button onClick={() => download(raw, "nodes.txt")} disabled={!canDownload}>
                <Download className="size-4" />
                下载 nodes.txt
              </Button>
              {!canDownload && <DownloadHint />}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <FileJson className="size-4 text-accent" />
                Base64 订阅
              </CardTitle>
              <CardDescription>客户端订阅导入</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              <code className="block break-all border border-border bg-popover p-3 font-mono text-xs text-muted-foreground">
                {b64}
              </code>
              <Button
                onClick={() => download(b64, "nodes.base64.txt")}
                variant="accent"
                disabled={!canDownload}
              >
                <Download className="size-4" />
                下载 base64
              </Button>
              {!canDownload && <DownloadHint />}
            </CardContent>
          </Card>
        </div>
        {downloadError && (
          <Alert variant="danger">
            <AlertTriangle />
            <AlertDescription>{downloadError}</AlertDescription>
          </Alert>
        )}

        <Card>
          <CardHeader>
            <CardTitle>磁盘导出（任务完成后）</CardTitle>
            <CardDescription>
              写入 <code className="text-primary">output/</code>，可用 Nginx 静态托管
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-1 text-sm text-muted-foreground">
            <p>· sub.txt / sub.base64 / clash.yaml（稳定文件名，适合远程拉取）</p>
            <p>· nodes-latest.txt / .base64.txt / .json / .clash.yaml</p>
            <p>· nodes-ai-friendly.txt（任一 AI 启发通过）</p>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
