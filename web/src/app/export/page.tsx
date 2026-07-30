"use client";

import { useCallback, useEffect, useState } from "react";
import { Copy, Download } from "lucide-react";
import { useLiveRefresh } from "@/components/live-provider";
import { PageHeader } from "@/components/page-header";
import { useSession } from "@/components/session-provider";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  ApiError,
  api,
  errorMessage,
  exportBase64Url,
  exportRawUrl,
  type Pool,
} from "@/lib/api";

export default function ExportPage() {
  const { authenticated } = useSession();
  const [pools, setPools] = useState<Pool[]>([]);
  const [origin, setOrigin] = useState("");
  const [limit, setLimit] = useState("300");
  const [minScore, setMinScore] = useState("70");
  const [ai, setAI] = useState("");
  const [error, setError] = useState("");
  const [copied, setCopied] = useState("");

  const load = useCallback(async () => {
    try {
      setPools(await api.pools());
      setError("");
    } catch (cause) {
      setError(errorMessage(cause, "加载订阅池失败"));
    }
  }, []);

  useEffect(() => {
    const initial = window.setTimeout(() => setOrigin(window.location.origin), 0);
    return () => window.clearTimeout(initial);
  }, []);

  useEffect(() => {
    const initial = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(initial);
  }, [load]);

  useLiveRefresh(load, authenticated);

  const params: Record<string, string> = {
    limit,
    min_score: minScore,
    alive: "1",
    hq: Number(minScore) >= 70 ? "1" : "0",
  };
  if (ai) params.ai = ai;
  const raw = exportRawUrl(params);
  const base64 = exportBase64Url(params);

  async function copy(value: string, key: string) {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(key);
    } catch {
      setError("无法复制链接。");
    }
  }

  async function download(url: string, filename: string) {
    try {
      const response = await fetch(url, {
        credentials: "same-origin",
      });
      if (!response.ok) throw new ApiError(response.status, (await response.text()) || response.statusText);
      const objectURL = URL.createObjectURL(await response.blob());
      const link = document.createElement("a");
      link.href = objectURL;
      link.download = filename;
      link.click();
      URL.revokeObjectURL(objectURL);
      setError("");
    } catch (cause) {
      setError(errorMessage(cause, "下载失败"));
    }
  }

  return (
    <>
      <PageHeader
        title="订阅池与导出"
        description="复制公开订阅地址，或按质量条件生成临时节点列表。"
      />
      {error && (
        <Alert variant="destructive">
          <AlertTitle>导出失败</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <section className="space-y-4" aria-labelledby="public-pools-title">
        <div>
          <h2 id="public-pools-title" className="text-lg font-semibold">公开订阅池</h2>
          <p className="text-sm text-muted-foreground">复制适合客户端格式的订阅地址。</p>
        </div>
        <div className="grid gap-4 xl:grid-cols-2">
          {pools.map((pool) => (
            <Card key={pool.key} className="min-w-0">
              <CardHeader>
                <CardTitle>{pool.title}</CardTitle>
                <CardDescription>
                  {pool.description + " · " + pool.count + " 个节点"}
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-3">
                {(["raw", "base64", "clash"] as const).map((format) => {
                  const value = origin + pool.urls[format];
                  const key = pool.key + ":" + format;
                  return (
                    <div key={format} className="space-y-2 rounded-md border p-3">
                      <p className="text-sm font-medium">{format}</p>
                      <p className="break-all font-mono text-xs text-muted-foreground">
                        {value}
                      </p>
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        onClick={() => void copy(value, key)}
                      >
                        <Copy />
                        {copied === key ? "已复制" : "复制地址"}
                      </Button>
                    </div>
                  );
                })}
              </CardContent>
            </Card>
          ))}
          {!pools.length && (
            <Card>
              <CardContent className="py-12 text-center text-muted-foreground">
                暂无订阅池。
              </CardContent>
            </Card>
          )}
        </div>
      </section>
      <Card>
        <CardHeader>
          <CardTitle>临时导出</CardTitle>
          <CardDescription>导出接口需要登录，浏览器会附带当前会话凭证。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="grid gap-4 md:grid-cols-3">
            <div className="space-y-2">
              <Label htmlFor="export-limit">数量上限</Label>
              <Input
                id="export-limit"
                type="number"
                min="1"
                value={limit}
                onChange={(event) => setLimit(event.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="export-score">最低评分</Label>
              <Input
                id="export-score"
                type="number"
                min="0"
                max="100"
                value={minScore}
                onChange={(event) => setMinScore(event.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="export-ai">AI 过滤</Label>
              <Input
                id="export-ai"
                value={ai}
                placeholder="例如 chatgpt"
                onChange={(event) => setAI(event.target.value)}
              />
            </div>
          </div>
          <div className="grid gap-4 xl:grid-cols-2">
            <div className="space-y-3 rounded-md border p-4">
              <div className="space-y-1">
                <p className="font-medium">原始节点</p>
                <p className="break-all font-mono text-xs text-muted-foreground">{raw}</p>
              </div>
              <Button type="button" onClick={() => void download(raw, "nodes.txt")}>
                <Download />
                下载原始节点
              </Button>
            </div>
            <div className="space-y-3 rounded-md border p-4">
              <div className="space-y-1">
                <p className="font-medium">Base64</p>
                <p className="break-all font-mono text-xs text-muted-foreground">{base64}</p>
              </div>
              <Button
                type="button"
                variant="outline"
                onClick={() => void download(base64, "nodes.base64.txt")}
              >
                <Download />
                下载 Base64
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>
    </>
  );
}
