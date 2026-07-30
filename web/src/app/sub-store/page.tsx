"use client";

import { useCallback, useEffect, useState } from "react";
import { Copy, ExternalLink } from "lucide-react";
import { AuthRequired } from "@/components/auth-required";
import { PageHeader } from "@/components/page-header";
import { useSession } from "@/components/session-provider";
import { StatusBadge } from "@/components/status-badge";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
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
      <PageHeader
        title="Sub-Store 订阅工坊"
        description="将 NodeHarvest 发布结果接入 Sub-Store 进行订阅加工。"
      />
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
            <StatusBadge status="enabled">已启用</StatusBadge>
            {Object.entries(sources).map(([format, value]) => (
              <div key={format} className="space-y-2 rounded-md border p-4">
                <p className="text-sm font-medium">{format}</p>
                <p className="break-all font-mono text-xs text-muted-foreground">{value}</p>
                <Button type="button" size="sm" variant="outline" onClick={() => void copy(value, format)}>
                  <Copy />
                  {copied === format ? "已复制" : "复制数据源"}
                </Button>
              </div>
            ))}
            <Button asChild variant="secondary">
              <a href={subStore.frontend_url} target="_blank" rel="noreferrer">
                <ExternalLink />
                打开 Sub-Store
              </a>
            </Button>
          </CardContent>
        </Card>
      )}
      {!config && !error && (
        <Card>
          <CardContent className="py-12 text-center text-muted-foreground">
            正在加载 Sub-Store…
          </CardContent>
        </Card>
      )}
    </>
  );
}
