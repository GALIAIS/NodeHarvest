"use client";

import { useCallback, useEffect, useState } from "react";
import { AlertOctagon, CheckCheck, RefreshCw, ShieldCheck } from "lucide-react";
import { api, type AlertRecord } from "@/lib/api";
import { PageHeader } from "@/components/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { formatTime } from "@/lib/utils";

export default function AlertsPage() {
  const [alerts, setAlerts] = useState<AlertRecord[]>([]);
  const [activeOnly, setActiveOnly] = useState(true);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    try {
      setAlerts(await api.alerts(activeOnly));
      setError("");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "加载失败");
    }
  }, [activeOnly]);

  useEffect(() => {
    const initial = setTimeout(load, 0);
    const timer = setInterval(load, 10000);
    return () => {
      clearTimeout(initial);
      clearInterval(timer);
    };
  }, [load]);

  async function change(alert: AlertRecord, action: "acknowledge" | "resolve") {
    setBusy(`${alert.id}:${action}`);
    try {
      await api.changeAlert(alert.id, action);
      await load();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "操作失败");
    } finally {
      setBusy("");
    }
  }

  const critical = alerts.filter((alert) => alert.active && alert.severity === "critical").length;

  return (
    <div className="flex flex-1 flex-col">
      <PageHeader
        eyebrow="Anomaly operations"
        title="异常告警台"
        description="覆盖质量全灭、HQ 骤降、国家集中和连续任务失败。确认代表已接手，解决代表关闭当前异常。"
        actions={
          <>
            <label className="flex items-center gap-2 text-xs text-slate-500">
              <input type="checkbox" className="accent-cyan-400" checked={activeOnly} onChange={(e) => setActiveOnly(e.target.checked)} />
              仅活跃
            </label>
            <Button size="sm" variant="secondary" onClick={load}><RefreshCw className="h-3.5 w-3.5" />刷新</Button>
          </>
        }
      />
      <div className="reveal space-y-4 p-4 sm:p-6 lg:p-8">
        <div className="grid gap-3 sm:grid-cols-3">
          <Card><CardContent className="p-4"><p className="font-mono text-[9px] uppercase tracking-widest text-slate-600">Visible</p><p className="mt-1 text-3xl text-slate-100">{alerts.length}</p></CardContent></Card>
          <Card className={critical ? "border-rose-500/30" : ""}><CardContent className="p-4"><p className="font-mono text-[9px] uppercase tracking-widest text-slate-600">Critical</p><p className="mt-1 text-3xl text-rose-300">{critical}</p></CardContent></Card>
          <Card><CardContent className="p-4"><p className="font-mono text-[9px] uppercase tracking-widest text-slate-600">Acknowledged</p><p className="mt-1 text-3xl text-cyan-200">{alerts.filter((alert) => alert.acknowledged_at).length}</p></CardContent></Card>
        </div>
        {error && <div role="alert" className="rounded-md border border-rose-500/30 bg-rose-500/10 px-4 py-3 text-sm text-rose-300">{error}</div>}
        <div className="space-y-3">
          {alerts.map((alert) => (
            <Card key={alert.id} className={alert.active && alert.severity === "critical" ? "border-rose-500/30" : ""}>
              <CardContent className="grid gap-4 p-4 lg:grid-cols-[auto_minmax(0,1fr)_auto] lg:items-start">
                <span className={`flex h-10 w-10 items-center justify-center rounded-md border ${alert.severity === "critical" ? "border-rose-500/30 bg-rose-500/10 text-rose-300" : "border-amber-500/30 bg-amber-500/10 text-amber-300"}`}>
                  <AlertOctagon className="h-4 w-4" />
                </span>
                <div>
                  <div className="flex flex-wrap items-center gap-2">
                    <h2 className="text-sm font-semibold text-slate-200">{alert.message}</h2>
                    <Badge variant={alert.severity === "critical" ? "danger" : "warn"}>{alert.severity}</Badge>
                    <Badge variant={alert.active ? "default" : "secondary"}>{alert.active ? "ACTIVE" : "RESOLVED"}</Badge>
                  </div>
                  <p className="mt-1 font-mono text-[10px] text-slate-600">{alert.kind} · {formatTime(alert.created_at)}</p>
                  {alert.acknowledged_at && <p className="mt-2 text-xs text-cyan-400/70">由 {alert.acknowledged_by} 于 {formatTime(alert.acknowledged_at)} 确认</p>}
                  {alert.details && (
                    <pre className="mt-3 overflow-auto rounded-md border border-slate-800 bg-slate-950/60 p-3 font-mono text-[10px] text-slate-500">
                      {JSON.stringify(alert.details, null, 2)}
                    </pre>
                  )}
                </div>
                {alert.active && (
                  <div className="flex gap-2">
                    {!alert.acknowledged_at && (
                      <Button size="sm" variant="outline" disabled={busy.startsWith(alert.id)} onClick={() => change(alert, "acknowledge")}>
                        <CheckCheck className="h-3.5 w-3.5" />确认
                      </Button>
                    )}
                    <Button size="sm" variant="secondary" disabled={busy.startsWith(alert.id)} onClick={() => change(alert, "resolve")}>
                      <ShieldCheck className="h-3.5 w-3.5" />解决
                    </Button>
                  </div>
                )}
              </CardContent>
            </Card>
          ))}
          {alerts.length === 0 && <Card><CardContent className="py-20 text-center text-sm text-emerald-400/70"><ShieldCheck className="mx-auto mb-3 h-6 w-6" />当前没有匹配告警</CardContent></Card>}
        </div>
      </div>
    </div>
  );
}
