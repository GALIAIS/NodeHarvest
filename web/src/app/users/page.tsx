"use client";

import type { FormEvent } from "react";
import { useCallback, useEffect, useState } from "react";
import { Plus, RefreshCw } from "lucide-react";
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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
      <PageHeader
        title="用户"
        description={"管理本地账户、角色与启用状态，共 " + total + " 个用户。"}
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
      <Card>
        <CardHeader>
          <CardTitle>新建用户</CardTitle>
          <CardDescription>密码至少为 12 个字符。</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="space-y-4" onSubmit={create}>
            <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
              <div className="space-y-2">
                <Label htmlFor="user-name">用户名</Label>
                <Input
                  id="user-name"
                  value={form.username}
                  onChange={(event) => setForm({ ...form, username: event.target.value })}
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="user-email">邮箱</Label>
                <Input
                  id="user-email"
                  type="email"
                  value={form.email}
                  onChange={(event) => setForm({ ...form, email: event.target.value })}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="user-password">初始密码</Label>
                <Input
                  id="user-password"
                  type="password"
                  minLength={12}
                  value={form.password}
                  onChange={(event) => setForm({ ...form, password: event.target.value })}
                  required
                  autoComplete="new-password"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="user-role">角色</Label>
                <Select
                  value={form.role}
                  onValueChange={(role) => setForm({ ...form, role })}
                >
                  <SelectTrigger id="user-role" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="viewer">viewer</SelectItem>
                    <SelectItem value="operator">operator</SelectItem>
                    <SelectItem value="admin">admin</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
            <Button type="submit" disabled={busy === "create"}>
              <Plus />
              {busy === "create" ? "创建中…" : "创建用户"}
            </Button>
          </form>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>用户列表</CardTitle>
          <CardDescription>启停账号不会删除历史审计记录。</CardDescription>
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
                  <TableCell className="font-medium">{user.username}</TableCell>
                  <TableCell>{user.email || "—"}</TableCell>
                  <TableCell><StatusBadge status={user.role}>{user.role}</StatusBadge></TableCell>
                  <TableCell>
                    <StatusBadge status={user.enabled ? "enabled" : "disabled"}>
                      {user.enabled ? "启用" : "停用"}
                    </StatusBadge>
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
                <TableEmpty colSpan={7}>暂无用户。</TableEmpty>
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
