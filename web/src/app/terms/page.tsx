"use client";

import { useEffect, useState } from "react";
import { ExternalLink } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { api, errorMessage, type Terms as TermsData } from "@/lib/api";

export default function TermsPage() {
  const [terms, setTerms] = useState<TermsData | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    void api.terms().then(
      (value) => setTerms(value),
      (cause) => setError(errorMessage(cause, "加载合规信息失败")),
    );
  }, []);

  return (
    <>
      <PageHeader
        title="合规"
        description="使用 NodeHarvest 前请阅读相关条款、限制和责任说明。"
      />
      {error && (
        <Alert variant="destructive">
          <AlertTitle>加载失败</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <Card>
        <CardHeader>
          <CardTitle>{terms?.title ?? "使用条款"}</CardTitle>
          <CardDescription>{terms?.notice ?? "正在加载…"}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {terms?.restrictions.length ? (
            <ul className="list-disc space-y-2 pl-5 text-sm text-muted-foreground">
              {terms.restrictions.map((restriction) => (
                <li key={restriction}>{restriction}</li>
              ))}
            </ul>
          ) : (
            <p className="text-sm text-muted-foreground">正在加载限制说明…</p>
          )}
          {terms?.terms_url && (
            <Button asChild variant="outline">
              <a href={terms.terms_url} target="_blank" rel="noreferrer">
                <ExternalLink />
                查看完整条款
              </a>
            </Button>
          )}
        </CardContent>
      </Card>
    </>
  );
}
