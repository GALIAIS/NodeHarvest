"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { api } from "@/lib/api";
import { useSession } from "@/components/session-provider";
import { Button } from "@/components/ui/button";

const routes = [
  { href: "/", label: "仪表盘" },
  { href: "/nodes", label: "节点库" },
  { href: "/sources", label: "采集源" },
  { href: "/jobs", label: "任务中心" },
  { href: "/ai", label: "AI 可达" },
  { href: "/export", label: "订阅池" },
  { href: "/sub-store", label: "订阅工坊" },
  { href: "/alerts", label: "异常告警" },
  { href: "/tokens", label: "Token" },
  { href: "/users", label: "用户" },
  { href: "/audit", label: "审计" },
  { href: "/system", label: "系统" },
  { href: "/terms", label: "合规" },
];

export function Sidebar() {
  const pathname = usePathname();
  const { authenticated, session } = useSession();

  if (pathname === "/login") return null;

  return (
    <nav className="border-b bg-background" aria-label="主导航">
      <div className="container mx-auto flex max-w-7xl flex-wrap items-center gap-2 px-4 py-3 md:px-6">
        <Link href="/" className="mr-2 text-lg font-semibold">
          NodeHarvest
        </Link>
        <div className="flex flex-1 flex-wrap gap-2">
          {(authenticated ? routes : routes.slice(0, 1)).map((route) => {
            const active = route.href === "/" ? pathname === "/" : pathname.startsWith(route.href);
            return (
              <Button key={route.href} asChild variant={active ? "secondary" : "ghost"}>
                <Link href={route.href} aria-current={active ? "page" : undefined}>
                  {route.label}
                </Link>
              </Button>
            );
          })}
        </div>
        {authenticated && session ? (
          <div className="flex items-center gap-2">
            <p className="text-sm text-muted-foreground">
              {session.principal.name + " · " + session.principal.role}
            </p>
            <Button
              variant="outline"
              onClick={() => {
                void api.logout().finally(() => window.location.assign("/login"));
              }}
            >
              退出登录
            </Button>
          </div>
        ) : (
          <Button variant="outline" asChild>
            <Link href="/login">管理登录</Link>
          </Button>
        )}
      </div>
    </nav>
  );
}
