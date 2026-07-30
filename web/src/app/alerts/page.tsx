"use client";

import { useCallback, useEffect, useState } from "react";
import { RefreshCw } from "lucide-react";
import { AuthRequired } from "@/components/auth-required";
import { useLiveRefresh } from "@/components/live-provider";
import { PageHeader } from "@/components/page-header";
import { PaginationControls } from "@/components/pagination-controls";
import { useSession } from "@/components/session-provider";
import { StatusBadge } from "@/components/status-badge";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { api, errorMessage, type AlertRecord } from "@/lib/api";
import { formatTime } from "@/lib/utils";

export default function AlertsPage() {
  const { authenticated, loading, canAdmin } = useSession();
  const [alerts, setAlerts] = useState<AlertRecord[]>([]);
  const [activeOnly, setActiveOnly] = useState(true);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [total, setTotal] = useState(0);
  const [nextCursor, setNextCursor] = useState("");
  const [cursorStack, setCursorStack] = useState<string[]>([]);

  const load = useCallback(async (cursor = "") => {
    if (!canAdmin) return;
    try {
      const page = await api.alertsPage({ active: activeOnly, cursor: cursor || undefined });
      setAlerts(page.alerts);
      setTotal(page.total ?? 0);
      setNextCursor(page.next_cursor || "");
      setError("");
    } catch (cause) {
      setError(errorMessage(cause, "加载告警失败"));
    }
  }, [activeOnly, canAdmin]);

  useEffect(() => {
    const initial = window.setTimeout(() => {
      setCursorStack([]);
      void load();
    }, 0);
    return () => window.clearTimeout(initial);
  }, [load]);

  const currentCursor = cursorStack[cursorStack.length - 1] || "";
  useLiveRefresh(() => load(currentCursor), canAdmin);

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

  async function change(alert: AlertRecord, action: "acknowledge" | "resolve") {
    setBusy(alert.id + ":" + action);
    try {
      await api.changeAlert(alert.id, action);
      await load(currentCursor);
    } catch (cause) {
      setError(errorMessage(cause, "更新告警失败"));
    } finally {
      setBusy("");
    }
  }

  if (!authenticated && !loading) return <AuthRequired />;
  if (authenticated && !canAdmin) return <AuthRequired reason="forbidden" />;

  return (
    <>
      <PageHeader
        title="异常告警"
        description={"查看、确认并解决系统异常，共 " + total + " 条。"}
        actions={
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              setCursorStack([]);
              void load();
            }}
          >
            <RefreshCw />
            刷新
          </Button>
        }
      />
      {error && (
        <Alert variant="destructive">
          <AlertTitle>告警操作失败</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <Card>
        <CardHeader>
          <CardTitle>筛选</CardTitle>
          <CardDescription>选择显示活动告警或全部告警。</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2">
            <Switch
              id="alerts-active-only"
              checked={activeOnly}
              onCheckedChange={setActiveOnly}
            />
            <Label htmlFor="alerts-active-only">仅活动告警</Label>
          </div>
        </CardContent>
      </Card>
      {alerts.map((alert) => (
        <Card key={alert.id}>
          <CardHeader>
            <CardTitle>{alert.message}</CardTitle>
            <CardDescription>{alert.kind + " · " + formatTime(alert.created_at)}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex flex-wrap gap-2">
              <StatusBadge status={alert.severity} />
              <StatusBadge status={alert.active ? "active" : "inactive"}>
                {alert.active ? "活动" : "已解决"}
              </StatusBadge>
            </div>
            {alert.acknowledged_at && <p>{"已由 " + (alert.acknowledged_by || "未知用户") + " 确认。"}</p>}
            {alert.details && (
              <pre className="overflow-x-auto rounded-md bg-muted p-4 text-xs">
                {JSON.stringify(alert.details, null, 2)}
              </pre>
            )}
            {alert.active && (
              <div className="flex flex-wrap gap-2">
                {!alert.acknowledged_at && (
                  <Button
                    type="button"
                    variant="outline"
                    disabled={busy.startsWith(alert.id)}
                    onClick={() => void change(alert, "acknowledge")}
                  >
                    确认
                  </Button>
                )}
                <Button
                  type="button"
                  variant="destructive"
                  disabled={busy.startsWith(alert.id)}
                  onClick={() => void change(alert, "resolve")}
                >
                  解决
                </Button>
              </div>
            )}
          </CardContent>
        </Card>
      ))}
      {!alerts.length && (
        <Card>
          <CardContent className="py-12 text-center text-muted-foreground">
            没有匹配的告警。
          </CardContent>
        </Card>
      )}
      <PaginationControls
        page={cursorStack.length + 1}
        total={total}
        count={alerts.length}
        hasNext={Boolean(nextCursor)}
        onPrevious={previousPage}
        onNext={nextPage}
      />
    </>
  );
}
