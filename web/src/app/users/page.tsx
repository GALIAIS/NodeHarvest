"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { Ban, Plus, Power, Shield, UserRound } from "lucide-react";
import { api, type UserRecord } from "@/lib/api";
import { PageHeader } from "@/components/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
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
      <div className="reveal space-y-5 p-4 sm:p-6 lg:p-8">
        {error && <div role="alert" className="rounded-md border border-rose-500/30 bg-rose-500/10 px-4 py-3 text-sm text-rose-300">{error}</div>}
        <div className="grid gap-4 xl:grid-cols-[minmax(320px,.55fr)_minmax(0,1.45fr)]">
          <Card className="h-fit">
            <CardHeader>
              <CardTitle className="flex items-center gap-2"><Plus className="h-4 w-4 text-cyan-300" /> 新建本地用户</CardTitle>
              <CardDescription>密码使用 bcrypt 哈希保存，不会通过管理 API 返回。</CardDescription>
            </CardHeader>
            <CardContent>
              <form onSubmit={create} className="space-y-4">
                <label className="block text-xs text-slate-500">
                  用户名<Input className="mt-1.5" value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} required />
                </label>
                <label className="block text-xs text-slate-500">
                  邮箱<Input className="mt-1.5" type="email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} />
                </label>
                <label className="block text-xs text-slate-500">
                  初始密码<Input className="mt-1.5" type="password" autoComplete="new-password" minLength={12} value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} required />
                </label>
                <label className="block text-xs text-slate-500">
                  角色
                  <select className="mt-1.5 h-10 w-full px-3 text-sm" value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value })}>
                    <option value="viewer">viewer · 查看</option>
                    <option value="operator">operator · 操作</option>
                    <option value="admin">admin · 管理</option>
                  </select>
                </label>
                <Button type="submit" className="w-full" disabled={busy === "create"}>
                  <UserRound className="h-4 w-4" /> {busy === "create" ? "创建中…" : "创建用户"}
                </Button>
              </form>
            </CardContent>
          </Card>

          <Card>
            <CardContent className="p-0">
              <div className="table-wrap">
                <table>
                  <thead><tr><th>身份</th><th>角色</th><th>来源</th><th>状态</th><th>最近登录</th><th>创建</th><th></th></tr></thead>
                  <tbody>
                    {users.map((user) => (
                      <tr key={user.id}>
                        <td>
                          <div className="flex items-center gap-2">
                            <span className="flex h-8 w-8 items-center justify-center rounded-md border border-slate-800 bg-slate-950">
                              <UserRound className="h-3.5 w-3.5 text-slate-500" />
                            </span>
                            <div>
                              <p className="text-sm text-slate-200">{user.username}</p>
                              <p className="text-[10px] text-slate-600">{user.email || user.tenant_id}</p>
                            </div>
                          </div>
                        </td>
                        <td><Badge variant={user.role === "admin" ? "warn" : user.role === "operator" ? "default" : "secondary"}><Shield className="mr-1 h-3 w-3" />{user.role}</Badge></td>
                        <td className="font-mono text-[10px] text-slate-500">{user.oidc_issuer ? "OIDC" : "LOCAL"}</td>
                        <td><Badge variant={user.enabled ? "success" : "secondary"}>{user.enabled ? "ACTIVE" : "DISABLED"}</Badge></td>
                        <td className="font-mono text-[10px]">{formatTime(user.last_login_at)}</td>
                        <td className="font-mono text-[10px]">{formatTime(user.created_at)}</td>
                        <td>
                          <Button size="sm" variant={user.enabled ? "secondary" : "outline"} disabled={busy === user.id} onClick={() => toggle(user)}>
                            {user.enabled ? <Ban className="h-3.5 w-3.5" /> : <Power className="h-3.5 w-3.5" />}
                          </Button>
                        </td>
                      </tr>
                    ))}
                    {users.length === 0 && <tr><td colSpan={7} className="py-16 text-center text-slate-600">暂无用户或当前身份无权查看</td></tr>}
                  </tbody>
                </table>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
