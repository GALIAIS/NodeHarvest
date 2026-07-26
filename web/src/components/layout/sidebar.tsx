"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  Activity,
  BellRing,
  Bot,
  Download,
  FileClock,
  KeyRound,
  LayoutDashboard,
  Lock,
  LogIn,
  LogOut,
  Network,
  Radio,
  Scale,
  Server,
  Settings2,
  ShieldCheck,
  Users,
} from "lucide-react";
import { api } from "@/lib/api";
import { useSession } from "@/components/session-provider";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

// `admin: true` marks views whose data comes from /api/admin/* or /api/jobs,
// which return 401 to anonymous visitors — the nav shows a lock so the
// restriction is visible before navigating rather than after.
const groups = [
  {
    label: "Observe",
    items: [
      { href: "/", label: "仪表盘", icon: LayoutDashboard },
      { href: "/nodes", label: "节点库", icon: Network },
      { href: "/sources", label: "采集源", icon: Server },
      { href: "/jobs", label: "任务中心", icon: Activity, admin: true },
      { href: "/ai", label: "AI 可达", icon: Bot },
      { href: "/export", label: "订阅池", icon: Download },
    ],
  },
  {
    label: "Govern",
    items: [
      { href: "/alerts", label: "异常告警", icon: BellRing, admin: true },
      { href: "/tokens", label: "Token", icon: KeyRound, admin: true },
      { href: "/users", label: "用户", icon: Users, admin: true },
      { href: "/audit", label: "审计", icon: FileClock, admin: true },
      { href: "/system", label: "系统", icon: Settings2 },
      { href: "/terms", label: "合规", icon: Scale },
    ],
  },
];

function NavItem({
  href,
  label,
  icon: Icon,
  active,
  compact,
  locked,
}: {
  href: string;
  label: string;
  icon: React.ComponentType<{ className?: string }>;
  active: boolean;
  compact?: boolean;
  locked?: boolean;
}) {
  return (
    <Link
      href={href}
      aria-current={active ? "page" : undefined}
      title={locked ? `${label}（需要登录）` : undefined}
      className={cn(
        // active state is a solid inset rail on the left edge — no rounded pill
        "group flex items-center gap-2.5 border-l-2 px-3 py-2 text-sm transition-colors",
        compact && "shrink-0 border-l-0 border-b-2 px-2.5 py-1.5 text-xs",
        active
          ? "border-primary bg-primary/10 text-primary"
          : "border-transparent text-muted-foreground hover:bg-muted hover:text-foreground",
      )}
    >
      <Icon className={cn("size-4", active ? "text-primary" : "text-muted-foreground/70 group-hover:text-foreground")} />
      {label}
      {locked && !compact && (
        <Lock className="ml-auto size-3 text-muted-foreground/60" aria-hidden />
      )}
    </Link>
  );
}

export function Sidebar() {
  const pathname = usePathname();
  const { session, authenticated, loading } = useSession();

  if (pathname === "/login") return null;

  const active = (href: string) => (href === "/" ? pathname === "/" : pathname.startsWith(href));

  return (
    <>
      <aside className="relative z-20 hidden w-64 shrink-0 flex-col border-r border-border bg-card/90 backdrop-blur-xl md:flex">
        <div className="border-b border-border px-5 py-5">
          <Link href="/" className="flex items-center gap-3">
            <span className="corner-ticks relative flex size-10 items-center justify-center border border-primary/40 bg-primary/10">
              <Radio className="size-4 text-primary" />
              <span className="absolute inset-x-0 bottom-0 h-0.5 bg-accent" />
            </span>
            <span className="block">
              <span className="block font-display text-sm font-semibold tracking-[0.1em] text-foreground">
                NODEHARVEST
              </span>
              <span className="block font-mono text-[9px] uppercase tracking-[0.2em] text-muted-foreground">
                Network operations
              </span>
            </span>
          </Link>
        </div>

        <nav className="flex flex-1 flex-col gap-5 overflow-y-auto py-4">
          {groups.map((group) => (
            <div key={group.label}>
              <div className="mb-2 flex items-center gap-2 px-3">
                <span className="font-mono text-[9px] uppercase tracking-[0.24em] text-muted-foreground">
                  {group.label}
                </span>
                <Separator className="flex-1" />
              </div>
              <div>
                {group.items.map(({ admin, ...item }) => (
                  <NavItem
                    key={item.href}
                    {...item}
                    active={active(item.href)}
                    locked={Boolean(admin) && !authenticated && !loading}
                  />
                ))}
              </div>
            </div>
          ))}
        </nav>

        <div className="border-t border-border p-3">
          {authenticated && session ? (
            <div className="flex items-center justify-between gap-2 border border-border bg-muted/50 p-3">
              <div className="min-w-0">
                <p className="truncate text-xs font-semibold text-foreground">
                  {session.principal.name}
                </p>
                <p className="mt-0.5 truncate font-mono text-[9px] uppercase tracking-wider text-muted-foreground/70">
                  {session.principal.tenant_id} · {session.principal.role}
                </p>
              </div>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label="退出登录"
                    onClick={() => {
                      // A full navigation remounts the provider, so refreshing
                      // the session here first would only add a round trip.
                      // api.logout() clears the tab-local admin token even if
                      // the request itself fails.
                      void api.logout().finally(() => window.location.assign("/login"));
                    }}
                    className="hover:text-destructive"
                  >
                    <LogOut className="size-4" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent side="top">退出登录</TooltipContent>
              </Tooltip>
            </div>
          ) : (
            <Button variant="outline" asChild className="w-full justify-between border-accent/40 text-accent hover:border-accent hover:bg-accent/10">
              <Link href="/login">
                <span className="flex items-center gap-2">
                  <LogIn className="size-4" /> 管理登录
                </span>
                <ShieldCheck className="size-4 opacity-60" />
              </Link>
            </Button>
          )}
          <p className="mt-3 px-1 font-mono text-[9px] uppercase tracking-[0.16em] text-muted-foreground">
            Quality first · trace everything
          </p>
        </div>
      </aside>

      <div className="sticky top-0 z-30 border-b border-border bg-card/95 backdrop-blur-xl md:hidden">
        <div className="flex items-center justify-between px-3 py-2.5">
          <Link href="/" className="flex items-center gap-2 font-display text-xs font-semibold tracking-[0.1em] text-foreground">
            <Radio className="size-4 text-primary" /> NODEHARVEST
          </Link>
          <Link href="/login" aria-label="管理登录" className="text-muted-foreground">
            <ShieldCheck className="size-4" />
          </Link>
        </div>
        <nav className="flex gap-1 overflow-x-auto border-t border-border px-2">
          {groups.flatMap((group) =>
            group.items.map((item) => (
              <NavItem
                key={item.href}
                href={item.href}
                label={item.label}
                icon={item.icon}
                active={active(item.href)}
                compact
              />
            )),
          )}
        </nav>
      </div>
    </>
  );
}
