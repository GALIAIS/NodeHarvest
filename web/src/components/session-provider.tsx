"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
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
  // Monotonic ticket: only the newest probe may write state, so a slow response
  // from a previous route (or a manual refresh) can never clobber a newer one.
  const latest = useRef(0);

  // Single place where a probe result is allowed to become state, so the
  // effect and the manual refresh cannot drift apart.
  const commit = useCallback((ticket: number, next: SessionInfo | null) => {
    if (ticket !== latest.current) return;
    setSession(next);
    setLoading(false);
  }, []);

  const probe = useCallback(() => {
    const ticket = ++latest.current;
    return api.me().then(
      (next) => commit(ticket, next),
      () => commit(ticket, null),
    );
  }, [commit]);

  useEffect(() => {
    let cancelled = false;
    const ticket = ++latest.current;
    api.me().then(
      (next) => {
        if (!cancelled) commit(ticket, next);
      },
      () => {
        if (!cancelled) commit(ticket, null);
      },
    );
    return () => {
      cancelled = true;
    };
  }, [commit, pathname]);

  const value = useMemo<SessionState>(() => {
    const authenticated = Boolean(session?.authenticated);
    const role = authenticated && session ? session.principal.role : null;
    return {
      session,
      loading,
      authenticated,
      role,
      canOperate: role === "operator" || role === "admin",
      canAdmin: role === "admin",
      refresh: probe,
    };
  }, [session, loading, probe]);

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession() {
  return useContext(SessionContext);
}
