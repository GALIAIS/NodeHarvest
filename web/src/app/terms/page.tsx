"use client";

import { useEffect, useState } from "react";
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
      <header>
        <h1>合规</h1>
        <p>使用 NodeHarvest 前请阅读相关说明。</p>
      </header>
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
        <CardContent>
          <p>{terms?.restrictions.join("；") ?? ""}</p>
          {terms?.terms_url && (
            <Button asChild variant="outline">
              <a href={terms.terms_url} target="_blank" rel="noreferrer">
                查看完整条款
              </a>
            </Button>
          )}
        </CardContent>
      </Card>
    </>
  );
}
