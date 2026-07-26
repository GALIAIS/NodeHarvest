"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { AlertTriangle, Ban, Plus, Power, Shield, UserRound } from "lucide-react";
import { api, type UserRecord } from "@/lib/api";
import { PageHeader } from "@/components/page-header";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Field } from "@/components/ui/label";
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
  TableEmpty,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { formatTime } from "@/lib/utils";

export default function UsersPage() {
  const [users, setUsers] = useState<UserRecord[]>([]);
  const [form, setForm] = useState({ username: "", email: "", password: "", role: "viewer" });
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    try {
      setUsers(await api.users());
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
    try {
      await api.createUser(form);
      setForm({ username: "", email: "", password: "", role: "viewer" });
      await load();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "创建失败");
    } finally {
      setBusy("");
    }
  }

  async function toggle(user: UserRecord) {
    setBusy(user.id);
    try {
      await api.setUserEnabled(user.id, !user.enabled);
      await load();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "操作失败");
    } finally {
      setBusy("");
    }
  }

  return (
    <div className="flex flex-1 flex-col">
      <PageHeader
        eyebrow="Tenant access control"
        title="用户与角色"
        description="当前租户内的本地与 OIDC 身份。viewer 只读，operator 可运行任务，admin 可管理用户和凭证。"
      />
      <div className="reveal space-y-4 p-4 sm:p-6 lg:p-8">
        {error && (
          <Alert variant="danger">
            <AlertTriangle />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        <div className="grid gap-4 xl:grid-cols-[minmax(320px,.55fr)_minmax(0,1.45fr)]">
          <Card className="h-fit">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Plus className="size-4 text-primary" /> 新建本地用户
              </CardTitle>
              <CardDescription>密码使用 bcrypt 哈希保存，不会通过管理 API 返回。</CardDescription>
            </CardHeader>
            <CardContent>
              <form onSubmit={create} className="space-y-4">
                <Field label="用户名" htmlFor="user-username">
                  <Input
                    id="user-username"
                    value={form.username}
                    onChange={(e) => setForm({ ...form, username: e.target.value })}
                    required
                  />
                </Field>
                <Field label="邮箱" htmlFor="user-email">
                  <Input
                    id="user-email"
                    type="email"
                    value={form.email}
                    onChange={(e) => setForm({ ...form, email: e.target.value })}
                  />
                </Field>
                <Field label="初始密码" htmlFor="user-password">
                  <Input
                    id="user-password"
                    type="password"
                    autoComplete="new-password"
                    minLength={12}
                    value={form.password}
                    onChange={(e) => setForm({ ...form, password: e.target.value })}
                    required
                  />
                </Field>
                <Field label="角色">
                  <Select
                    value={form.role}
                    onValueChange={(value) => setForm({ ...form, role: value })}
                  >
                    <SelectTrigger className="w-full" aria-label="角色">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="viewer">viewer · 查看</SelectItem>
                      <SelectItem value="operator">operator · 操作</SelectItem>
                      <SelectItem value="admin">admin · 管理</SelectItem>
                    </SelectContent>
                  </Select>
                </Field>
                <Button type="submit" className="w-full" disabled={busy === "create"}>
                  <UserRound className="size-4" /> {busy === "create" ? "创建中…" : "创建用户"}
                </Button>
              </form>
            </CardContent>
          </Card>

          <Card>
            <CardContent className="px-0 pb-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>身份</TableHead>
                    <TableHead>角色</TableHead>
                    <TableHead>来源</TableHead>
                    <TableHead>状态</TableHead>
                    <TableHead>最近登录</TableHead>
                    <TableHead>创建</TableHead>
                    <TableHead><span className="sr-only">操作</span></TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {users.map((user) => (
                    <TableRow key={user.id}>
                      <TableCell>
                        <div className="flex items-center gap-2">
                          <span className="flex size-8 items-center justify-center border border-border bg-popover">
                            <UserRound className="size-3.5 text-muted-foreground" />
                          </span>
                          <div>
                            <p className="text-sm text-foreground">{user.username}</p>
                            <p className="text-[10px] text-muted-foreground">
                              {user.email || user.tenant_id}
                            </p>
                          </div>
                        </div>
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant={
                            user.role === "admin"
                              ? "warn"
                              : user.role === "operator"
                                ? "default"
                                : "secondary"
                          }
                        >
                          <Shield className="size-3" />
                          {user.role}
                        </Badge>
                      </TableCell>
                      <TableCell className="font-mono text-[10px] text-muted-foreground">
                        {user.oidc_issuer ? "OIDC" : "LOCAL"}
                      </TableCell>
                      <TableCell>
                        <Badge variant={user.enabled ? "success" : "secondary"}>
                          {user.enabled ? "ACTIVE" : "DISABLED"}
                        </Badge>
                      </TableCell>
                      <TableCell className="font-mono text-[10px]">
                        {formatTime(user.last_login_at)}
                      </TableCell>
                      <TableCell className="font-mono text-[10px]">
                        {formatTime(user.created_at)}
                      </TableCell>
                      <TableCell>
                        <Button
                          size="icon-sm"
                          variant={user.enabled ? "secondary" : "outline"}
                          aria-label={user.enabled ? "停用" : "启用"}
                          disabled={busy === user.id}
                          onClick={() => toggle(user)}
                        >
                          {user.enabled ? (
                            <Ban className="size-3.5" />
                          ) : (
                            <Power className="size-3.5" />
                          )}
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                  {users.length === 0 && (
                    <TableEmpty colSpan={7} icon={UserRound}>
                      暂无用户或当前身份无权查看
                    </TableEmpty>
                  )}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
