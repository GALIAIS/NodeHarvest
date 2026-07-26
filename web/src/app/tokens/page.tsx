"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { AlertTriangle, Ban, Check, Copy, KeyRound, Plus, Power, Trash2 } from "lucide-react";
import { api, errorMessage, isAuthError, type TokenRecord } from "@/lib/api";
import { AuthRequired } from "@/components/auth-required";
import { PageHeader } from "@/components/page-header";
import { useSession } from "@/components/session-provider";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardEmpty, CardHeader, CardTitle } from "@/components/ui/card";
import { CheckboxChip } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Field } from "@/components/ui/label";
import { Progress } from "@/components/ui/progress";
import { formatBytes, formatTime } from "@/lib/utils";

const protocols = ["vmess", "vless", "trojan", "ss", "ssr", "hysteria2", "tuic"];

export default function TokensPage() {
  const [tokens, setTokens] = useState<TokenRecord[]>([]);
  const [created, setCreated] = useState<TokenRecord | null>(null);
  const [form, setForm] = useState({
    name: "",
    note: "",
    countries: "",
    protocols: [] as string[],
    days: "90",
    max_rps: "2",
    daily_quota: "1000",
  });
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  // 登录态失效(401/403)属于正常状态,单独记录以渲染 AuthRequired 而非报错横幅
  const [authError, setAuthError] = useState(false);
  const [copied, setCopied] = useState(false);
  const { authenticated, loading: sessionLoading } = useSession();
  // open 与 target 分离:关闭时仅收起对话框,target 保留到下次打开,
  // 这样退出动画期间文案不会闪空
  const [deleteTarget, setDeleteTarget] = useState<TokenRecord | null>(null);
  const [deleteOpen, setDeleteOpen] = useState(false);

  const load = useCallback(async () => {
    // 管理面接口,匿名访客不发请求,避免无意义的 401
    if (!authenticated) return;
    try {
      setTokens(await api.tokens());
      setError("");
      setAuthError(false);
    } catch (cause) {
      if (isAuthError(cause)) setAuthError(true);
      else setError(errorMessage(cause, "加载失败"));
    }
  }, [authenticated]);

  useEffect(() => {
    const initial = setTimeout(load, 0);
    return () => clearTimeout(initial);
  }, [load]);

  async function create(event: FormEvent) {
    event.preventDefault();
    setBusy("create");
    setError("");
    try {
      const token = await api.createToken({
        name: form.name,
        note: form.note,
        countries: form.countries.split(/[\s,]+/).filter(Boolean),
        protocols: form.protocols,
        days: Number(form.days),
        max_rps: Number(form.max_rps),
        daily_quota: Number(form.daily_quota),
      });
      setCreated(token);
      setForm((current) => ({ ...current, name: "", note: "" }));
      await load();
    } catch (cause) {
      if (isAuthError(cause)) setAuthError(true);
      else setError(errorMessage(cause, "创建失败"));
    } finally {
      setBusy("");
    }
  }

  async function toggle(token: TokenRecord) {
    setBusy(token.id);
    try {
      await api.setTokenEnabled(token.id, !token.enabled);
      await load();
    } catch (cause) {
      if (isAuthError(cause)) setAuthError(true);
      else setError(errorMessage(cause, "操作失败"));
    } finally {
      setBusy("");
    }
  }

  async function remove(token: TokenRecord) {
    setBusy(token.id);
    try {
      await api.deleteToken(token.id);
      await load();
    } catch (cause) {
      if (isAuthError(cause)) setAuthError(true);
      else setError(errorMessage(cause, "删除失败"));
    } finally {
      setBusy("");
      setDeleteOpen(false);
    }
  }

  async function copyPlain() {
    if (!created?.token) return;
    await navigator.clipboard.writeText(created.token);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }

  // 会话仍在探测时不渲染登录提示,避免每次刷新闪现
  const needsLogin = (!authenticated && !sessionLoading) || authError;

  return (
    <div className="flex flex-1 flex-col">
      <PageHeader
        eyebrow="Credential governance"
        title="订阅 Token"
        description="创建、吊销、到期、按日配额、请求速率以及国家/协议 ACL。明文 Token 只在创建时展示一次。"
      />
      <div className="reveal space-y-4 p-4 sm:p-6 lg:p-8">
        {created?.token && (
          <Card className="border-accent/40 bg-accent/5">
            <CardHeader>
              <CardTitle className="text-accent">立即保存新 Token</CardTitle>
              <CardDescription>离开页面后无法再次读取明文；遗失时请吊销并重新创建。</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex flex-col gap-2 sm:flex-row">
                <code className="min-w-0 flex-1 overflow-x-auto border border-accent/30 bg-popover px-3 py-2.5 font-mono text-xs text-accent">
                  {created.token}
                </code>
                <Button variant="accent" onClick={copyPlain}>
                  {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
                  {copied ? "已复制" : "复制"}
                </Button>
                <Button variant="ghost" onClick={() => setCreated(null)}>我已保存</Button>
              </div>
            </CardContent>
          </Card>
        )}

        {error && (
          <Alert variant="danger">
            <AlertTriangle />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {needsLogin ? (
          <AuthRequired
            title="凭证管理需要登录"
            description="订阅 Token 的创建、吊销与配额属于管理面，登录后可管理。"
          />
        ) : (
        <div className="grid gap-4 xl:grid-cols-[minmax(340px,.65fr)_minmax(0,1.35fr)]">
          <Card className="h-fit">
            <CardHeader>
              <CardTitle className="flex items-center gap-2"><Plus className="size-4 text-primary" /> 新建凭证</CardTitle>
              <CardDescription>国家使用 ISO 两位代码；留空 ACL 表示不限制。</CardDescription>
            </CardHeader>
            <CardContent>
              <form className="space-y-4" onSubmit={create}>
                <Field label="名称" htmlFor="token-name">
                  <Input id="token-name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required maxLength={100} placeholder="例如：上海研发组" />
                </Field>
                <Field label="备注" htmlFor="token-note">
                  <Input id="token-note" value={form.note} onChange={(e) => setForm({ ...form, note: e.target.value })} maxLength={1000} placeholder="用途与负责人" />
                </Field>
                <Field label="允许国家" htmlFor="token-countries">
                  <Input id="token-countries" className="font-mono uppercase" value={form.countries} onChange={(e) => setForm({ ...form, countries: e.target.value })} placeholder="US, JP, SG" />
                </Field>
                <fieldset>
                  <legend className="mb-2 text-xs leading-none font-medium text-muted-foreground">允许协议</legend>
                  <div className="flex flex-wrap gap-2">
                    {protocols.map((protocol) => (
                      <CheckboxChip
                        key={protocol}
                        className="h-8 px-2 font-mono text-[10px]"
                        checked={form.protocols.includes(protocol)}
                        onCheckedChange={(state) =>
                          setForm({
                            ...form,
                            protocols: state === true
                              ? [...form.protocols, protocol]
                              : form.protocols.filter((item) => item !== protocol),
                          })
                        }
                      >
                        {protocol}
                      </CheckboxChip>
                    ))}
                  </div>
                </fieldset>
                <div className="grid grid-cols-3 gap-2">
                  <Field label="有效天数" htmlFor="token-days">
                    <Input id="token-days" type="number" min="0" max="3650" value={form.days} onChange={(e) => setForm({ ...form, days: e.target.value })} />
                  </Field>
                  <Field label="最大 RPS" htmlFor="token-rps">
                    <Input id="token-rps" type="number" min="0" max="1000" step="0.1" value={form.max_rps} onChange={(e) => setForm({ ...form, max_rps: e.target.value })} />
                  </Field>
                  <Field label="日配额" htmlFor="token-quota">
                    <Input id="token-quota" type="number" min="0" value={form.daily_quota} onChange={(e) => setForm({ ...form, daily_quota: e.target.value })} />
                  </Field>
                </div>
                <Button className="w-full" type="submit" disabled={busy === "create"}>
                  <KeyRound className="size-4" /> {busy === "create" ? "生成中…" : "生成 Token"}
                </Button>
              </form>
            </CardContent>
          </Card>

          <div className="space-y-3">
            {tokens.map((token) => {
              const usage = token.daily_quota > 0 ? Math.min(100, (token.requests_today / token.daily_quota) * 100) : 0;
              const expired = Boolean(token.expires_at && new Date(token.expires_at) < new Date());
              return (
                <Card key={token.id} className={!token.enabled || expired ? "opacity-70" : ""}>
                  <CardContent className="p-4">
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div>
                        <div className="flex flex-wrap items-center gap-2">
                          <h2 className="text-sm font-semibold text-foreground">{token.name}</h2>
                          <Badge variant={expired ? "danger" : token.enabled ? "success" : "secondary"}>
                            {expired ? "EXPIRED" : token.enabled ? "ACTIVE" : "DISABLED"}
                          </Badge>
                          <code className="font-mono text-[10px] text-muted-foreground">{token.token_prefix}••••</code>
                        </div>
                        <p className="mt-1 text-xs text-muted-foreground">{token.note || "无备注"}</p>
                      </div>
                      <div className="flex gap-2">
                        <Button size="sm" variant={token.enabled ? "secondary" : "outline"} disabled={busy === token.id} onClick={() => toggle(token)}>
                          {token.enabled ? <Ban className="size-3.5" /> : <Power className="size-3.5" />}
                          {token.enabled ? "停用" : "启用"}
                        </Button>
                        <Button size="icon-sm" variant="destructive" aria-label="删除 Token" disabled={busy === token.id} onClick={() => { setDeleteTarget(token); setDeleteOpen(true); }}>
                          <Trash2 className="size-3.5" />
                        </Button>
                      </div>
                    </div>
                    <div className="mt-4 grid gap-3 sm:grid-cols-[1fr_auto]">
                      <div>
                        <div className="mb-1.5 flex justify-between font-mono text-[10px] tabular-nums text-muted-foreground">
                          <span>今日请求 {token.requests_today}</span>
                          <span>{token.daily_quota > 0 ? `/ ${token.daily_quota}` : "不限额"}</span>
                        </div>
                        {token.daily_quota > 0 ? <Progress value={usage} /> : <div className="h-1.5 border border-border bg-muted" />}
                      </div>
                      <p className="text-xs text-muted-foreground">流量 {formatBytes(token.bytes_today)} · {token.max_rps || "∞"} RPS</p>
                    </div>
                    <div className="mt-4 flex flex-wrap gap-x-5 gap-y-1 border-t border-border pt-3 font-mono text-[10px] text-muted-foreground">
                      <span>国家 {token.allow_countries?.join(", ") || "ALL"}</span>
                      <span>协议 {token.allow_protocols?.join(", ") || "ALL"}</span>
                      <span>到期 {formatTime(token.expires_at)}</span>
                      <span>最近使用 {formatTime(token.last_used_at)}</span>
                    </div>
                  </CardContent>
                </Card>
              );
            })}
            {tokens.length === 0 && <Card><CardEmpty>暂无数据库 Token</CardEmpty></Card>}
          </div>
        </div>
        )}
      </div>

      <Dialog
        open={deleteOpen}
        onOpenChange={(open) => {
          // 删除请求进行中时禁止关闭弹窗，避免中途丢失上下文
          if (!open && busy !== deleteTarget?.id) setDeleteOpen(false);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>删除 Token</DialogTitle>
            <DialogDescription>
              Token「{deleteTarget?.name}」将被永久删除，使用它的客户端将立即失效。此操作不可撤销。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="secondary" disabled={busy === deleteTarget?.id} onClick={() => setDeleteOpen(false)}>取消</Button>
            <Button
              variant="destructive"
              disabled={!deleteTarget || busy === deleteTarget.id}
              onClick={() => deleteTarget && remove(deleteTarget)}
            >
              {deleteTarget && busy === deleteTarget.id ? "删除中…" : "确认删除"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
