"use client";

import { useCallback, useEffect, useState } from "react";
import { useLiveRefresh } from "@/components/live-provider";
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
      <header>
        <h1>订阅池与导出</h1>
        <p>使用公开订阅池，或按条件下载节点列表。</p>
      </header>
      {error && (
        <Alert variant="destructive">
          <AlertTitle>导出失败</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <Card>
        <CardHeader>
          <CardTitle>公开订阅池</CardTitle>
          <CardDescription>复制适合客户端的订阅地址。</CardDescription>
        </CardHeader>
        <CardContent>
          {pools.map((pool) => (
            <section key={pool.key}>
              <h2>{pool.title + " · " + pool.count + " 个节点"}</h2>
              <p>{pool.description}</p>
              {(["raw", "base64", "clash"] as const).map((format) => {
                const value = origin + pool.urls[format];
                const key = pool.key + ":" + format;
                return (
                  <p key={format}>
                    {format + ": " + value}
                    <Button type="button" size="sm" variant="outline" onClick={() => void copy(value, key)}>
                      {copied === key ? "已复制" : "复制"}
                    </Button>
                  </p>
                );
              })}
            </section>
          ))}
          {!pools.length && <p>暂无订阅池。</p>}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>临时导出</CardTitle>
          <CardDescription>导出接口需要登录，浏览器会附带当前会话凭证。</CardDescription>
        </CardHeader>
        <CardContent>
          <Label htmlFor="export-limit">数量上限</Label>
          <Input
            id="export-limit"
            type="number"
            min="1"
            value={limit}
            onChange={(event) => setLimit(event.target.value)}
          />
          <Label htmlFor="export-score">最低评分</Label>
          <Input
            id="export-score"
            type="number"
            min="0"
            max="100"
            value={minScore}
            onChange={(event) => setMinScore(event.target.value)}
          />
          <Label htmlFor="export-ai">AI 过滤</Label>
          <Input
            id="export-ai"
            value={ai}
            placeholder="例如 chatgpt"
            onChange={(event) => setAI(event.target.value)}
          />
          <p>{raw}</p>
          <Button type="button" onClick={() => void download(raw, "nodes.txt")}>
            下载原始节点
          </Button>
          <p>{base64}</p>
          <Button type="button" variant="outline" onClick={() => void download(base64, "nodes.base64.txt")}>
            下载 Base64
          </Button>
        </CardContent>
      </Card>
    </>
  );
}
