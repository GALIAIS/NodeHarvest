"use client";

import type { FormEvent } from "react";
import { useCallback, useEffect, useState } from "react";
import { AuthRequired } from "@/components/auth-required";
import { useLiveRefresh } from "@/components/live-provider";
import { PaginationControls } from "@/components/pagination-controls";
import { useSession } from "@/components/session-provider";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
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
import { api, errorMessage, type UserRecord } from "@/lib/api";
import { formatTime } from "@/lib/utils";

export default function UsersPage() {
  const { authenticated, loading, canAdmin } = useSession();
  const [users, setUsers] = useState<UserRecord[]>([]);
  const [form, setForm] = useState({ username: "", email: "", password: "", role: "viewer" });
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [total, setTotal] = useState(0);
  const [nextCursor, setNextCursor] = useState("");
  const [cursorStack, setCursorStack] = useState<string[]>([]);

  const load = useCallback(async (cursor = "") => {
    if (!canAdmin) return;
    try {
      const page = await api.usersPage({ cursor: cursor || undefined });
      setUsers(page.users);
      setTotal(page.total ?? 0);
      setNextCursor(page.next_cursor || "");
      setError("");
    } catch (cause) {
      setError(errorMessage(cause, "加载用户失败"));
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
      await api.createUser(form);
      setForm({ username: "", email: "", password: "", role: "viewer" });
      await load(currentCursor);
    } catch (cause) {
      setError(errorMessage(cause, "创建用户失败"));
    } finally {
      setBusy("");
    }
  }

  async function toggle(user: UserRecord) {
    setBusy(user.id);
    try {
      await api.setUserEnabled(user.id, !user.enabled);
      await load(currentCursor);
    } catch (cause) {
      setError(errorMessage(cause, "更新用户失败"));
    } finally {
      setBusy("");
    }
  }

  if (!authenticated && !loading) return <AuthRequired />;
  if (authenticated && !canAdmin) return <AuthRequired reason="forbidden" />;

  return (
    <>
      <header>
        <h1>用户</h1>
        <p>管理本地用户与角色。</p>
      </header>
      {error && (
        <Alert variant="destructive">
          <AlertTitle>操作失败</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <Card>
        <CardHeader>
          <CardTitle>新建用户</CardTitle>
          <CardDescription>密码至少为 12 个字符。</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={create}>
            <Label htmlFor="user-name">用户名</Label>
            <Input
              id="user-name"
              value={form.username}
              onChange={(event) => setForm({ ...form, username: event.target.value })}
              required
            />
            <Label htmlFor="user-email">邮箱</Label>
            <Input
              id="user-email"
              type="email"
              value={form.email}
              onChange={(event) => setForm({ ...form, email: event.target.value })}
            />
            <Label htmlFor="user-password">初始密码</Label>
            <Input
              id="user-password"
              type="password"
              minLength={12}
              value={form.password}
              onChange={(event) => setForm({ ...form, password: event.target.value })}
              required
            />
            <p>角色</p>
            {(["viewer", "operator", "admin"] as const).map((role) => (
              <Button
                key={role}
                type="button"
                variant={form.role === role ? "secondary" : "outline"}
                aria-pressed={form.role === role}
                onClick={() => setForm({ ...form, role })}
              >
                {role}
              </Button>
            ))}
            <Button type="submit" disabled={busy === "create"}>
              {busy === "create" ? "创建中…" : "创建用户"}
            </Button>
          </form>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>用户列表</CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>用户名</TableHead>
                <TableHead>邮箱</TableHead>
                <TableHead>角色</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>最近登录</TableHead>
                <TableHead>创建时间</TableHead>
                <TableHead>操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {users.map((user) => (
                <TableRow key={user.id}>
                  <TableCell>{user.username}</TableCell>
                  <TableCell>{user.email || "—"}</TableCell>
                  <TableCell>{user.role}</TableCell>
                  <TableCell>
                    <Badge variant={user.enabled ? "default" : "secondary"}>
                      {user.enabled ? "启用" : "停用"}
                    </Badge>
                  </TableCell>
                  <TableCell>{formatTime(user.last_login_at)}</TableCell>
                  <TableCell>{formatTime(user.created_at)}</TableCell>
                  <TableCell>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      disabled={busy === user.id}
                      onClick={() => void toggle(user)}
                    >
                      {user.enabled ? "停用" : "启用"}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
              {!users.length && (
                <TableRow>
                  <TableCell colSpan={7}>暂无用户。</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      <PaginationControls
        page={cursorStack.length + 1}
        total={total}
        count={users.length}
        hasNext={Boolean(nextCursor)}
        onPrevious={previousPage}
        onNext={nextPage}
      />
    </>
  );
}
