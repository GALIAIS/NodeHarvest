"use client";

import { useCallback, useEffect, useState } from "react";
import { AlertOctagon, AlertTriangle, CheckCheck, RefreshCw, ShieldCheck } from "lucide-react";
import { api, errorMessage, isForbidden, isUnauthenticated, type AlertRecord } from "@/lib/api";
import { AuthRequired } from "@/components/auth-required";
import { PageHeader } from "@/components/page-header";
import { PaginationControls } from "@/components/pagination-controls";
import { useSession } from "@/components/session-provider";
import { useLiveRefresh } from "@/components/live-provider";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardEmpty } from "@/components/ui/card";
import { Hint } from "@/components/ui/hint";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { cn, formatTime } from "@/lib/utils";

export default function AlertsPage() {
  const { authenticated, loading: sessionLoading } = useSession();
  const [alerts, setAlerts] = useState<AlertRecord[]>([]);
  const [activeOnly, setActiveOnly] = useState(true);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [authBlocked, setAuthBlocked] = useState<null | "anonymous" | "forbidden">(null);
  const [total, setTotal] = useState(0);
  const [nextCursor, setNextCursor] = useState("");
  const [cursorStack, setCursorStack] = useState<string[]>([]);

  const load = useCallback(async (cursor = "") => {
    // The alerts feed is admin-only; anonymous requests would just 401.
    if (!authenticated) return;
    try {
      const page = await api.alertsPage({ active: activeOnly, cursor: cursor || undefined });
      setAlerts(page.alerts);
      setTotal(page.total ?? 0);
      setNextCursor(page.next_cursor || "");
      setError("");
      setAuthBlocked(null);
    } catch (cause) {
      if (isForbidden(cause)) {
        setAuthBlocked("forbidden");
        setError("");
      } else if (isUnauthenticated(cause)) {
        setAuthBlocked("anonymous");
        setError("");
      } else {
        setError(errorMessage(cause, "加载失败"));
      }
    }
  }, [activeOnly, authenticated]);

  useEffect(() => {
    if (!authenticated || authBlocked) return;
    const initial = setTimeout(() => {
      setCursorStack([]);
      void load();
    }, 0);
    return () => {
      clearTimeout(initial);
    };
  }, [load, authenticated, authBlocked]);

  const currentCursor = cursorStack[cursorStack.length - 1] || "";
  useLiveRefresh(() => load(currentCursor), authenticated && !authBlocked);

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
    setBusy(`${alert.id}:${action}`);
    try {
      await api.changeAlert(alert.id, action);
      await load(currentCursor);
    } catch (cause) {
      if (isForbidden(cause)) {
        setAuthBlocked("forbidden");
        setError("");
      } else if (isUnauthenticated(cause)) {
        setAuthBlocked("anonymous");
        setError("");
      } else {
        setError(errorMessage(cause, "操作失败"));
      }
    } finally {
      setBusy("");
    }
  }

  const needsLogin = (!authenticated && !sessionLoading) || authBlocked !== null;

  const critical = alerts.filter((alert) => alert.active && alert.severity === "critical").length;

  return (
    <div className="flex flex-1 flex-col">
      <PageHeader
        eyebrow="Anomaly operations"
        title="异常告警台"
        description="覆盖质量全灭、HQ 骤降、国家集中和连续任务失败。确认代表已接手，解决代表关闭当前异常。"
        actions={
          <>
            <div className="flex items-center gap-2">
              <Hint content="登录后可用" disabled={!authenticated}>
                <Switch
                  id="alerts-active-only"
                  checked={activeOnly}
                  onCheckedChange={setActiveOnly}
                  disabled={!authenticated}
                />
              </Hint>
              <Label htmlFor="alerts-active-only">仅活跃</Label>
            </div>
            <Hint content="登录后可用" disabled={!authenticated}>
              <Button size="sm" variant="secondary" onClick={() => { void load(currentCursor); }} disabled={!authenticated}>
                <RefreshCw className="size-3.5" />
                刷新
              </Button>
            </Hint>
          </>
        }
      />

      <div className="reveal space-y-4 p-4 sm:p-6 lg:p-8">
        {needsLogin ? (
          authBlocked === "forbidden" ? (
            <AuthRequired reason="forbidden" />
          ) : (
            <AuthRequired
              title="告警台需要登录"
              description="异常告警与确认/解决操作属于管理面，登录后可查看。"
            />
          )
        ) : (
          <>
            <div className="grid gap-3 sm:grid-cols-3">
              <Card>
                <CardContent className="p-4">
                  <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-muted-foreground">
                    Visible
                  </p>
                  <p className="mt-1 font-display text-3xl tabular-nums text-foreground">
                    {alerts.length}
                  </p>
                </CardContent>
              </Card>
              <Card className={critical ? "border-destructive/40" : undefined}>
                <CardContent className="p-4">
                  <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-muted-foreground">
                    Critical
                  </p>
                  <p className="mt-1 font-display text-3xl tabular-nums text-destructive">
                    {critical}
                  </p>
                </CardContent>
              </Card>
              <Card>
                <CardContent className="p-4">
                  <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-muted-foreground">
                    Acknowledged
                  </p>
                  <p className="mt-1 font-display text-3xl tabular-nums text-primary">
                    {alerts.filter((alert) => alert.acknowledged_at).length}
                  </p>
                </CardContent>
              </Card>
            </div>

            {error && (
              <Alert variant="danger">
                <AlertTriangle />
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}

            <div className="space-y-3">
              {alerts.map((alert) => (
                <Card
                  key={alert.id}
                  className={cn(
                    "border-l-2",
                    alert.active && alert.severity === "critical" && "border-destructive/30",
                    alert.severity === "critical" ? "border-l-destructive" : "border-l-accent",
                  )}
                >
                  <CardContent className="grid gap-4 p-4 lg:grid-cols-[auto_minmax(0,1fr)_auto] lg:items-start">
                    <span
                      className={cn(
                        "flex size-10 items-center justify-center border",
                        alert.severity === "critical"
                          ? "border-destructive/30 bg-destructive/10 text-destructive"
                          : "border-accent/30 bg-accent/10 text-accent",
                      )}
                    >
                      <AlertOctagon className="size-4" />
                    </span>
                    <div>
                      <div className="flex flex-wrap items-center gap-2">
                        <h2 className="text-sm font-semibold text-foreground">{alert.message}</h2>
                        <Badge variant={alert.severity === "critical" ? "danger" : "warn"}>
                          {alert.severity}
                        </Badge>
                        <Badge variant={alert.active ? "default" : "secondary"}>
                          {alert.active ? "ACTIVE" : "RESOLVED"}
                        </Badge>
                      </div>
                      <p className="mt-1 font-mono text-[10px] text-muted-foreground">
                        {alert.kind} · {formatTime(alert.created_at)}
                      </p>
                      {alert.acknowledged_at && (
                        <p className="mt-2 text-xs text-primary/70">
                          由 {alert.acknowledged_by} 于 {formatTime(alert.acknowledged_at)} 确认
                        </p>
                      )}
                      {alert.details && (
                        <pre className="mt-3 overflow-auto border border-border bg-popover p-3 font-mono text-[10px] text-muted-foreground">
                          {JSON.stringify(alert.details, null, 2)}
                        </pre>
                      )}
                    </div>
                    {alert.active && (
                      <div className="flex gap-2">
                        {!alert.acknowledged_at && (
                          <Button
                            size="sm"
                            variant="outline"
                            disabled={busy.startsWith(alert.id)}
                            onClick={() => change(alert, "acknowledge")}
                          >
                            <CheckCheck className="size-3.5" />
                            确认
                          </Button>
                        )}
                        <Button
                          size="sm"
                          variant="secondary"
                          disabled={busy.startsWith(alert.id)}
                          onClick={() => change(alert, "resolve")}
                        >
                          <ShieldCheck className="size-3.5" />
                          解决
                        </Button>
                      </div>
                    )}
                  </CardContent>
                </Card>
              ))}
              {alerts.length === 0 && (
                <Card>
                  <CardEmpty icon={ShieldCheck} className="text-success/80">
                    当前没有匹配告警
                  </CardEmpty>
                </Card>
              )}
              <PaginationControls
                page={cursorStack.length + 1}
                total={total}
                count={alerts.length}
                hasNext={Boolean(nextCursor)}
                onPrevious={previousPage}
                onNext={nextPage}
                disabled={Boolean(busy)}
              />
            </div>
          </>
        )}
      </div>
    </div>
  );
}
