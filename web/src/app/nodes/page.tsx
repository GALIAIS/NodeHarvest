"use client";

import { useCallback, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { AlertTriangle, Check, Copy, Filter, Network, RefreshCw, Search } from "lucide-react";
import { api, type CountryRow, type NodeItem } from "@/lib/api";
import { PageHeader } from "@/components/page-header";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { CheckboxChip } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
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
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn, formatMs, gradeColor } from "@/lib/utils";

const PROTOCOLS = ["vmess", "vless", "trojan", "ss", "ssr", "hysteria2", "tuic"];
const GRADES = ["S", "A", "B", "C", "D", "F"];
const AI_TARGETS = ["chatgpt", "gemini", "claude", "grok", "openai", "copilot"];

/** Radix Select reserves the empty string, so "all" stands in for "no filter". */
const ALL = "all";
const unset = (value: string) => (value === ALL ? "" : value);

export default function NodesPage() {
  const sp = useSearchParams();
  const [nodes, setNodes] = useState<NodeItem[]>([]);
  const [total, setTotal] = useState(0);
  const [q, setQ] = useState("");
  const [protocol, setProtocol] = useState(ALL);
  const [grade, setGrade] = useState(ALL);
  const [country, setCountry] = useState(ALL);
  const [minScore, setMinScore] = useState(sp.get("hq") === "1" ? "70" : "");
  const [hq, setHq] = useState(sp.get("hq") === "1");
  const [alive, setAlive] = useState(true);
  const [verified, setVerified] = useState(false);
  const [ai, setAi] = useState(ALL);
  const [countries, setCountries] = useState<CountryRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [copied, setCopied] = useState<string | null>(null);
  const [nextCursor, setNextCursor] = useState("");

  const load = useCallback(
    async (cursor = "", append = false) => {
      setLoading(true);
      try {
        const res = await api.nodes({
          limit: 200,
          q: q || undefined,
          protocol: unset(protocol) || undefined,
          grade: unset(grade) || undefined,
          country: unset(country) || undefined,
          hq: hq || undefined,
          alive: alive || undefined,
          verified: verified || undefined,
          ai: unset(ai) || undefined,
          min_score: minScore || (hq ? 70 : undefined),
          cursor: cursor || undefined,
        });
        setNodes((current) => (append ? [...current, ...res.nodes] : res.nodes));
        setTotal(res.total);
        setNextCursor(res.next_cursor || "");
        setErr(null);
      } catch (cause) {
        setErr(cause instanceof Error ? cause.message : "加载失败");
      } finally {
        setLoading(false);
      }
    },
    [q, protocol, grade, country, minScore, hq, alive, verified, ai],
  );

  useEffect(() => {
    const initial = setTimeout(load, 0);
    return () => clearTimeout(initial);
  }, [load]);

  useEffect(() => {
    api
      .countries({ alive: true })
      .then((result) => setCountries(result.countries))
      .catch(() => {});
  }, []);

  async function copyURI(uri: string, id: string) {
    try {
      await navigator.clipboard.writeText(uri);
      setCopied(id);
      setTimeout(() => setCopied(null), 1500);
    } catch {
      /* clipboard unavailable outside secure contexts */
    }
  }

  return (
    <div className="flex flex-1 flex-col">
      <PageHeader
        eyebrow="Node inventory"
        title="节点资产库"
        description={`按国家、协议、评分、真实拨测与 AI 可达性筛选 · 当前 ${total} 条`}
        actions={
          <Button variant="secondary" size="sm" onClick={() => load()} disabled={loading}>
            <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
            刷新
          </Button>
        }
      />

      <div className="reveal space-y-4 p-4 sm:p-6 lg:p-8">
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2">
              <Filter className="size-4 text-primary" />
              过滤器
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-wrap items-center gap-2">
            <div className="relative min-w-52 flex-1">
              <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                className="pl-9"
                placeholder="搜索名称 / 服务器 / 来源"
                aria-label="搜索名称 / 服务器 / 来源"
                value={q}
                onChange={(event) => setQ(event.target.value)}
              />
            </div>

            <Select value={protocol} onValueChange={setProtocol}>
              <SelectTrigger className="w-36" aria-label="协议筛选">
                <SelectValue placeholder="全部协议" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>全部协议</SelectItem>
                {PROTOCOLS.map((item) => (
                  <SelectItem key={item} value={item} className="font-mono">
                    {item}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            <Select value={country} onValueChange={setCountry}>
              <SelectTrigger className="w-44" aria-label="国家筛选">
                <SelectValue placeholder="全部国家" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>全部国家</SelectItem>
                {countries.map((item) => (
                  <SelectItem key={item.code} value={item.code}>
                    {item.flag} {item.name || item.code} ({item.count})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            <Select value={grade} onValueChange={setGrade}>
              <SelectTrigger className="w-32" aria-label="等级筛选">
                <SelectValue placeholder="全部等级" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>全部等级</SelectItem>
                {GRADES.map((item) => (
                  <SelectItem key={item} value={item} className="font-mono">
                    {item}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            <Input
              className="w-32"
              type="number"
              min="0"
              max="100"
              value={minScore}
              onChange={(event) => setMinScore(event.target.value)}
              placeholder="最低评分"
              aria-label="最低评分"
            />

            <Select value={ai} onValueChange={setAi}>
              <SelectTrigger className="w-40" aria-label="AI 可达筛选">
                <SelectValue placeholder="AI 过滤" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>AI 过滤</SelectItem>
                {AI_TARGETS.map((item) => (
                  <SelectItem key={item} value={item}>
                    可通过 {item}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            <CheckboxChip checked={hq} onCheckedChange={(state) => setHq(state === true)}>
              仅高质量
            </CheckboxChip>
            <CheckboxChip checked={alive} onCheckedChange={(state) => setAlive(state === true)}>
              仅存活
            </CheckboxChip>
            <CheckboxChip
              checked={verified}
              onCheckedChange={(state) => setVerified(state === true)}
            >
              真实拨测
            </CheckboxChip>
          </CardContent>
        </Card>

        {err && (
          <Alert variant="danger">
            <AlertTriangle />
            <AlertDescription>{err}</AlertDescription>
          </Alert>
        )}

        <Card>
          <CardContent className="px-0 pb-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>等级</TableHead>
                  <TableHead>分数</TableHead>
                  <TableHead>名称</TableHead>
                  <TableHead>国家</TableHead>
                  <TableHead>协议</TableHead>
                  <TableHead>地址</TableHead>
                  <TableHead>延迟</TableHead>
                  <TableHead>成功率</TableHead>
                  <TableHead>抖动</TableHead>
                  <TableHead>AI</TableHead>
                  <TableHead>来源</TableHead>
                  <TableHead>
                    <span className="sr-only">操作</span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {nodes.map((node) => {
                  const aiOk = Object.entries(node.ai_access || {})
                    .filter(([, result]) => result?.ok)
                    .map(([key]) => key);
                  return (
                    <TableRow key={node.id}>
                      <TableCell>
                        <span
                          className={cn(
                            "inline-flex size-6 items-center justify-center border font-mono text-xs font-semibold",
                            gradeColor(node.grade),
                          )}
                        >
                          {node.grade || "—"}
                        </span>
                      </TableCell>
                      <TableCell className="font-mono tabular-nums text-accent">
                        {typeof node.score === "number" ? node.score.toFixed(1) : "—"}
                      </TableCell>
                      <TableCell className="max-w-44 truncate" title={node.name}>
                        {node.name}
                      </TableCell>
                      <TableCell className="font-mono text-xs text-muted-foreground">
                        {node.country || "ZZ"}
                      </TableCell>
                      <TableCell>
                        <Badge variant="secondary">{node.protocol}</Badge>
                      </TableCell>
                      <TableCell className="font-mono text-xs text-muted-foreground">
                        {node.server}:{node.port}
                      </TableCell>
                      <TableCell className="font-mono text-xs tabular-nums">
                        {formatMs(node.latency_ms)}
                      </TableCell>
                      <TableCell className="font-mono text-xs tabular-nums">
                        {node.quality ? `${Math.round(node.quality.success_rate * 100)}%` : "—"}
                      </TableCell>
                      <TableCell className="font-mono text-xs tabular-nums">
                        {node.quality ? formatMs(node.quality.jitter_ms) : "—"}
                      </TableCell>
                      <TableCell>
                        <div className="flex max-w-36 flex-wrap gap-1">
                          {aiOk.length === 0 && (
                            <span className="text-xs text-muted-foreground">—</span>
                          )}
                          {aiOk.slice(0, 3).map((key) => (
                            <Badge key={key} variant="success">
                              {key}
                            </Badge>
                          ))}
                        </div>
                      </TableCell>
                      <TableCell className="max-w-28 truncate text-xs text-muted-foreground">
                        {node.source}
                      </TableCell>
                      <TableCell>
                        <Tooltip>
                          {/* wrapper span keeps the tooltip reachable when the
                              button is disabled (pointer-events-none) */}
                          <TooltipTrigger asChild>
                            <span tabIndex={node.raw_uri ? -1 : 0} className="inline-flex">
                              <Button
                                size="icon-sm"
                                variant="ghost"
                                aria-label="复制节点 URI"
                                onClick={() => node.raw_uri && copyURI(node.raw_uri, node.id)}
                                disabled={!node.raw_uri}
                              >
                                {copied === node.id ? (
                                  <Check className="size-3.5 text-success" />
                                ) : (
                                  <Copy className="size-3.5" />
                                )}
                              </Button>
                            </span>
                          </TooltipTrigger>
                          <TooltipContent side="left">
                            {node.raw_uri ? "复制 URI" : "输入管理 Token 后可复制"}
                          </TooltipContent>
                        </Tooltip>
                      </TableCell>
                    </TableRow>
                  );
                })}
                {nodes.length === 0 && (
                  <TableEmpty colSpan={12} icon={Network}>
                    无匹配节点
                  </TableEmpty>
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>

        {nextCursor && (
          <div className="flex justify-center">
            <Button variant="secondary" disabled={loading} onClick={() => load(nextCursor, true)}>
              {loading ? "加载中…" : `继续加载（已显示 ${nodes.length}/${total}）`}
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}
