"use client";

import { useCallback, useEffect, useState } from "react";
import { Copy, RefreshCw } from "lucide-react";
import { useLiveRefresh } from "@/components/live-provider";
import { PageHeader } from "@/components/page-header";
import { PaginationControls } from "@/components/pagination-controls";
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
import { Switch } from "@/components/ui/switch";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { api, errorMessage, type CountryRow, type NodeItem } from "@/lib/api";
import { formatMs, formatNumber, formatTime } from "@/lib/utils";

export default function NodesPage() {
  const [nodes, setNodes] = useState<NodeItem[]>([]);
  const [countries, setCountries] = useState<CountryRow[]>([]);
  const [query, setQuery] = useState("");
  const [protocol, setProtocol] = useState("");
  const [grade, setGrade] = useState("");
  const [country, setCountry] = useState("");
  const [minScore, setMinScore] = useState("");
  const [highQuality, setHighQuality] = useState(false);
  const [alive, setAlive] = useState(true);
  const [error, setError] = useState("");
  const [total, setTotal] = useState(0);
  const [nextCursor, setNextCursor] = useState("");
  const [cursorStack, setCursorStack] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);

  const load = useCallback(async (cursor = "") => {
    setLoading(true);
    try {
      const page = await api.nodes({
        q: query,
        protocol,
        grade,
        country,
        min_score: minScore ? Number(minScore) : undefined,
        hq: highQuality ? true : undefined,
        alive: alive ? true : undefined,
        limit: 50,
        cursor: cursor || undefined,
      });
      setNodes(page.nodes);
      setTotal(page.total);
      setNextCursor(page.next_cursor || "");
      setError("");
    } catch (cause) {
      setError(errorMessage(cause, "加载节点失败"));
    } finally {
      setLoading(false);
    }
  }, [alive, country, grade, highQuality, minScore, protocol, query]);

  useEffect(() => {
    const initial = window.setTimeout(() => {
      setCursorStack([]);
      void load();
    }, 0);
    return () => window.clearTimeout(initial);
  }, [load]);

  useEffect(() => {
    void api.countries({ hq: true, alive: true, limit: 300 }).then(
      (page) => setCountries(page.countries),
      () => setCountries([]),
    );
  }, []);

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

  async function copyNode(node: NodeItem) {
    if (!node.raw_uri) return;
    try {
      await navigator.clipboard.writeText(node.raw_uri);
    } catch {
      setError("无法复制节点链接。");
    }
  }

  return (
    <>
      <PageHeader
        title="节点库"
        description={"检索、筛选并查看 " + total + " 个节点的质量状态。"}
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
          <AlertTitle>节点加载失败</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <Card>
        <CardHeader>
          <CardTitle>筛选</CardTitle>
          <CardDescription>筛选条件会立即应用。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-5">
            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="node-query">搜索</Label>
              <Input
                id="node-query"
                value={query}
                placeholder="名称、服务器或标签"
                onChange={(event) => setQuery(event.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="node-protocol">协议</Label>
              <Select
                value={protocol || "all"}
                onValueChange={(value) => setProtocol(value === "all" ? "" : value)}
              >
                <SelectTrigger id="node-protocol" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">全部协议</SelectItem>
                  {["vless", "vmess", "trojan", "ss", "hysteria2", "tuic"].map((item) => (
                    <SelectItem key={item} value={item}>{item}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="node-grade">等级</Label>
              <Select
                value={grade || "all"}
                onValueChange={(value) => setGrade(value === "all" ? "" : value)}
              >
                <SelectTrigger id="node-grade" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">全部等级</SelectItem>
                  {["S", "A", "B", "C", "D"].map((item) => (
                    <SelectItem key={item} value={item}>{item}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="node-country">国家</Label>
              <Input
                id="node-country"
                list="node-countries"
                value={country}
                placeholder="ISO 代码"
                onChange={(event) => setCountry(event.target.value)}
              />
              <datalist id="node-countries">
                {countries.map((item) => (
                  <option key={item.code} value={item.code}>
                    {item.display}
                  </option>
                ))}
              </datalist>
            </div>
            <div className="space-y-2">
              <Label htmlFor="node-min-score">最低评分</Label>
              <Input
                id="node-min-score"
                type="number"
                min="0"
                max="100"
                value={minScore}
                onChange={(event) => setMinScore(event.target.value)}
              />
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-x-6 gap-y-3">
            <div className="flex items-center gap-2">
              <Switch id="node-alive" checked={alive} onCheckedChange={setAlive} />
              <Label htmlFor="node-alive">仅存活节点</Label>
            </div>
            <div className="flex items-center gap-2">
              <Switch
                id="node-high-quality"
                checked={highQuality}
                onCheckedChange={setHighQuality}
              />
              <Label htmlFor="node-high-quality">仅高质量节点</Label>
            </div>
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>节点列表</CardTitle>
          <CardDescription>当前筛选结果共 {total} 条。</CardDescription>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>名称</TableHead>
                <TableHead>协议</TableHead>
                <TableHead>服务器</TableHead>
                <TableHead>位置</TableHead>
                <TableHead>等级</TableHead>
                <TableHead className="text-right">评分</TableHead>
                <TableHead className="text-right">延迟</TableHead>
                <TableHead>最后测试</TableHead>
                <TableHead>操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {nodes.map((node) => (
                <TableRow key={node.id}>
                  <TableCell className="font-medium">{node.name || "未命名"}</TableCell>
                  <TableCell>
                    <StatusBadge status={node.alive ? "alive" : "dead"}>
                      {node.protocol}
                    </StatusBadge>
                  </TableCell>
                  <TableCell className="max-w-72 truncate font-mono text-xs" title={node.server + ":" + node.port}>
                    {node.server + ":" + node.port}
                  </TableCell>
                  <TableCell>{[node.country, node.city].filter(Boolean).join(" · ") || "—"}</TableCell>
                  <TableCell>{node.grade}</TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatNumber(node.score)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">{formatMs(node.latency_ms)}</TableCell>
                  <TableCell>{formatTime(node.tested_at)}</TableCell>
                  <TableCell>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      disabled={!node.raw_uri}
                      onClick={() => void copyNode(node)}
                    >
                      <Copy />
                      复制链接
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
              {!nodes.length && (
                <TableEmpty colSpan={9}>没有匹配的节点。</TableEmpty>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      <PaginationControls
        page={cursorStack.length + 1}
        total={total}
        count={nodes.length}
        hasNext={Boolean(nextCursor)}
        disabled={loading}
        onPrevious={previousPage}
        onNext={nextPage}
      />
    </>
  );
}
