"use client";

import type { FormEvent } from "react";
import { useCallback, useEffect, useState } from "react";
import { Copy, Plus, RefreshCw } from "lucide-react";
import { AuthRequired } from "@/components/auth-required";
import { useLiveRefresh } from "@/components/live-provider";
import { PageHeader } from "@/components/page-header";
import { PaginationControls } from "@/components/pagination-controls";
import { useSession } from "@/components/session-provider";
import { StatusBadge } from "@/components/status-badge";
import { TableEmpty } from "@/components/table-empty";
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { api, errorMessage, type TokenRecord } from "@/lib/api";
import { formatBytes, formatTime } from "@/lib/utils";

export default function TokensPage() {
  const { authenticated, loading, canAdmin } = useSession();
  const [tokens, setTokens] = useState<TokenRecord[]>([]);
  const [created, setCreated] = useState<TokenRecord | null>(null);
  const [confirmDelete, setConfirmDelete] = useState("");
  const [form, setForm] = useState({
    name: "",
    note: "",
    countries: "",
    protocols: "",
    days: "90",
    max_rps: "2",
    daily_quota: "1000",
  });
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [total, setTotal] = useState(0);
  const [nextCursor, setNextCursor] = useState("");
  const [cursorStack, setCursorStack] = useState<string[]>([]);

  const load = useCallback(async (cursor = "") => {
    if (!canAdmin) return;
    try {
      const page = await api.tokensPage({ cursor: cursor || undefined });
      setTokens(page.tokens);
      setTotal(page.total ?? 0);
      setNextCursor(page.next_cursor || "");
      setError("");
    } catch (cause) {
      setError(errorMessage(cause, "加载 Token 失败"));
    }
  }, [canAdmin]);

  useEffect(() => {
    const initial = window.setTimeout(() => {
      setCursorStack([]);
      void load();
    }, 0);
    return () => window.clearTimeout(initial);
  }, [load]);

  const currentCursor = cursorStack[cursorStack.length - 1] || "";
  useLiveRefresh(() => load(currentCursor), canAdmin);

  function nextPage() {
    if (!nextCursor) return;
    setCursorStack((current) => [...current, nextCursor]);
    void load(nextCursor);
  }

  function previousPage() {
    const previous = cursorStack.slice(0, -1);
    setCursorStack(previous);
    void load(previous[previous.length - 1] || "");
  }

  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy("create");
    try {
      const token = await api.createToken({
        name: form.name,
        note: form.note,
        countries: form.countries.split(/[\s,]+/).filter(Boolean),
        protocols: form.protocols.split(/[\s,]+/).filter(Boolean),
        days: Number(form.days),
        max_rps: Number(form.max_rps),
        daily_quota: Number(form.daily_quota),
      });
      setCreated(token);
      setForm({ ...form, name: "", note: "" });
      await load(currentCursor);
    } catch (cause) {
      setError(errorMessage(cause, "创建 Token 失败"));
    } finally {
      setBusy("");
    }
  }

  async function toggle(token: TokenRecord) {
    setBusy(token.id);
    try {
      await api.setTokenEnabled(token.id, !token.enabled);
      await load(currentCursor);
    } catch (cause) {
      setError(errorMessage(cause, "更新 Token 失败"));
    } finally {
      setBusy("");
    }
  }

  async function remove(token: TokenRecord) {
    if (confirmDelete !== token.id) {
      setConfirmDelete(token.id);
      return;
    }
    setBusy(token.id);
    try {
      await api.deleteToken(token.id);
      setConfirmDelete("");
      await load(currentCursor);
    } catch (cause) {
      setError(errorMessage(cause, "删除 Token 失败"));
    } finally {
      setBusy("");
    }
  }

  async function copy() {
    if (!created?.token) return;
    try {
      await navigator.clipboard.writeText(created.token);
    } catch {
      setError("无法复制 Token。");
    }
  }

  if (!authenticated && !loading) return <AuthRequired />;
  if (authenticated && !canAdmin) return <AuthRequired reason="forbidden" />;

  return (
    <>
      <PageHeader
        title="订阅 Token"
        description={"创建并管理租户隔离的订阅凭证，共 " + total + " 个。"}
        actions={
          <Button type="button" variant="outline" onClick={() => void load(currentCursor)}>
            <RefreshCw />
            刷新
          </Button>
        }
      />
      {error && (
        <Alert variant="destructive">
          <AlertTitle>操作失败</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      {created?.token && (
        <Card>
          <CardHeader>
            <CardTitle>新 Token</CardTitle>
            <CardDescription>明文仅在创建后展示，请立即保存。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <code className="block break-all rounded-md bg-muted p-4 text-sm">
              {created.token}
            </code>
            <div className="flex flex-wrap gap-2">
              <Button type="button" variant="outline" onClick={() => void copy()}>
                <Copy />
                复制
              </Button>
              <Button type="button" variant="ghost" onClick={() => setCreated(null)}>
                我已保存
              </Button>
            </div>
          </CardContent>
        </Card>
      )}
      <Card>
        <CardHeader>
          <CardTitle>新建 Token</CardTitle>
          <CardDescription>国家和协议用逗号或空格分隔；留空表示不限制。</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="space-y-4" onSubmit={create}>
            <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
              <div className="space-y-2">
                <Label htmlFor="token-name">名称</Label>
                <Input
                  id="token-name"
                  value={form.name}
                  onChange={(event) => setForm({ ...form, name: event.target.value })}
                  required
                  maxLength={100}
                />
              </div>
              <div className="space-y-2 md:col-span-1 xl:col-span-3">
                <Label htmlFor="token-note">备注</Label>
                <Input
                  id="token-note"
                  value={form.note}
                  onChange={(event) => setForm({ ...form, note: event.target.value })}
                  maxLength={1000}
                />
              </div>
              <div className="space-y-2 md:col-span-2">
                <Label htmlFor="token-countries">允许国家</Label>
                <Input
                  id="token-countries"
                  value={form.countries}
                  placeholder="US, JP, SG"
                  onChange={(event) => setForm({ ...form, countries: event.target.value })}
                />
              </div>
              <div className="space-y-2 md:col-span-2">
                <Label htmlFor="token-protocols">允许协议</Label>
                <Input
                  id="token-protocols"
                  value={form.protocols}
                  placeholder="vless, trojan"
                  onChange={(event) => setForm({ ...form, protocols: event.target.value })}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="token-days">有效天数</Label>
                <Input
                  id="token-days"
                  type="number"
                  min="0"
                  max="3650"
                  value={form.days}
                  onChange={(event) => setForm({ ...form, days: event.target.value })}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="token-rps">最大 RPS</Label>
                <Input
                  id="token-rps"
                  type="number"
                  min="0"
                  max="1000"
                  step="0.1"
                  value={form.max_rps}
                  onChange={(event) => setForm({ ...form, max_rps: event.target.value })}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="token-quota">日配额</Label>
                <Input
                  id="token-quota"
                  type="number"
                  min="0"
                  value={form.daily_quota}
                  onChange={(event) => setForm({ ...form, daily_quota: event.target.value })}
                />
              </div>
            </div>
            <Button type="submit" disabled={busy === "create"}>
              <Plus />
              {busy === "create" ? "创建中…" : "创建 Token"}
            </Button>
          </form>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Token 列表</CardTitle>
          <CardDescription>明文不会再次显示；启停操作立即生效。</CardDescription>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>名称</TableHead>
                <TableHead>前缀</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>限制</TableHead>
                <TableHead>今日用量</TableHead>
                <TableHead>到期</TableHead>
                <TableHead>操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {tokens.map((token) => (
                <TableRow key={token.id}>
                  <TableCell className="font-medium">{token.name}</TableCell>
                  <TableCell className="font-mono text-xs">{token.token_prefix}</TableCell>
                  <TableCell>
                    <StatusBadge status={token.enabled ? "enabled" : "disabled"}>
                      {token.enabled ? "启用" : "停用"}
                    </StatusBadge>
                  </TableCell>
                  <TableCell className="max-w-sm whitespace-normal">
                    {(token.allow_countries?.join(", ") || "全部国家") +
                      " · " +
                      (token.allow_protocols?.join(", ") || "全部协议")}
                  </TableCell>
                  <TableCell className="tabular-nums">
                    {token.requests_today +
                      " 请求 · " +
                      formatBytes(token.bytes_today) +
                      (token.daily_quota ? " / " + token.daily_quota : "")}
                  </TableCell>
                  <TableCell>{formatTime(token.expires_at)}</TableCell>
                  <TableCell>
                    <div className="flex gap-2">
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        disabled={busy === token.id}
                        onClick={() => void toggle(token)}
                      >
                        {token.enabled ? "停用" : "启用"}
                      </Button>
                      <Button
                        type="button"
                        size="sm"
                        variant="destructive"
                        disabled={busy === token.id}
                        onClick={() => void remove(token)}
                      >
                        {confirmDelete === token.id ? "再次确认" : "删除"}
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
              {!tokens.length && (
                <TableEmpty colSpan={7}>暂无 Token。</TableEmpty>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      <PaginationControls
        page={cursorStack.length + 1}
        total={total}
        count={tokens.length}
        hasNext={Boolean(nextCursor)}
        onPrevious={previousPage}
        onNext={nextPage}
      />
    </>
  );
}
