"use client";

import { useCallback, useEffect, useState } from "react";
import { AlertTriangle, Check, Code, Copy, ExternalLink, Workflow } from "lucide-react";
import { api, errorMessage } from "@/lib/api";
import { AuthRequired } from "@/components/auth-required";
import { PageHeader } from "@/components/page-header";
import { useSession } from "@/components/session-provider";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

type SubStoreInfo = {
  enabled: boolean;
  public_url: string;
  backend_path: string;
  frontend_url: string;
  version: string;
};

type IntegrationConfig = {
  sub_store?: SubStoreInfo;
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
  const [error, setError] = useState("");
  const [copied, setCopied] = useState("");

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
    const initial = setTimeout(load, 0);
    return () => clearTimeout(initial);
  }, [canUseSubStore, load]);

  async function copy(value: string, key: string) {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(key);
      setTimeout(() => setCopied(""), 1500);
    } catch {
      setError("浏览器未授予剪贴板权限");
    }
  }

  if (!loading && !canUseSubStore) {
    return (
      <div className="flex flex-1 flex-col">
        <PageHeader
          eyebrow="Subscription workshop"
          title="Sub-Store 订阅工坊"
          description="完整的订阅转换、组合、过滤、脚本处理、文件托管与多客户端输出。"
        />
        <div className="p-4 sm:p-6 lg:p-8">
          <AuthRequired
            reason="forbidden"
            title="需要操作员权限"
            description="Sub-Store 是共享系统工作区，仅 default 租户的 operator 或 admin 账号可使用。"
          />
        </div>
      </div>
    );
  }

  const subStore = config?.sub_store;
  const publish = config?.publish;
  const sourceBase = publish?.public_url || "";
  const sourcePrefix = publish?.path_prefix || "/sub";
  const tokenHint = publish?.token_set && config?.security?.allow_query_token ? "?token=YOUR_NODE_TOKEN" : "";
  const sources = {
    raw: `${sourceBase}${sourcePrefix}/raw${tokenHint}`,
    base64: `${sourceBase}${sourcePrefix}/base64${tokenHint}`,
    clash: `${sourceBase}${sourcePrefix}/clash${tokenHint}`,
  };

  return (
    <div className="flex flex-1 flex-col">
      <PageHeader
        eyebrow="Subscription workshop"
        title="Sub-Store 订阅工坊"
        description="上游完整功能由独立容器提供，NodeHarvest 账号会话负责管理面认证，节点 Token 只负责对外订阅。"
        actions={
          subStore?.enabled && subStore.frontend_url ? (
            <>
              <Button size="sm" variant="ghost" asChild>
                <a
                  href={`https://github.com/sub-store-org/Sub-Store/tree/${encodeURIComponent(subStore.version)}`}
                  target="_blank"
                  rel="noreferrer"
                >
                  <Code className="size-3.5" />
                  对应源码
                </a>
              </Button>
              <Button size="sm" variant="secondary" asChild>
                <a href={subStore.frontend_url} target="_blank" rel="noreferrer">
                  <ExternalLink className="size-3.5" />
                  全屏打开
                </a>
              </Button>
            </>
          ) : undefined
        }
      />

      <div className="reveal space-y-4 p-4 sm:p-6 lg:p-8">
        {error && (
          <Alert variant="danger">
            <AlertTriangle />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {config && !subStore?.enabled && (
          <Alert>
            <Workflow />
            <AlertDescription>
              Sub-Store 尚未启用。请按部署文档设置独立子域、共享会话域和后端路径后重启生产栈。
            </AlertDescription>
          </Alert>
        )}

        {subStore?.enabled && (
          <>
            <Card>
              <CardHeader>
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div>
                    <CardTitle>NodeHarvest 数据源</CardTitle>
                    <CardDescription className="mt-1">
                      可直接作为 Sub-Store 单条订阅；模板不会暴露真实 Token。
                    </CardDescription>
                  </div>
                  <Badge variant="success">Sub-Store {subStore.version}</Badge>
                </div>
              </CardHeader>
              <CardContent className="space-y-2">
                {Object.entries(sources).map(([format, value]) => (
                  <div key={format} className="flex items-center gap-2">
                    <span className="w-14 shrink-0 font-mono text-[10px] uppercase text-muted-foreground">
                      {format}
                    </span>
                    <code className="min-w-0 flex-1 truncate border border-border bg-popover px-2 py-1.5 font-mono text-[10px] text-muted-foreground">
                      {value}
                    </code>
                    <Button
                      size="icon-sm"
                      variant="ghost"
                      aria-label={`复制 ${format} 数据源`}
                      disabled={!sourceBase}
                      onClick={() => void copy(value, format)}
                    >
                      {copied === format ? (
                        <Check className="size-3.5 text-success" />
                      ) : (
                        <Copy className="size-3.5" />
                      )}
                    </Button>
                  </div>
                ))}
                <p className="pt-1 text-xs leading-relaxed text-muted-foreground">
                  对外使用 Sub-Store 生成的 /share 或 /download 链接时，追加
                  <code className="mx-1 text-primary">nh_token=YOUR_NODE_TOKEN</code>。
                  浏览器内预览由账号会话授权，不消耗订阅 Token 配额。
                </p>
                {publish?.token_set && !config?.security?.allow_query_token && (
                  <p className="text-xs text-accent">
                    URL Token 当前关闭。Sub-Store 数据源需改用自定义 Authorization /
                    X-Sub-Token 请求头，或显式开启 NODE_HARVEST_ALLOW_QUERY_TOKEN。
                  </p>
                )}
              </CardContent>
            </Card>

            <Card className="overflow-hidden border-primary/25">
              <CardContent className="p-0">
                <iframe
                  title="Sub-Store 订阅管理"
                  src={subStore.frontend_url}
                  referrerPolicy="no-referrer"
                  allow="clipboard-read; clipboard-write"
                  className="h-[calc(100dvh-10rem)] min-h-[640px] w-full bg-background"
                />
              </CardContent>
            </Card>
          </>
        )}

        {!config && !error && (
          <Card>
            <CardContent className="py-16 text-center font-mono text-xs text-muted-foreground">
              正在加载 Sub-Store…
            </CardContent>
          </Card>
        )}
      </div>
    </div>
  );
}
