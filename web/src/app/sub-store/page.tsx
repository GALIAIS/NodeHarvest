"use client";

import { useCallback, useEffect, useState } from "react";
import { AuthRequired } from "@/components/auth-required";
import { useSession } from "@/components/session-provider";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { api, errorMessage } from "@/lib/api";

type IntegrationConfig = {
  sub_store?: {
    enabled: boolean;
    frontend_url: string;
    version: string;
  };
  publish?: {
    public_url?: string;
    path_prefix?: string;
    token_set?: boolean;
  };
  security?: {
    allow_query_token?: boolean;
  };
};

export default function SubStorePage() {
  const { canOperate, loading, session } = useSession();
  const canUseSubStore = canOperate && session?.principal.tenant_id === "default";
  const [config, setConfig] = useState<IntegrationConfig | null>(null);
  const [copied, setCopied] = useState("");
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    try {
      setConfig((await api.config()) as IntegrationConfig);
      setError("");
    } catch (cause) {
      setError(errorMessage(cause, "加载 Sub-Store 配置失败"));
    }
  }, []);

  useEffect(() => {
    if (!canUseSubStore) return;
    const initial = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(initial);
  }, [canUseSubStore, load]);

  async function copy(value: string, key: string) {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(key);
    } catch {
      setError("无法复制数据源。");
    }
  }

  if (!loading && !canUseSubStore) {
    return <AuthRequired reason="forbidden" title="需要 default 租户的操作员权限" />;
  }

  const subStore = config?.sub_store;
  const publish = config?.publish;
  const base = publish?.public_url || "";
  const prefix = publish?.path_prefix || "/sub";
  const tokenHint = publish?.token_set && config?.security?.allow_query_token ? "?token=YOUR_NODE_TOKEN" : "";
  const sources = {
    raw: base + prefix + "/raw" + tokenHint,
    base64: base + prefix + "/base64" + tokenHint,
    clash: base + prefix + "/clash" + tokenHint,
  };

  return (
    <>
      <header className="space-y-1">
        <h1 className="text-3xl font-bold tracking-tight">Sub-Store 订阅工坊</h1>
        <p className="text-muted-foreground">使用 NodeHarvest 订阅作为 Sub-Store 数据源。</p>
      </header>
      {error && (
        <Alert variant="destructive">
          <AlertTitle>加载失败</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      {!subStore?.enabled && config && (
        <Alert>
          <AlertTitle>Sub-Store 未启用</AlertTitle>
          <AlertDescription>请配置独立服务后重启生产栈。</AlertDescription>
        </Alert>
      )}
      {subStore?.enabled && (
        <Card>
          <CardHeader>
            <CardTitle>数据源</CardTitle>
            <CardDescription>Sub-Store {subStore.version}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <Badge>已启用</Badge>
            {Object.entries(sources).map(([format, value]) => (
              <p key={format}>
                {format + ": " + value}
                <Button type="button" size="sm" variant="outline" onClick={() => void copy(value, format)}>
                  {copied === format ? "已复制" : "复制"}
                </Button>
              </p>
            ))}
            <Button asChild variant="secondary">
              <a href={subStore.frontend_url} target="_blank" rel="noreferrer">
                打开 Sub-Store
              </a>
            </Button>
          </CardContent>
        </Card>
      )}
      {!config && !error && <p>正在加载 Sub-Store…</p>}
    </>
  );
}
