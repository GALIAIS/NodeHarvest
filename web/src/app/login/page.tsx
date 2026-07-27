"use client";

import { FormEvent, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { AlertTriangle, LockKeyhole, ShieldCheck } from "lucide-react";
import { api, errorMessage } from "@/lib/api";
import { useSession } from "@/components/session-provider";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Field } from "@/components/ui/label";

export default function LoginPage() {
  const router = useRouter();
  const { refresh } = useSession();
  const [tenant, setTenant] = useState("default");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [local, setLocal] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    api
      .me()
      .then((session) => {
        setLocal(session.local_enabled);
        if (session.authenticated) router.replace("/");
      })
      .catch(() => {});
  }, [router]);

  async function login(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api.login({ tenant, username, password });
      // Let the shared session re-probe /auth/me before navigating, so the
      // landing page's first frame already renders as authenticated instead
      // of flashing anonymous panels for one paint.
      await refresh();
      router.replace("/");
      router.refresh();
    } catch (cause) {
      setError(errorMessage(cause, "登录失败"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="relative flex min-h-screen flex-1 items-center justify-center overflow-hidden p-4 sm:p-8">
      <div className="pointer-events-none absolute left-[12%] top-[16%] h-px w-36 bg-gradient-to-r from-transparent to-primary/70" />
      <div className="pointer-events-none absolute bottom-[18%] right-[9%] h-px w-48 bg-gradient-to-l from-transparent to-accent/70" />
      <div className="reveal w-full max-w-5xl">
        <div className="mb-8 grid gap-5 lg:grid-cols-[1.1fr_.9fr] lg:items-end">
          <div>
            <p className="font-mono text-[10px] uppercase tracking-[0.3em] text-primary">
              Secure operations gateway
            </p>
            <h1 className="mt-3 max-w-2xl font-display text-4xl font-semibold leading-tight text-foreground sm:text-5xl">
              进入节点观测与治理控制面
            </h1>
          </div>
          <p className="border-l border-accent/50 pl-4 text-sm leading-6 text-muted-foreground">
            管理面仅支持账户与密码登录。会话 Cookie 为 HttpOnly；订阅 Token 只能用于节点订阅，不能登录控制台。
          </p>
        </div>

        <Card className="mx-auto max-w-xl border-primary/25 bg-card">
          <CardContent className="p-6">
            <div className="mb-6 flex items-center gap-3">
              <span className="corner-ticks flex size-10 items-center justify-center border border-primary/40 bg-primary/10">
                <LockKeyhole className="size-4 text-primary" />
              </span>
              <div>
                <h2 className="text-sm font-semibold text-foreground">账户登录</h2>
                <p className="text-xs text-muted-foreground">租户隔离 · RBAC 会话</p>
              </div>
            </div>
            <form onSubmit={login} className="space-y-4">
              <Field label="租户" htmlFor="login-tenant">
                <Input
                  id="login-tenant"
                  value={tenant}
                  onChange={(e) => setTenant(e.target.value)}
                  required
                />
              </Field>
              <Field label="用户名" htmlFor="login-username">
                <Input
                  id="login-username"
                  autoComplete="username"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  required
                />
              </Field>
              <Field label="密码" htmlFor="login-password">
                <Input
                  id="login-password"
                  type="password"
                  autoComplete="current-password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                />
              </Field>
              <Button className="w-full" type="submit" disabled={busy || !local}>
                <ShieldCheck className="size-4" />
                {busy ? "验证中…" : local ? "登录控制台" : "本地登录未启用"}
              </Button>
            </form>
          </CardContent>
        </Card>
        {error && (
          <Alert variant="danger" className="mt-4">
            <AlertTriangle />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
      </div>
    </div>
  );
}
