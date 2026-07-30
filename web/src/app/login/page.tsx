"use client";

import type { FormEvent } from "react";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
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
import { api, errorMessage } from "@/lib/api";

export default function LoginPage() {
  const router = useRouter();
  const { loading, refresh } = useSession();
  const [form, setForm] = useState({ tenant: "default", username: "", password: "" });
  const [localEnabled, setLocalEnabled] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;
    void api.me().then(
      (session) => {
        if (cancelled) return;
        setLocalEnabled(session.local_enabled);
        if (session.authenticated) router.replace("/");
      },
      () => {},
    );
    return () => {
      cancelled = true;
    };
  }, [router]);

  async function login(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api.login(form);
      await refresh();
      router.replace("/");
      router.refresh();
    } catch (cause) {
      setError(errorMessage(cause, "登录失败"));
    } finally {
      setBusy(false);
    }
  }

  if (loading) return <p>正在检查登录状态…</p>;

  return (
    <>
      <header className="space-y-1">
        <h1 className="text-3xl font-bold tracking-tight">管理登录</h1>
        <p className="text-muted-foreground">管理面仅支持账户与密码登录；订阅 Token 不能登录控制台。</p>
      </header>
      {error && (
        <Alert variant="destructive">
          <AlertTitle>登录失败</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <Card className="mx-auto max-w-md">
        <CardHeader>
          <CardTitle>账户登录</CardTitle>
          <CardDescription>使用租户隔离的 RBAC 会话。</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="space-y-4" onSubmit={login}>
            <Label htmlFor="tenant">租户</Label>
            <Input
              id="tenant"
              value={form.tenant}
              onChange={(event) => setForm({ ...form, tenant: event.target.value })}
              required
            />
            <Label htmlFor="username">用户名</Label>
            <Input
              id="username"
              value={form.username}
              onChange={(event) => setForm({ ...form, username: event.target.value })}
              required
              autoComplete="username"
            />
            <Label htmlFor="password">密码</Label>
            <Input
              id="password"
              type="password"
              value={form.password}
              onChange={(event) => setForm({ ...form, password: event.target.value })}
              required
              autoComplete="current-password"
            />
            <Button type="submit" disabled={busy || !localEnabled}>
              {busy ? "登录中…" : localEnabled ? "登录" : "本地登录未启用"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </>
  );
}
