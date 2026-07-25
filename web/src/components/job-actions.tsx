"use client";

import { useState } from "react";
import { Loader2, Play, Radar, Sparkles, Zap } from "lucide-react";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";

type Props = {
  onStarted?: (jobId: string) => void;
  compact?: boolean;
};

export function JobActions({ onStarted, compact }: Props) {
  const [loading, setLoading] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function run(kind: "full" | "fetch" | "quality" | "ai") {
    setError(null);
    setLoading(kind);
    try {
      const opts =
        kind === "quality" || kind === "full"
          ? { max_test: 800, rounds: 3 }
          : kind === "ai"
            ? {}
            : {};
      const job =
        kind === "full"
          ? await api.startFull(opts)
          : kind === "fetch"
            ? await api.startFetch(opts)
            : kind === "quality"
              ? await api.startQuality(opts)
              : await api.startAI(opts);
      onStarted?.(job.id);
    } catch (e) {
      setError(e instanceof Error ? e.message : "启动失败");
    } finally {
      setLoading(null);
    }
  }

  return (
    <div className="space-y-2">
      <div className={`flex flex-wrap gap-2 ${compact ? "" : ""}`}>
        <Button onClick={() => run("full")} disabled={!!loading} variant="amber">
          {loading === "full" ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Sparkles className="h-4 w-4" />
          )}
          一键全流程
        </Button>
        <Button onClick={() => run("fetch")} disabled={!!loading} variant="secondary">
          {loading === "fetch" ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Play className="h-4 w-4" />
          )}
          采集
        </Button>
        <Button onClick={() => run("quality")} disabled={!!loading} variant="outline">
          {loading === "quality" ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Zap className="h-4 w-4" />
          )}
          智能测速
        </Button>
        <Button onClick={() => run("ai")} disabled={!!loading} variant="ghost">
          {loading === "ai" ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Radar className="h-4 w-4" />
          )}
          AI 探测
        </Button>
      </div>
      {error && <p className="text-xs text-rose-400">{error}</p>}
    </div>
  );
}
