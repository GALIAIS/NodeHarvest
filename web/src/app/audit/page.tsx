"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { AuthRequired } from "@/components/auth-required";
import { useLiveRefresh } from "@/components/live-provider";
import { PaginationControls } from "@/components/pagination-controls";
import { useSession } from "@/components/session-provider";
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
import { api, errorMessage, type AuditEntry } from "@/lib/api";
import { formatTime } from "@/lib/utils";

export default function AuditPage() {
  const { authenticated, loading, canAdmin } = useSession();
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [query, setQuery] = useState("");
  const [error, setError] = useState("");
  const [total, setTotal] = useState(0);
  const [nextCursor, setNextCursor] = useState("");
  const [cursorStack, setCursorStack] = useState<string[]>([]);

  const load = useCallback(async (cursor = "") => {
    if (!canAdmin) return;
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
    } catch (cause) {
      setError(errorMessage(cause, "加载审计日志失败"));
    }
  }, [canAdmin, from, to]);

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

  const visible = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return entries;
    return entries.filter((entry) =>
      [entry.actor, entry.action, entry.detail].some((value) => value.toLowerCase().includes(needle)),
    );
  }, [entries, query]);

  function download() {
    const blob = new Blob([JSON.stringify(visible, null, 2)], { type: "application/json" });
    const objectURL = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = objectURL;
    link.download = "nodeharvest-audit.json";
    link.click();
    URL.revokeObjectURL(objectURL);
  }

  if (!authenticated && !loading) return <AuthRequired />;
  if (authenticated && !canAdmin) return <AuthRequired reason="forbidden" />;

  return (
    <>
      <header className="space-y-1">
        <h1 className="text-3xl font-bold tracking-tight">审计</h1>
        <p className="text-muted-foreground">查询并导出当前租户的操作记录。</p>
      </header>
      {error && (
        <Alert variant="destructive">
          <AlertTitle>加载失败</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <Card>
        <CardHeader>
          <CardTitle>筛选</CardTitle>
          <CardDescription>关键字仅筛选当前已加载结果。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <Label htmlFor="audit-from">开始日期</Label>
          <Input
            id="audit-from"
            type="date"
            value={from}
            max={to || undefined}
            onChange={(event) => setFrom(event.target.value)}
          />
          <Label htmlFor="audit-to">结束日期</Label>
          <Input
            id="audit-to"
            type="date"
            value={to}
            min={from || undefined}
            onChange={(event) => setTo(event.target.value)}
          />
          <Label htmlFor="audit-query">关键字</Label>
          <Input
            id="audit-query"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="actor、action 或 detail"
          />
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              setCursorStack([]);
              void load();
            }}
          >
            刷新
          </Button>
          <Button type="button" disabled={!visible.length} onClick={download}>
            导出 JSON
          </Button>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>操作记录</CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>ID</TableHead>
                <TableHead>时间</TableHead>
                <TableHead>操作人</TableHead>
                <TableHead>动作</TableHead>
                <TableHead>详情</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {visible.map((entry) => (
                <TableRow key={entry.id}>
                  <TableCell>{entry.id}</TableCell>
                  <TableCell>{formatTime(entry.at)}</TableCell>
                  <TableCell>{entry.actor}</TableCell>
                  <TableCell>{entry.action}</TableCell>
                  <TableCell>{entry.detail || "—"}</TableCell>
                </TableRow>
              ))}
              {!visible.length && (
                <TableRow>
                  <TableCell colSpan={5}>没有匹配的审计记录。</TableCell>
                </TableRow>
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
  );
}
