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
    <nav aria-label="主导航">
      <p>NodeHarvest</p>
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
      {authenticated && session ? (
        <>
          <p>{session.principal.name + " · " + session.principal.role}</p>
          <Button
            variant="outline"
            onClick={() => {
              void api.logout().finally(() => window.location.assign("/login"));
            }}
          >
            退出登录
          </Button>
        </>
      ) : (
        <Button variant="outline" asChild>
          <Link href="/login">管理登录</Link>
        </Button>
      )}
    </nav>
  );
}
