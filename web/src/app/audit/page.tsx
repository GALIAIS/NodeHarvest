"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Download, FileClock, Filter, RefreshCw } from "lucide-react";
import { api, type AuditEntry } from "@/lib/api";
import { PageHeader } from "@/components/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { formatTime } from "@/lib/utils";

export default function AuditPage() {
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [query, setQuery] = useState("");
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    try {
      setEntries(await api.audit({ from: from || undefined, to: to || undefined }));
      setError("");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "加载失败");
    }
  }, [from, to]);

  useEffect(() => {
    const initial = setTimeout(load, 0);
    return () => clearTimeout(initial);
  }, [load]);

  const visible = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return entries;
    return entries.filter((entry) =>
      [entry.actor, entry.action, entry.detail].some((value) => value.toLowerCase().includes(needle)),
    );
  }, [entries, query]);

  function download() {
    const href = URL.createObjectURL(new Blob([JSON.stringify(visible, null, 2)], { type: "application/json" }));
    const link = document.createElement("a");
    link.href = href;
    link.download = `nodeharvest-audit-${new Date().toISOString().slice(0, 10)}.json`;
    link.click();
    URL.revokeObjectURL(href);
  }

  return (
    <div className="flex flex-1 flex-col">
      <PageHeader
        eyebrow="Accountability ledger"
        title="审计日志"
        description="按租户隔离的不可变操作轨迹，覆盖任务、导出、源治理、凭证、用户、配置和告警动作。"
        actions={
          <>
            <Button size="sm" variant="secondary" onClick={load}><RefreshCw className="h-3.5 w-3.5" />刷新</Button>
            <Button size="sm" variant="outline" onClick={download} disabled={!visible.length}><Download className="h-3.5 w-3.5" />导出 JSON</Button>
          </>
        }
      />
      <div className="reveal space-y-4 p-4 sm:p-6 lg:p-8">
        <Card>
          <CardContent className="control-grid p-4">
            <label className="text-xs text-slate-500">开始日期<Input className="mt-1.5" type="date" value={from} max={to || undefined} onChange={(e) => setFrom(e.target.value)} /></label>
            <label className="text-xs text-slate-500">结束日期<Input className="mt-1.5" type="date" value={to} min={from || undefined} onChange={(e) => setTo(e.target.value)} /></label>
            <label className="text-xs text-slate-500">
              <span className="flex items-center gap-1"><Filter className="h-3 w-3" />筛选当前结果</span>
              <Input className="mt-1.5" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="actor / action / detail" />
            </label>
          </CardContent>
        </Card>
        {error && <div role="alert" className="rounded-md border border-rose-500/30 bg-rose-500/10 px-4 py-3 text-sm text-rose-300">{error}</div>}
        <Card>
          <CardContent className="p-0">
            <div className="table-wrap">
              <table>
                <thead><tr><th>ID</th><th>时间</th><th>Actor</th><th>动作</th><th>详情</th></tr></thead>
                <tbody>
                  {visible.map((entry) => (
                    <tr key={entry.id}>
                      <td className="font-mono text-[10px] text-slate-700">#{entry.id}</td>
                      <td className="whitespace-nowrap font-mono text-[10px]">{formatTime(entry.at)}</td>
                      <td><Badge variant={entry.actor === "system" ? "secondary" : "default"}>{entry.actor}</Badge></td>
                      <td className="font-mono text-xs text-cyan-200">{entry.action}</td>
                      <td className="max-w-lg break-words text-xs text-slate-500">{entry.detail || "—"}</td>
                    </tr>
                  ))}
                  {visible.length === 0 && (
                    <tr><td colSpan={5} className="py-16 text-center text-slate-600"><FileClock className="mx-auto mb-3 h-5 w-5" />当前范围没有审计记录</td></tr>
                  )}
                </tbody>
              </table>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
