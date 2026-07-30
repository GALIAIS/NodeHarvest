"use client";

import { useCallback, useEffect, useState } from "react";
import { useLiveRefresh } from "@/components/live-provider";
import { PaginationControls } from "@/components/pagination-controls";
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
import { api, errorMessage, type CountryRow, type NodeItem } from "@/lib/api";
import { formatMs, formatTime } from "@/lib/utils";

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
      <header>
        <h1>节点库</h1>
        <p>共 {total} 个节点。</p>
      </header>
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
        <CardContent>
          <Label htmlFor="node-query">搜索</Label>
          <Input
            id="node-query"
            value={query}
            placeholder="名称、服务器或标签"
            onChange={(event) => setQuery(event.target.value)}
          />
          <Label htmlFor="node-protocol">协议</Label>
          <Input
            id="node-protocol"
            value={protocol}
            placeholder="例如 vless"
            onChange={(event) => setProtocol(event.target.value)}
          />
          <Label htmlFor="node-grade">等级</Label>
          <Input
            id="node-grade"
            value={grade}
            placeholder="S、A、B"
            onChange={(event) => setGrade(event.target.value)}
          />
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
          <Label htmlFor="node-min-score">最低评分</Label>
          <Input
            id="node-min-score"
            type="number"
            min="0"
            max="100"
            value={minScore}
            onChange={(event) => setMinScore(event.target.value)}
          />
          <Button
            type="button"
            variant={alive ? "secondary" : "outline"}
            aria-pressed={alive}
            onClick={() => setAlive((value) => !value)}
          >
            仅存活节点
          </Button>
          <Button
            type="button"
            variant={highQuality ? "secondary" : "outline"}
            aria-pressed={highQuality}
            onClick={() => setHighQuality((value) => !value)}
          >
            仅高质量节点
          </Button>
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
          <CardTitle>结果</CardTitle>
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
                <TableHead>评分</TableHead>
                <TableHead>延迟</TableHead>
                <TableHead>最后测试</TableHead>
                <TableHead>操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {nodes.map((node) => (
                <TableRow key={node.id}>
                  <TableCell>{node.name || "未命名"}</TableCell>
                  <TableCell>{node.protocol}</TableCell>
                  <TableCell>{node.server + ":" + node.port}</TableCell>
                  <TableCell>{[node.country, node.city].filter(Boolean).join(" · ") || "—"}</TableCell>
                  <TableCell>{node.grade}</TableCell>
                  <TableCell>{node.score}</TableCell>
                  <TableCell>{formatMs(node.latency_ms)}</TableCell>
                  <TableCell>{formatTime(node.tested_at)}</TableCell>
                  <TableCell>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      disabled={!node.raw_uri}
                      onClick={() => void copyNode(node)}
                    >
                      复制链接
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
              {!nodes.length && (
                <TableRow>
                  <TableCell colSpan={9}>没有匹配的节点。</TableCell>
                </TableRow>
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
