"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  AlertTriangle,
  Ban,
  CheckCircle2,
  CircleDot,
  RefreshCw,
  TerminalSquare,
  TimerReset,
} from "lucide-react";
import { api, type Job, type JobEvent, type QueuedTask } from "@/lib/api";
import { JobActions } from "@/components/job-actions";
import { PageHeader } from "@/components/page-header";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardEmpty, CardHeader, CardTitle } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { cn, formatTime } from "@/lib/utils";

function statusVariant(status: string): "success" | "warn" | "danger" | "default" | "secondary" {
  if (status === "completed") return "success";
  if (status === "running" || status === "pending" || status === "queued") return "warn";
  if (status === "failed" || status === "dead") return "danger";
  return "secondary";
}

export default function JobsPage() {
  const [jobs, setJobs] = useState<Job[]>([]);
  const [tasks, setTasks] = useState<QueuedTask[]>([]);
  const [queue, setQueue] = useState<Record<string, number>>({});
  const [selectedID, setSelectedID] = useState("");
  const [events, setEvents] = useState<JobEvent[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [canceling, setCanceling] = useState("");

  const load = useCallback(async () => {
    try {
      const [jobPage, taskList, queueState] = await Promise.all([
        api.jobs({ limit: 100 }),
        api.tasks(),
        api.queue(),
      ]);
      setJobs(jobPage.jobs);
      setTasks(taskList);
      setQueue(queueState.tasks || {});
      setSelectedID((current) => current || jobPage.jobs[0]?.id || "");
      setErr(null);
    } catch (cause) {
      setErr(cause instanceof Error ? cause.message : "加载失败");
    }
  }, []);

  const loadEvents = useCallback(async () => {
    if (!selectedID) {
      setEvents([]);
      return;
    }
    try {
      setEvents(await api.jobEvents(selectedID));
    } catch (cause) {
      setErr(cause instanceof Error ? cause.message : "事件流加载失败");
    }
  }, [selectedID]);

  useEffect(() => {
    const initial = setTimeout(load, 0);
    const timer = setInterval(load, 2500);
    return () => {
      clearTimeout(initial);
      clearInterval(timer);
    };
  }, [load]);

  useEffect(() => {
    const initial = setTimeout(loadEvents, 0);
    const timer = setInterval(loadEvents, 1500);
    return () => {
      clearTimeout(initial);
      clearInterval(timer);
    };
  }, [loadEvents]);

  const taskByID = useMemo(() => new Map(tasks.map((task) => [task.id, task])), [tasks]);
  const selected = jobs.find((job) => job.id === selectedID);

  async function cancel(id: string) {
    setCanceling(id);
    setErr(null);
    try {
      await api.cancelTask(id);
      await load();
    } catch (cause) {
      setErr(cause instanceof Error ? cause.message : "取消失败");
    } finally {
      setCanceling("");
    }
  }

  return (
    <div className="flex flex-1 flex-col">
      <PageHeader
        eyebrow="Durable workflow"
        title="任务与事件中心"
        description="任务写入持久化优先级队列，由独立 Worker 租约执行；事件时间线持续轮询，可追踪重试、失败与取消。"
        actions={
          <Button variant="secondary" size="sm" onClick={load}>
            <RefreshCw className="size-3.5" /> 刷新
          </Button>
        }
      />

      <div className="reveal space-y-4 p-4 sm:p-6 lg:p-8">
        <div className="grid gap-3 sm:grid-cols-3 lg:grid-cols-6">
          {["queued", "running", "completed", "failed", "dead", "canceled"].map((status) => (
            <Card key={status}>
              <CardContent className="p-4">
                <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-muted-foreground">
                  {status}
                </p>
                <p className="mt-1 font-display text-2xl tabular-nums text-foreground">
                  {queue[status] || 0}
                </p>
              </CardContent>
            </Card>
          ))}
        </div>

        <Card>
          <CardHeader>
            <CardTitle>启动任务</CardTitle>
            <CardDescription>一键全流程包含采集、质量探测、Geo/ASN、真实协议抽样、AI 探测与发布</CardDescription>
          </CardHeader>
          <CardContent>
            <JobActions onStarted={(id) => { setSelectedID(id); load(); }} />
          </CardContent>
        </Card>

        {err && (
          <Alert variant="danger">
            <AlertTriangle />
            <AlertDescription>{err}</AlertDescription>
          </Alert>
        )}

        <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(360px,.75fr)]">
          <div className="space-y-2">
            {jobs.map((job) => {
              const task = taskByID.get(job.id);
              const running = ["running", "pending", "queued"].includes(job.status) ||
                ["running", "queued"].includes(task?.status || "");
              return (
                <button
                  type="button"
                  key={job.id}
                  onClick={() => setSelectedID(job.id)}
                  className={cn(
                    "w-full border border-border border-l-2 bg-card p-4 text-left transition-colors",
                    selectedID === job.id
                      ? "border-l-primary bg-primary/5"
                      : "border-l-transparent hover:border-input",
                  )}
                >
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div>
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge variant={statusVariant(task?.status || job.status)}>{task?.status || job.status}</Badge>
                        <span className="font-mono text-sm text-primary">{job.type}</span>
                        {task && (
                          <span className="font-mono text-[10px] text-muted-foreground">
                            P{task.priority} · {task.attempts}/{task.max_attempts}
                          </span>
                        )}
                      </div>
                      <p className="mt-2 text-sm text-foreground">{job.message || "等待执行"}</p>
                    </div>
                    <p className="font-mono text-[10px] text-muted-foreground">{formatTime(job.created_at)}</p>
                  </div>
                  {running && <Progress className="mt-3" value={job.progress} />}
                  {job.error && <p className="mt-2 text-xs text-destructive">{job.error}</p>}
                </button>
              );
            })}
            {jobs.length === 0 && (
              <Card><CardEmpty>还没有任务记录</CardEmpty></Card>
            )}
          </div>

          <Card className="h-fit xl:sticky xl:top-4">
            <CardHeader className="flex-row items-start justify-between gap-3">
              <div className="min-w-0">
                <CardTitle className="flex items-center gap-2">
                  <TerminalSquare className="size-4 text-primary" /> 事件时间线
                </CardTitle>
                <CardDescription className="mt-1 truncate font-mono">{selectedID || "选择一个任务"}</CardDescription>
              </div>
              {selected && ["running", "pending"].includes(selected.status) && (
                <Button
                  size="sm"
                  variant="destructive"
                  disabled={canceling === selected.id}
                  onClick={() => cancel(selected.id)}
                >
                  <Ban className="size-3.5" /> 取消
                </Button>
              )}
            </CardHeader>
            <CardContent>
              {selected && (
                <div className="mb-4 grid grid-cols-3 gap-2 border-b border-border pb-4 text-center">
                  <div><p className="text-[10px] text-muted-foreground">状态</p><p className="mt-1 text-xs text-foreground">{selected.status}</p></div>
                  <div><p className="text-[10px] text-muted-foreground">进度</p><p className="mt-1 font-mono text-xs tabular-nums text-foreground">{Math.round(selected.progress)}%</p></div>
                  <div><p className="text-[10px] text-muted-foreground">事件</p><p className="mt-1 font-mono text-xs tabular-nums text-foreground">{events.length}</p></div>
                </div>
              )}
              <div className="max-h-[36rem] space-y-0 overflow-y-auto pr-1">
                {events.map((event, index) => (
                  <div key={event.id} className="relative grid grid-cols-[1.25rem_1fr] gap-3 pb-4">
                    {index < events.length - 1 && <span className="absolute bottom-0 left-[9px] top-4 w-px bg-border" />}
                    <span className="relative z-10 mt-0.5 flex size-[18px] items-center justify-center border border-border bg-popover">
                      {event.level === "error" ? (
                        <TimerReset className="size-2.5 text-destructive" />
                      ) : event.level === "done" ? (
                        <CheckCircle2 className="size-2.5 text-success" />
                      ) : (
                        <CircleDot className="size-2.5 text-primary" />
                      )}
                    </span>
                    <div>
                      <p className={cn("text-xs leading-5", event.level === "error" ? "text-destructive" : "text-foreground")}>{event.message}</p>
                      <p className="font-mono text-[9px] text-muted-foreground">{formatTime(event.at)} · {event.level}</p>
                    </div>
                  </div>
                ))}
                {events.length === 0 && (
                  <p className="py-16 text-center text-xs text-muted-foreground">
                    {selectedID ? "等待任务事件…" : "从左侧选择任务"}
                  </p>
                )}
              </div>
              {selected?.stats && Object.keys(selected.stats).length > 0 && (
                <details className="mt-3 border-t border-border pt-3">
                  <summary className="cursor-pointer text-xs text-muted-foreground">结果统计</summary>
                  <pre className="mt-2 overflow-auto border border-border bg-popover p-3 font-mono text-[10px] text-muted-foreground">
                    {JSON.stringify(selected.stats, null, 2)}
                  </pre>
                </details>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
