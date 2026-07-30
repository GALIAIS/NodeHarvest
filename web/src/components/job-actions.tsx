"use client";

import { useState } from "react";
import { Bot, Gauge, Play, RadioTower } from "lucide-react";
import { api, errorMessage } from "@/lib/api";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";

type Kind = "full" | "fetch" | "quality" | "ai";

const actions: Array<{
  kind: Kind;
  label: string;
  variant: "default" | "secondary" | "outline" | "ghost";
  icon: typeof Play;
}> = [
  { kind: "full", label: "一键全流程", variant: "default", icon: Play },
  { kind: "fetch", label: "采集", variant: "secondary", icon: RadioTower },
  { kind: "quality", label: "智能测速", variant: "outline", icon: Gauge },
  { kind: "ai", label: "AI 探测", variant: "ghost", icon: Bot },
];

export function JobActions({
  onStarted,
  disabled = false,
}: {
  onStarted?: (jobId: string) => void;
  disabled?: boolean;
}) {
  const [loading, setLoading] = useState<Kind | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function run(kind: Kind) {
    setError(null);
    setLoading(kind);
    try {
      const options = kind === "quality" || kind === "full" ? { max_test: 800, rounds: 3 } : {};
      const job =
        kind === "full"
          ? await api.startFull(options)
          : kind === "fetch"
            ? await api.startFetch(options)
            : kind === "quality"
              ? await api.startQuality(options)
              : await api.startAI(options);
      onStarted?.(job.id);
    } catch (cause) {
      setError(errorMessage(cause, "启动失败"));
    } finally {
      setLoading(null);
    }
  }

  return (
    <section className="space-y-4" aria-label="任务操作">
      <div className="flex flex-wrap gap-2">
        {actions.map((action) => (
          <Button
            key={action.kind}
            variant={action.variant}
            disabled={disabled || loading !== null}
            onClick={() => run(action.kind)}
          >
            <action.icon />
            {loading === action.kind ? "正在启动…" : action.label}
          </Button>
        ))}
      </div>
      {error && (
        <Alert variant="destructive">
          <AlertTitle>任务未启动</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
    </section>
  );
}
