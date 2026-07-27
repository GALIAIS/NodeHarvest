"use client";

import { createContext, useContext, useEffect, useRef, useState } from "react";
import { type DashboardSnapshot } from "@/lib/api";
import { useSession } from "@/components/session-provider";

type LiveState = {
  dashboard: DashboardSnapshot | null;
  revision: number;
  connected: boolean;
};

const LiveContext = createContext<LiveState>({ dashboard: null, revision: 0, connected: false });

export function LiveProvider({ children }: { children: React.ReactNode }) {
  const { authenticated, loading } = useSession();
  const [dashboard, setDashboard] = useState<DashboardSnapshot | null>(null);
  const [revision, setRevision] = useState(0);
  const [connected, setConnected] = useState(false);

  useEffect(() => {
    if (loading) return;
    const stream = new EventSource(authenticated ? "/api/v1/events" : "/api/public/dashboard/events");
    const onDashboard = (event: Event) => {
      try {
        setDashboard(JSON.parse((event as MessageEvent<string>).data) as DashboardSnapshot);
      } catch {
        // Ignore a malformed update and let EventSource keep the existing connection alive.
      }
    };
    const onRefresh = () => setRevision((current) => current + 1);
    stream.addEventListener("dashboard", onDashboard);
    stream.addEventListener("refresh", onRefresh);
    stream.onopen = () => setConnected(true);
    stream.onerror = () => setConnected(false);
    return () => {
      stream.close();
      setConnected(false);
    };
  }, [authenticated, loading]);

  return (
    <LiveContext.Provider value={{ dashboard, revision, connected }}>
      {children}
    </LiveContext.Provider>
  );
}

export function useLive() {
  return useContext(LiveContext);
}

export function useLiveRefresh(refresh: () => void | Promise<void>, enabled = true) {
  const { revision } = useLive();
  const latest = useRef(refresh);
  const seen = useRef(0);

  useEffect(() => {
    latest.current = refresh;
  }, [refresh]);

  useEffect(() => {
    if (!enabled || revision === 0 || revision === seen.current) return;
    seen.current = revision;
    void latest.current();
  }, [enabled, revision]);
}
