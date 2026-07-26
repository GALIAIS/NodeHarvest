"use client";

import { useState } from "react";
import { AlertTriangle, Loader2, Play, Radar, Sparkles, Zap } from "lucide-react";
import { api, errorMessage } from "@/lib/api";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button, type ButtonProps } from "@/components/ui/button";

type Kind = "full" | "fetch" | "quality" | "ai";

const actions: Array<{
  kind: Kind;
  label: string;
  icon: React.ComponentType<{ className?: string }>;
  variant: ButtonProps["variant"];
}> = [
  { kind: "full", label: "一键全流程", icon: Sparkles, variant: "accent" },
  { kind: "fetch", label: "采集", icon: Play, variant: "secondary" },
  { kind: "quality", label: "智能测速", icon: Zap, variant: "outline" },
  { kind: "ai", label: "AI 探测", icon: Radar, variant: "ghost" },
];

export function JobActions({
  onStarted,
  className,
  disabled = false,
}: {
  onStarted?: (jobId: string) => void;
  className?: string;
  disabled?: boolean;
}) {
  const [loading, setLoading] = useState<Kind | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function run(kind: Kind) {
    setError(null);
    setLoading(kind);
    try {
      const opts = kind === "quality" || kind === "full" ? { max_test: 800, rounds: 3 } : {};
      const job =
        kind === "full"
          ? await api.startFull(opts)
          : kind === "fetch"
            ? await api.startFetch(opts)
            : kind === "quality"
              ? await api.startQuality(opts)
              : await api.startAI(opts);
      onStarted?.(job.id);
    } catch (cause) {
      setError(errorMessage(cause, "启动失败"));
    } finally {
      setLoading(null);
    }
  }

  return (
    <div className={className}>
      <div className="flex flex-wrap gap-2">
        {actions.map(({ kind, label, icon: Icon, variant }) => (
          <Button
            key={kind}
            variant={variant}
            onClick={() => run(kind)}
            disabled={disabled || loading !== null}
          >
            {loading === kind ? <Loader2 className="size-4 animate-spin" /> : <Icon className="size-4" />}
            {label}
          </Button>
        ))}
      </div>
      {error && (
        <Alert variant="danger" className="mt-3">
          <AlertTriangle />
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
    </div>
  );
}
