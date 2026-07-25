"use client";

import { useCallback, useEffect, useState } from "react";
import { api, type Job } from "@/lib/api";
import { JobActions } from "@/components/job-actions";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { formatTime } from "@/lib/utils";

function statusVariant(s: string): "success" | "warn" | "danger" | "default" | "secondary" {
  if (s === "completed") return "success";
  if (s === "running" || s === "pending") return "warn";
  if (s === "failed") return "danger";
  return "secondary";
}

export default function JobsPage() {
  const [jobs, setJobs] = useState<Job[]>([]);
  const [err, setErr] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      setJobs(await api.jobs());
      setErr(null);
    } catch (e) {
      setErr(e instanceof Error ? e.message : "加载失败");
    }
  }, []);

  useEffect(() => {
    load();
    const t = setInterval(load, 2500);
    return () => clearInterval(t);
  }, [load]);

  return (
    <div className="flex flex-1 flex-col">
      <header className="border-b border-slate-800/80 px-8 py-5">
        <h1 className="font-[family-name:var(--font-display)] text-xl font-semibold">
          任务中心
        </h1>
        <p className="mt-1 text-sm text-slate-500">
          同时仅允许一个重任务，避免测速风暴
        </p>
      </header>

      <div className="space-y-6 p-8">
        <Card>
          <CardHeader>
            <CardTitle>启动任务</CardTitle>
            <CardDescription>
              智能测速默认多轮 TCP + TLS；AI 探测默认启发模式，配置 SOCKS5 可真实代理测
            </CardDescription>
          </CardHeader>
          <CardContent>
            <JobActions onStarted={() => load()} />
          </CardContent>
        </Card>

        {err && (
          <div className="rounded-lg border border-rose-500/30 bg-rose-500/10 px-4 py-3 text-sm text-rose-300">
            {err}
          </div>
        )}

        <div className="space-y-3">
          {jobs.map((j) => (
            <Card key={j.id}>
              <CardContent className="space-y-3 p-5">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="flex items-center gap-2">
                    <Badge variant={statusVariant(j.status)}>{j.status}</Badge>
                    <span className="font-mono text-sm text-cyan-200">{j.type}</span>
                    <span className="text-xs text-slate-500">{j.id}</span>
                  </div>
                  <div className="text-xs text-slate-500">
                    {formatTime(j.created_at)} → {formatTime(j.ended_at || j.updated_at)}
                  </div>
                </div>
                <p className="text-sm text-slate-300">{j.message}</p>
                {(j.status === "running" || j.status === "pending") && (
                  <Progress value={j.progress} />
                )}
                {j.error && (
                  <p className="text-xs text-rose-400">{j.error}</p>
                )}
                {j.stats && Object.keys(j.stats).length > 0 && (
                  <pre className="overflow-auto rounded-lg bg-slate-950/80 p-3 font-mono text-[11px] text-slate-400">
                    {JSON.stringify(j.stats, null, 2)}
                  </pre>
                )}
              </CardContent>
            </Card>
          ))}
          {jobs.length === 0 && (
            <Card>
              <CardContent className="py-12 text-center text-slate-500">
                还没有任务记录
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </div>
  );
}
