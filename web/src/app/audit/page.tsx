"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { AlertTriangle, Download, FileClock, RefreshCw } from "lucide-react";
import { api, type AuditEntry } from "@/lib/api";
import { PageHeader } from "@/components/page-header";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Field } from "@/components/ui/label";
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
            <Button size="sm" variant="secondary" onClick={load}>
              <RefreshCw className="size-3.5" />
              刷新
            </Button>
            <Button size="sm" variant="outline" onClick={download} disabled={!visible.length}>
              <Download className="size-3.5" />
              导出 JSON
            </Button>
          </>
        }
      />
      <div className="reveal space-y-4 p-4 sm:p-6 lg:p-8">
        <Card>
          <CardContent className="control-grid p-4">
            <Field label="开始日期" htmlFor="audit-from">
              <Input
                id="audit-from"
                type="date"
                value={from}
                max={to || undefined}
                onChange={(e) => setFrom(e.target.value)}
              />
            </Field>
            <Field label="结束日期" htmlFor="audit-to">
              <Input
                id="audit-to"
                type="date"
                value={to}
                min={from || undefined}
                onChange={(e) => setTo(e.target.value)}
              />
            </Field>
            <Field label="筛选当前结果" htmlFor="audit-query">
              <Input
                id="audit-query"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="actor / action / detail"
              />
            </Field>
          </CardContent>
        </Card>

        {error && (
          <Alert variant="danger">
            <AlertTriangle />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        <Card>
          <CardContent className="px-0 pb-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>ID</TableHead>
                  <TableHead>时间</TableHead>
                  <TableHead>Actor</TableHead>
                  <TableHead>动作</TableHead>
                  <TableHead>详情</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {visible.map((entry) => (
                  <TableRow key={entry.id}>
                    <TableCell className="font-mono text-[10px] text-muted-foreground">
                      #{entry.id}
                    </TableCell>
                    <TableCell className="whitespace-nowrap font-mono text-[10px]">
                      {formatTime(entry.at)}
                    </TableCell>
                    <TableCell>
                      <Badge variant={entry.actor === "system" ? "secondary" : "default"}>
                        {entry.actor}
                      </Badge>
                    </TableCell>
                    <TableCell className="font-mono text-xs text-primary">{entry.action}</TableCell>
                    <TableCell className="max-w-lg break-words text-xs text-muted-foreground">
                      {entry.detail || "—"}
                    </TableCell>
                  </TableRow>
                ))}
                {visible.length === 0 && (
                  <TableEmpty colSpan={5} icon={FileClock}>
                    当前范围没有审计记录
                  </TableEmpty>
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
