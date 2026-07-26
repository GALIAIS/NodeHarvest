"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import { usePathname } from "next/navigation";
import { api, type SessionInfo } from "@/lib/api";

type SessionState = {
  session: SessionInfo | null;
  /** True until the first /auth/me response settles — pages should not render
   *  a "please sign in" state while this is pending, or it flashes on reload. */
  loading: boolean;
  authenticated: boolean;
  role: SessionInfo["principal"]["role"] | null;
  /** operator and admin may start jobs and mutate sources. */
  canOperate: boolean;
  canAdmin: boolean;
  refresh: () => Promise<void>;
};

const SessionContext = createContext<SessionState>({
  session: null,
  loading: true,
  authenticated: false,
  role: null,
  canOperate: false,
  canAdmin: false,
  refresh: async () => {},
});

export function SessionProvider({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const [session, setSession] = useState<SessionInfo | null>(null);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    try {
      setSession(await api.me());
    } catch {
      setSession(null);
    } finally {
      setLoading(false);
    }
  }, []);

  // Re-probe on navigation. State only ever lands in a promise callback, and the
  // cancelled flag keeps a stale response from overwriting a newer one.
  useEffect(() => {
    let cancelled = false;
    api
      .me()
      .then((next) => {
        if (!cancelled) setSession(next);
      })
      .catch(() => {
        if (!cancelled) setSession(null);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [pathname]);

  const value = useMemo<SessionState>(() => {
    const authenticated = Boolean(session?.authenticated);
    const role = authenticated ? session!.principal.role : null;
    return {
      session,
      loading,
      authenticated,
      role,
      canOperate: role === "operator" || role === "admin",
      canAdmin: role === "admin",
      refresh,
    };
  }, [session, loading, refresh]);

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession() {
  return useContext(SessionContext);
}
