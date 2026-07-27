"use client";

import { useEffect } from "react";
import { usePathname, useRouter } from "next/navigation";
import { useSession } from "@/components/session-provider";

export function AccessGate({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const { authenticated, loading } = useSession();
  const publicRoute = pathname === "/" || pathname === "/login";

  useEffect(() => {
    if (!publicRoute && !loading && !authenticated) router.replace("/");
  }, [authenticated, loading, publicRoute, router]);

  if (publicRoute || authenticated) return <>{children}</>;

  return (
    <div className="flex flex-1 items-center justify-center p-8 font-mono text-xs text-muted-foreground">
      {loading ? "正在验证会话…" : "正在返回仪表盘…"}
    </div>
  );
}
