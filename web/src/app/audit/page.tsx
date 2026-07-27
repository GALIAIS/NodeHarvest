"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { AlertTriangle, Download, FileClock, RefreshCw } from "lucide-react";
import { api, errorMessage, isAuthError, type AuditEntry } from "@/lib/api";
import { AuthRequired } from "@/components/auth-required";
import { PageHeader } from "@/components/page-header";
import { PaginationControls } from "@/components/pagination-controls";
import { useSession } from "@/components/session-provider";
import { useLiveRefresh } from "@/components/live-provider";
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
  const { authenticated, loading: sessionLoading } = useSession();
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [query, setQuery] = useState("");
  const [error, setError] = useState("");
  const [needsLogin, setNeedsLogin] = useState(false);
  const [total, setTotal] = useState(0);
  const [nextCursor, setNextCursor] = useState("");
  const [cursorStack, setCursorStack] = useState<string[]>([]);

  const load = useCallback(async (cursor = "") => {
    // /api/admin/audit rejects anonymous callers — don't even ask.
    if (!authenticated) return;
    try {
      const page = await api.auditPage({
        from: from || undefined,
        to: to || undefined,
        cursor: cursor || undefined,
      });
      setEntries(page.entries);
      setTotal(page.total ?? 0);
      setNextCursor(page.next_cursor || "");
      setError("");
      setNeedsLogin(false);
    } catch (cause) {
      if (isAuthError(cause)) {
        setNeedsLogin(true);
        setError("");
      } else {
        setError(errorMessage(cause, "加载失败"));
      }
    }
  }, [authenticated, from, to]);

  useEffect(() => {
    const initial = setTimeout(() => {
      setCursorStack([]);
      void load();
    }, 0);
    return () => clearTimeout(initial);
  }, [load]);

  const currentCursor = cursorStack[cursorStack.length - 1] || "";
  useLiveRefresh(() => load(currentCursor), authenticated && !needsLogin);

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

  const visible = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return entries;
    return entries.filter((entry) =>
      [entry.actor, entry.action, entry.detail].some((value) => value.toLowerCase().includes(needle)),
    );
  }, [entries, query]);

  // Being signed out is a normal state, not an error — but never flash the
  // login prompt while the session is still resolving.
  const locked = !sessionLoading && (!authenticated || needsLogin);

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
            <Button
              size="sm"
              variant="secondary"
              onClick={() => { void load(currentCursor); }}
              disabled={!authenticated}
              title={!authenticated ? "登录后可刷新审计日志" : undefined}
            >
              <RefreshCw className="size-3.5" />
              刷新
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={download}
              disabled={!authenticated || !visible.length}
              title={!authenticated ? "登录后可导出审计日志" : undefined}
            >
              <Download className="size-3.5" />
              导出 JSON
            </Button>
          </>
        }
      />
      <div className="reveal space-y-4 p-4 sm:p-6 lg:p-8">
        {locked ? (
          <AuthRequired
            title="审计日志需要登录"
            description="操作轨迹按租户隔离，登录后可查询与导出。"
          />
        ) : (
          <>
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
            <PaginationControls
              page={cursorStack.length + 1}
              total={total}
              count={visible.length}
              hasNext={Boolean(nextCursor)}
              onPrevious={previousPage}
              onNext={nextPage}
            />
          </>
        )}
      </div>
    </div>
  );
}
