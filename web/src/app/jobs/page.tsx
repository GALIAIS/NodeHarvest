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
import { api, errorMessage, isAuthError, type Job, type JobEvent, type QueuedTask } from "@/lib/api";
import { AuthRequired } from "@/components/auth-required";
import { JobActions } from "@/components/job-actions";
import { PageHeader } from "@/components/page-header";
import { PaginationControls } from "@/components/pagination-controls";
import { useSession } from "@/components/session-provider";
import { useLiveRefresh } from "@/components/live-provider";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardEmpty, CardHeader, CardTitle } from "@/components/ui/card";
import { Hint } from "@/components/ui/hint";
import { Progress } from "@/components/ui/progress";
import { cn, formatTime } from "@/lib/utils";

function statusVariant(status: string): "success" | "warn" | "danger" | "default" | "secondary" {
  if (status === "completed") return "success";
  if (status === "running" || status === "pending" || status === "queued") return "warn";
  if (status === "failed" || status === "dead") return "danger";
  return "secondary";
}

export default function JobsPage() {
  const { authenticated, loading } = useSession();
  const [jobs, setJobs] = useState<Job[]>([]);
  const [tasks, setTasks] = useState<QueuedTask[]>([]);
  const [queue, setQueue] = useState<Record<string, number>>({});
  const [selectedID, setSelectedID] = useState("");
  const [events, setEvents] = useState<JobEvent[]>([]);
  const [err, setErr] = useState<string | null>(null);
  // Set when a request comes back 401/403 mid-session (expired login):
  // swaps the admin content for AuthRequired instead of a red error banner.
  const [authExpired, setAuthExpired] = useState(false);
  const [canceling, setCanceling] = useState("");
  const [nextCursor, setNextCursor] = useState("");
  const [cursorStack, setCursorStack] = useState<string[]>([]);

  const load = useCallback(async (cursor = "") => {
    if (!authenticated) return;
    try {
      const [jobPage, taskList, queueState] = await Promise.all([
        api.jobs({ limit: 30, cursor: cursor || undefined }),
        api.tasksPage({ limit: 50 }),
        api.queue(),
      ]);
      setJobs(jobPage.jobs);
      setTasks(taskList.tasks);
      setQueue(queueState.tasks || {});
      setNextCursor(jobPage.next_cursor || "");
      setSelectedID((current) => current || jobPage.jobs[0]?.id || "");
      setErr(null);
    } catch (cause) {
      if (isAuthError(cause)) {
        setAuthExpired(true);
        setErr(null);
        return;
      }
      setErr(errorMessage(cause, "加载失败"));
    }
  }, [authenticated]);

  const loadEvents = useCallback(async () => {
    if (!authenticated) return;
    if (!selectedID) {
      setEvents([]);
      return;
    }
    try {
      setEvents(await api.jobEvents(selectedID));
    } catch (cause) {
      if (isAuthError(cause)) {
        setAuthExpired(true);
        setErr(null);
        return;
      }
      setErr(errorMessage(cause, "事件流加载失败"));
    }
  }, [authenticated, selectedID]);

  useEffect(() => {
    if (!authenticated || authExpired) return;
    const initial = setTimeout(() => {
      setCursorStack([]);
      void load();
    }, 0);
    return () => {
      clearTimeout(initial);
    };
  }, [load, authenticated, authExpired]);

  useEffect(() => {
    if (!authenticated || authExpired) return;
    const initial = setTimeout(loadEvents, 0);
    return () => {
      clearTimeout(initial);
    };
  }, [loadEvents, authenticated, authExpired]);

  const currentCursor = cursorStack[cursorStack.length - 1] || "";
  useLiveRefresh(() => {
    void load(currentCursor);
    void loadEvents();
  }, authenticated && !authExpired);

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

  const taskByID = useMemo(() => new Map(tasks.map((task) => [task.id, task])), [tasks]);
  const selected = jobs.find((job) => job.id === selectedID);

  async function cancel(id: string) {
    setCanceling(id);
    setErr(null);
    try {
      await api.cancelTask(id);
      await load(currentCursor);
    } catch (cause) {
      if (isAuthError(cause)) {
        setAuthExpired(true);
      } else {
        setErr(errorMessage(cause, "取消失败"));
      }
    } finally {
      setCanceling("");
    }
  }

  // Anonymous (settled) or expired sessions see the sign-in prompt instead of
  // the admin-only queue, launcher, job list and event timeline.
  const needsLogin = (!loading && !authenticated) || authExpired;

  return (
    <div className="flex flex-1 flex-col">
      <PageHeader
        eyebrow="Durable workflow"
        title="任务与事件中心"
        description="任务写入持久化优先级队列，由独立 Worker 租约执行；事件时间线通过 SSE 实时刷新，可追踪重试、失败与取消。"
        actions={
          <Hint content="登录后可用" disabled={needsLogin}>
            <Button variant="secondary" size="sm" onClick={() => { void load(currentCursor); }} disabled={needsLogin}>
              <RefreshCw className="size-3.5" /> 刷新
            </Button>
          </Hint>
        }
      />

      <div className="reveal space-y-4 p-4 sm:p-6 lg:p-8">
        {needsLogin ? (
          <AuthRequired
            title="任务中心需要登录"
            description="任务队列、事件时间线与任务操作都属于管理面；未登录时仅可查看仪表盘。"
          />
        ) : (
          <>
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
            <JobActions onStarted={(id) => { setSelectedID(id); void load(); }} />
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
            <PaginationControls
              page={cursorStack.length + 1}
              count={jobs.length}
              hasNext={Boolean(nextCursor)}
              onPrevious={previousPage}
              onNext={nextPage}
            />
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
          </>
        )}
      </div>
    </div>
  );
}
