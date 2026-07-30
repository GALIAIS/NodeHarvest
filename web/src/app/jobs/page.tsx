"use client";

import { useCallback, useEffect, useState } from "react";
import { AuthRequired } from "@/components/auth-required";
import { JobActions } from "@/components/job-actions";
import { useLiveRefresh } from "@/components/live-provider";
import { PaginationControls } from "@/components/pagination-controls";
import { useSession } from "@/components/session-provider";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  api,
  errorMessage,
  type Job,
  type JobEvent,
  type QueuedTask,
} from "@/lib/api";
import { formatTime } from "@/lib/utils";

export default function JobsPage() {
  const { authenticated, loading, canOperate } = useSession();
  const [jobs, setJobs] = useState<Job[]>([]);
  const [tasks, setTasks] = useState<QueuedTask[]>([]);
  const [events, setEvents] = useState<JobEvent[]>([]);
  const [selectedJob, setSelectedJob] = useState("");
  const [queue, setQueue] = useState<{ enabled: boolean; workers?: number; tasks?: Record<string, number> } | null>(null);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [nextCursor, setNextCursor] = useState("");
  const [cursorStack, setCursorStack] = useState<string[]>([]);

  const load = useCallback(async (cursor = "") => {
    if (!authenticated) return;
    try {
      const [jobsPage, queued, queueState] = await Promise.all([
        api.jobs({ limit: 30, cursor: cursor || undefined }),
        api.tasksPage({ limit: 50 }),
        api.queue(),
      ]);
      setJobs(jobsPage.jobs);
      setTasks(queued.tasks);
      setNextCursor(jobsPage.next_cursor || "");
      setQueue(queueState);
      setError("");
    } catch (cause) {
      setError(errorMessage(cause, "加载任务失败"));
    }
  }, [authenticated]);

  useEffect(() => {
    const initial = window.setTimeout(() => {
      setCursorStack([]);
      void load();
    }, 0);
    return () => window.clearTimeout(initial);
  }, [load]);

  const currentCursor = cursorStack[cursorStack.length - 1] || "";
  useLiveRefresh(() => load(currentCursor), authenticated);

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

  async function showEvents(job: Job) {
    setSelectedJob(job.id);
    try {
      setEvents(await api.jobEvents(job.id));
    } catch (cause) {
      setError(errorMessage(cause, "加载任务事件失败"));
    }
  }

  async function cancel(task: QueuedTask) {
    setBusy(task.id);
    try {
      await api.cancelTask(task.id);
      await load(currentCursor);
    } catch (cause) {
      setError(errorMessage(cause, "取消任务失败"));
    } finally {
      setBusy("");
    }
  }

  if (!authenticated && !loading) return <AuthRequired />;

  return (
    <>
      <header className="space-y-1">
        <h1 className="text-3xl font-bold tracking-tight">任务中心</h1>
        <p className="text-muted-foreground">查看运行记录、队列和任务事件。</p>
      </header>
      {error && (
        <Alert variant="destructive">
          <AlertTitle>任务中心不可用</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <Card>
        <CardHeader>
          <CardTitle>队列</CardTitle>
          <CardDescription>
            {queue ? (queue.enabled ? "已启用" : "未启用") + "，工作线程 " + (queue.workers ?? "—") : "加载中…"}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <JobActions
            disabled={!canOperate}
            onStarted={() => {
              setCursorStack([]);
              void load();
            }}
          />
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>运行记录</CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>类型</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>进度</TableHead>
                <TableHead>信息</TableHead>
                <TableHead>更新时间</TableHead>
                <TableHead>操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {jobs.map((job) => (
                <TableRow key={job.id}>
                  <TableCell>{job.type}</TableCell>
                  <TableCell>{job.status}</TableCell>
                  <TableCell>{job.progress + "%"}</TableCell>
                  <TableCell>{job.error || job.message}</TableCell>
                  <TableCell>{formatTime(job.updated_at)}</TableCell>
                  <TableCell>
                    <Button type="button" size="sm" variant="outline" onClick={() => void showEvents(job)}>
                      查看事件
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
              {!jobs.length && (
                <TableRow>
                  <TableCell colSpan={6}>暂无运行记录。</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      <PaginationControls
        page={cursorStack.length + 1}
        count={jobs.length}
        hasNext={Boolean(nextCursor)}
        onPrevious={previousPage}
        onNext={nextPage}
      />
      {selectedJob && (
        <Card>
          <CardHeader>
            <CardTitle>任务事件</CardTitle>
            <CardDescription>{selectedJob}</CardDescription>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>时间</TableHead>
                  <TableHead>等级</TableHead>
                  <TableHead>信息</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {events.map((event) => (
                  <TableRow key={event.id}>
                    <TableCell>{formatTime(event.at)}</TableCell>
                    <TableCell>{event.level}</TableCell>
                    <TableCell>{event.message}</TableCell>
                  </TableRow>
                ))}
                {!events.length && (
                  <TableRow>
                    <TableCell colSpan={3}>暂无事件。</TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}
      <Card>
        <CardHeader>
          <CardTitle>排队任务</CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>类型</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>优先级</TableHead>
                <TableHead>尝试次数</TableHead>
                <TableHead>可执行时间</TableHead>
                <TableHead>操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {tasks.map((task) => (
                <TableRow key={task.id}>
                  <TableCell>{task.type}</TableCell>
                  <TableCell>{task.status}</TableCell>
                  <TableCell>{task.priority}</TableCell>
                  <TableCell>{task.attempts + " / " + task.max_attempts}</TableCell>
                  <TableCell>{formatTime(task.available_at)}</TableCell>
                  <TableCell>
                    <Button
                      type="button"
                      size="sm"
                      variant="destructive"
                      disabled={!canOperate || busy === task.id}
                      onClick={() => void cancel(task)}
                    >
                      取消
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
              {!tasks.length && (
                <TableRow>
                  <TableCell colSpan={6}>队列为空。</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </>
  );
}
