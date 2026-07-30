"use client";

import { useCallback, useEffect, useState } from "react";
import { RefreshCw } from "lucide-react";
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
import { api, errorMessage, type Source } from "@/lib/api";
import { formatBytes, formatMs, formatNumber, formatTime } from "@/lib/utils";

const sortOptions = [
  { value: "priority", label: "优先级" },
  { value: "health", label: "健康度" },
  { value: "contribution", label: "贡献量" },
  { value: "success", label: "成功率" },
];

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
      <PageHeader
        title="采集源"
        description={"管理订阅源的可用性、健康度和贡献，共 " + total + " 个来源。"}
        actions={
          <Button
            type="button"
            variant="outline"
            disabled={loading}
            onClick={() => {
              setCursorStack([]);
              void load();
            }}
          >
            <RefreshCw className={loading ? "animate-spin" : undefined} />
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
          <CardTitle>排序</CardTitle>
          <CardDescription>选择用于排列采集源的指标。</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="max-w-sm space-y-2">
            <Label htmlFor="source-sort">排序指标</Label>
            <Select value={sort} onValueChange={setSort}>
              <SelectTrigger id="source-sort" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {sortOptions.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>采集源列表</CardTitle>
          <CardDescription>按{sortOptions.find((item) => item.value === sort)?.label}排序。</CardDescription>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>名称</TableHead>
                <TableHead>类型</TableHead>
                <TableHead>地址</TableHead>
                <TableHead>状态</TableHead>
                <TableHead className="text-right">健康度</TableHead>
                <TableHead className="text-right">成功率</TableHead>
                <TableHead className="text-right">延迟</TableHead>
                <TableHead className="text-right">贡献</TableHead>
                <TableHead className="text-right">流量</TableHead>
                <TableHead>最近成功</TableHead>
                <TableHead>操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sources.map((source) => (
                <TableRow key={source.name}>
                  <TableCell className="font-medium">{source.name}</TableCell>
                  <TableCell>{source.type}</TableCell>
                  <TableCell className="max-w-80 truncate font-mono text-xs" title={source.url}>
                    {source.url}
                  </TableCell>
                  <TableCell>
                    <StatusBadge status={source.effective_enabled ? "enabled" : "disabled"}>
                      {source.effective_enabled ? "启用" : "停用"}
                    </StatusBadge>
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatNumber(source.health_score)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {Math.round(source.success_rate * 100) + "%"}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">{formatMs(source.latency_ms)}</TableCell>
                  <TableCell className="text-right tabular-nums">
                    {source.contribution_hq + " / " + source.contribution_total}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">{formatBytes(source.bytes)}</TableCell>
                  <TableCell>{formatTime(source.last_success_at)}</TableCell>
                  <TableCell>
                    <div className="flex gap-2">
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
                    </div>
                  </TableCell>
                </TableRow>
              ))}
              {!sources.length && (
                <TableEmpty colSpan={11}>暂无采集源。</TableEmpty>
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
