"use client";

import { useEffect, useState } from "react";
import { AlertTriangle, ExternalLink, Scale, ShieldAlert } from "lucide-react";
import { api, type Terms } from "@/lib/api";
import { PageHeader } from "@/components/page-header";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

export default function TermsPage() {
  const [terms, setTerms] = useState<Terms | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    api.terms().then(setTerms).catch((cause) => setError(cause instanceof Error ? cause.message : "加载失败"));
  }, []);

  return (
    <div className="flex flex-1 flex-col">
      <PageHeader
        eyebrow="Acceptable use"
        title="合规与使用边界"
        description="本页来自服务端当前治理配置。运营方应结合当地法律、上游条款与组织政策进行二次审核。"
      />
      <div className="reveal mx-auto w-full max-w-5xl space-y-4 p-4 sm:p-6 lg:p-8">
        {error && (
          <Alert variant="danger">
            <AlertTriangle />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        <Card className="border-accent/40">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-accent">
              <ShieldAlert className="size-4" />
              重要声明
            </CardTitle>
            <CardDescription>{terms?.title || "nodeharvest acceptable use"}</CardDescription>
          </CardHeader>
          <CardContent className="text-sm leading-7 text-muted-foreground">
            {terms?.notice || "采集到的端点来自独立第三方，不对可用性、安全性或合法性作任何保证。"}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Scale className="size-4 text-primary" />
              使用限制
            </CardTitle>
            <CardDescription>访问或使用服务即表示理解并遵守以下限制。</CardDescription>
          </CardHeader>
          <CardContent>
            <ol className="space-y-3">
              {(terms?.restrictions || [
                "仅在合法且已获授权的场景使用。",
                "不得用于滥用、入侵、规避控制或非法访问。",
                "运营方可停用违反政策的采集源、凭证或租户。",
              ]).map((restriction, index) => (
                <li
                  key={restriction}
                  className="grid grid-cols-[2rem_1fr] gap-3 border border-border bg-muted/40 p-4 text-sm leading-6 text-muted-foreground"
                >
                  <span className="font-mono text-primary">0{index + 1}</span>
                  {restriction}
                </li>
              ))}
            </ol>
          </CardContent>
        </Card>
        {terms?.terms_url && (
          <a
            href={terms.terms_url}
            target="_blank"
            rel="noreferrer"
            className="flex items-center justify-between border border-primary/30 bg-primary/5 px-5 py-4 text-sm text-primary transition-colors hover:bg-primary/10"
          >
            查看运营方完整条款
            <ExternalLink className="size-4" />
          </a>
        )}
      </div>
    </div>
  );
}
