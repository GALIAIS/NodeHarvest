"use client";

import { useCallback, useEffect, useState } from "react";
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { api, errorMessage, type Source } from "@/lib/api";
import { formatBytes, formatMs, formatTime } from "@/lib/utils";

const sortOptions = ["priority", "health", "contribution", "success"];

export default function SourcesPage() {
  const { canOperate } = useSession();
  const [sources, setSources] = useState<Source[]>([]);
  const [sort, setSort] = useState("priority");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [total, setTotal] = useState(0);
  const [nextCursor, setNextCursor] = useState("");
  const [cursorStack, setCursorStack] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);

  const load = useCallback(async (cursor = "") => {
    setLoading(true);
    try {
      const page = await api.sourcesPage(sort, { cursor: cursor || undefined });
      setSources(page.sources);
      setTotal(page.total ?? 0);
      setNextCursor(page.next_cursor || "");
      setError("");
    } catch (cause) {
      setError(errorMessage(cause, "加载采集源失败"));
    } finally {
      setLoading(false);
    }
  }, [sort]);

  useEffect(() => {
    const initial = window.setTimeout(() => {
      setCursorStack([]);
      void load();
    }, 0);
    return () => window.clearTimeout(initial);
  }, [load]);

  const currentCursor = cursorStack[cursorStack.length - 1] || "";
  useLiveRefresh(() => load(currentCursor));

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

  async function toggle(source: Source) {
    setBusy(source.name);
    try {
      await api.setSourceEnabled(source.name, !source.manually_disabled);
      await load(currentCursor);
    } catch (cause) {
      setError(errorMessage(cause, "更新采集源失败"));
    } finally {
      setBusy("");
    }
  }

  async function probe(source: Source) {
    setBusy(source.name);
    try {
      await api.probeSource(source.name);
      await load(currentCursor);
    } catch (cause) {
      setError(errorMessage(cause, "探测采集源失败"));
    } finally {
      setBusy("");
    }
  }

  return (
    <>
      <header>
        <h1>采集源</h1>
        <p>管理订阅源的可用性和贡献。</p>
      </header>
      {error && (
        <Alert variant="destructive">
          <AlertTitle>操作失败</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <Card>
        <CardHeader>
          <CardTitle>排序</CardTitle>
          <CardDescription>选择用于排列采集源的指标。</CardDescription>
        </CardHeader>
        <CardContent>
          {sortOptions.map((option) => (
            <Button
              key={option}
              type="button"
              variant={sort === option ? "secondary" : "outline"}
              aria-pressed={sort === option}
              onClick={() => setSort(option)}
            >
              {option}
            </Button>
          ))}
          <Button
            type="button"
            variant="outline"
            disabled={loading}
            onClick={() => {
              setCursorStack([]);
              void load();
            }}
          >
            刷新
          </Button>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>采集源列表</CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>名称</TableHead>
                <TableHead>类型</TableHead>
                <TableHead>地址</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>健康度</TableHead>
                <TableHead>成功率</TableHead>
                <TableHead>延迟</TableHead>
                <TableHead>贡献</TableHead>
                <TableHead>流量</TableHead>
                <TableHead>最近成功</TableHead>
                <TableHead>操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sources.map((source) => (
                <TableRow key={source.name}>
                  <TableCell>{source.name}</TableCell>
                  <TableCell>{source.type}</TableCell>
                  <TableCell>{source.url}</TableCell>
                  <TableCell>{source.effective_enabled ? "启用" : "停用"}</TableCell>
                  <TableCell>{source.health_score}</TableCell>
                  <TableCell>{Math.round(source.success_rate * 100) + "%"}</TableCell>
                  <TableCell>{formatMs(source.latency_ms)}</TableCell>
                  <TableCell>{source.contribution_hq + " / " + source.contribution_total}</TableCell>
                  <TableCell>{formatBytes(source.bytes)}</TableCell>
                  <TableCell>{formatTime(source.last_success_at)}</TableCell>
                  <TableCell>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      disabled={!canOperate || busy === source.name}
                      onClick={() => void probe(source)}
                    >
                      探测
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant={source.manually_disabled ? "secondary" : "outline"}
                      disabled={!canOperate || busy === source.name}
                      onClick={() => void toggle(source)}
                    >
                      {source.manually_disabled ? "启用" : "停用"}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
              {!sources.length && (
                <TableRow>
                  <TableCell colSpan={11}>暂无采集源。</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      <PaginationControls
        page={cursorStack.length + 1}
        total={total}
        count={sources.length}
        hasNext={Boolean(nextCursor)}
        disabled={loading}
        onPrevious={previousPage}
        onNext={nextPage}
      />
    </>
  );
}
