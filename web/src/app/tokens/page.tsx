"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { Ban, Check, Copy, KeyRound, Plus, Power, Trash2 } from "lucide-react";
import { api, type TokenRecord } from "@/lib/api";
import { PageHeader } from "@/components/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
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
  const [copied, setCopied] = useState(false);

  const load = useCallback(async () => {
    try {
      setTokens(await api.tokens());
      setError("");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "加载失败");
    }
  }, []);

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
      setError(cause instanceof Error ? cause.message : "创建失败");
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
      setError(cause instanceof Error ? cause.message : "操作失败");
    } finally {
      setBusy("");
    }
  }

  async function remove(token: TokenRecord) {
    if (!window.confirm(`永久删除 Token「${token.name}」？客户端将立即失效。`)) return;
    setBusy(token.id);
    try {
      await api.deleteToken(token.id);
      await load();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "删除失败");
    } finally {
      setBusy("");
    }
  }

  async function copyPlain() {
    if (!created?.token) return;
    await navigator.clipboard.writeText(created.token);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }

  return (
    <div className="flex flex-1 flex-col">
      <PageHeader
        eyebrow="Credential governance"
        title="订阅 Token"
        description="创建、吊销、到期、按日配额、请求速率以及国家/协议 ACL。明文 Token 只在创建时展示一次。"
      />
      <div className="reveal space-y-5 p-4 sm:p-6 lg:p-8">
        {created?.token && (
          <Card className="border-amber-400/35 bg-amber-400/[0.04]">
            <CardHeader>
              <CardTitle className="text-amber-200">立即保存新 Token</CardTitle>
              <CardDescription>离开页面后无法再次读取明文；遗失时请吊销并重新创建。</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex flex-col gap-2 sm:flex-row">
                <code className="min-w-0 flex-1 overflow-x-auto rounded-md border border-amber-500/20 bg-slate-950 px-3 py-2.5 font-mono text-xs text-amber-100">
                  {created.token}
                </code>
                <Button variant="amber" onClick={copyPlain}>
                  {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
                  {copied ? "已复制" : "复制"}
                </Button>
                <Button variant="ghost" onClick={() => setCreated(null)}>我已保存</Button>
              </div>
            </CardContent>
          </Card>
        )}

        {error && (
          <div role="alert" className="rounded-md border border-rose-500/30 bg-rose-500/10 px-4 py-3 text-sm text-rose-300">{error}</div>
        )}

        <div className="grid gap-4 xl:grid-cols-[minmax(340px,.65fr)_minmax(0,1.35fr)]">
          <Card className="h-fit">
            <CardHeader>
              <CardTitle className="flex items-center gap-2"><Plus className="h-4 w-4 text-cyan-300" /> 新建凭证</CardTitle>
              <CardDescription>国家使用 ISO 两位代码；留空 ACL 表示不限制。</CardDescription>
            </CardHeader>
            <CardContent>
              <form className="space-y-4" onSubmit={create}>
                <label className="block text-xs text-slate-500">
                  名称
                  <Input className="mt-1.5" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required maxLength={100} placeholder="例如：上海研发组" />
                </label>
                <label className="block text-xs text-slate-500">
                  备注
                  <Input className="mt-1.5" value={form.note} onChange={(e) => setForm({ ...form, note: e.target.value })} maxLength={1000} placeholder="用途与负责人" />
                </label>
                <label className="block text-xs text-slate-500">
                  允许国家
                  <Input className="mt-1.5 font-mono uppercase" value={form.countries} onChange={(e) => setForm({ ...form, countries: e.target.value })} placeholder="US, JP, SG" />
                </label>
                <fieldset>
                  <legend className="mb-2 text-xs text-slate-500">允许协议</legend>
                  <div className="flex flex-wrap gap-2">
                    {protocols.map((protocol) => (
                      <label key={protocol} className="flex items-center gap-1.5 rounded-md border border-slate-800 bg-slate-950/50 px-2 py-1.5 font-mono text-[10px] text-slate-400">
                        <input
                          type="checkbox"
                          className="accent-cyan-400"
                          checked={form.protocols.includes(protocol)}
                          onChange={(event) =>
                            setForm({
                              ...form,
                              protocols: event.target.checked
                                ? [...form.protocols, protocol]
                                : form.protocols.filter((item) => item !== protocol),
                            })
                          }
                        />
                        {protocol}
                      </label>
                    ))}
                  </div>
                </fieldset>
                <div className="grid grid-cols-3 gap-2">
                  <label className="text-xs text-slate-500">有效天数<Input className="mt-1.5" type="number" min="0" max="3650" value={form.days} onChange={(e) => setForm({ ...form, days: e.target.value })} /></label>
                  <label className="text-xs text-slate-500">最大 RPS<Input className="mt-1.5" type="number" min="0" max="1000" step="0.1" value={form.max_rps} onChange={(e) => setForm({ ...form, max_rps: e.target.value })} /></label>
                  <label className="text-xs text-slate-500">日配额<Input className="mt-1.5" type="number" min="0" value={form.daily_quota} onChange={(e) => setForm({ ...form, daily_quota: e.target.value })} /></label>
                </div>
                <Button className="w-full" type="submit" disabled={busy === "create"}>
                  <KeyRound className="h-4 w-4" /> {busy === "create" ? "生成中…" : "生成 Token"}
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
                          <h2 className="text-sm font-semibold text-slate-200">{token.name}</h2>
                          <Badge variant={expired ? "danger" : token.enabled ? "success" : "secondary"}>
                            {expired ? "EXPIRED" : token.enabled ? "ACTIVE" : "DISABLED"}
                          </Badge>
                          <code className="font-mono text-[10px] text-slate-600">{token.token_prefix}••••</code>
                        </div>
                        <p className="mt-1 text-xs text-slate-600">{token.note || "无备注"}</p>
                      </div>
                      <div className="flex gap-2">
                        <Button size="sm" variant={token.enabled ? "secondary" : "outline"} disabled={busy === token.id} onClick={() => toggle(token)}>
                          {token.enabled ? <Ban className="h-3.5 w-3.5" /> : <Power className="h-3.5 w-3.5" />}
                          {token.enabled ? "停用" : "启用"}
                        </Button>
                        <Button size="sm" variant="danger" disabled={busy === token.id} onClick={() => remove(token)}>
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      </div>
                    </div>
                    <div className="mt-4 grid gap-3 sm:grid-cols-[1fr_auto]">
                      <div>
                        <div className="mb-1.5 flex justify-between font-mono text-[10px] text-slate-600">
                          <span>今日请求 {token.requests_today}</span>
                          <span>{token.daily_quota > 0 ? `/ ${token.daily_quota}` : "不限额"}</span>
                        </div>
                        {token.daily_quota > 0 ? <Progress value={usage} /> : <div className="h-2 rounded-full bg-slate-800" />}
                      </div>
                      <p className="text-xs text-slate-500">流量 {formatBytes(token.bytes_today)} · {token.max_rps || "∞"} RPS</p>
                    </div>
                    <div className="mt-4 flex flex-wrap gap-x-5 gap-y-1 border-t border-slate-800 pt-3 font-mono text-[10px] text-slate-600">
                      <span>国家 {token.allow_countries?.join(", ") || "ALL"}</span>
                      <span>协议 {token.allow_protocols?.join(", ") || "ALL"}</span>
                      <span>到期 {formatTime(token.expires_at)}</span>
                      <span>最近使用 {formatTime(token.last_used_at)}</span>
                    </div>
                  </CardContent>
                </Card>
              );
            })}
            {tokens.length === 0 && <Card><CardContent className="py-16 text-center text-sm text-slate-600">暂无数据库 Token</CardContent></Card>}
          </div>
        </div>
      </div>
    </div>
  );
}
