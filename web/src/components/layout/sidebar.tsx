"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import {
  Activity,
  BellRing,
  Bot,
  Download,
  FileClock,
  Gauge,
  KeyRound,
  LayoutDashboard,
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
import { api, type SessionInfo } from "@/lib/api";
import { cn } from "@/lib/utils";

const groups = [
  {
    label: "Observe",
    items: [
      { href: "/", label: "仪表盘", icon: LayoutDashboard },
      { href: "/nodes", label: "节点库", icon: Network },
      { href: "/sources", label: "采集源", icon: Server },
      { href: "/jobs", label: "任务中心", icon: Activity },
      { href: "/ai", label: "AI 可达", icon: Bot },
      { href: "/export", label: "订阅池", icon: Download },
    ],
  },
  {
    label: "Govern",
    items: [
      { href: "/alerts", label: "异常告警", icon: BellRing },
      { href: "/tokens", label: "Token", icon: KeyRound },
      { href: "/users", label: "用户", icon: Users },
      { href: "/audit", label: "审计", icon: FileClock },
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
}: {
  href: string;
  label: string;
  icon: React.ComponentType<{ className?: string }>;
  active: boolean;
  compact?: boolean;
}) {
  return (
    <Link
      href={href}
      className={cn(
        "group flex items-center gap-2.5 rounded-md border px-3 py-2 text-sm transition-all",
        compact && "shrink-0 py-1.5 text-xs",
        active
          ? "border-cyan-500/25 bg-cyan-500/10 text-cyan-100 shadow-[inset_3px_0_0_#22d3ee]"
          : "border-transparent text-slate-500 hover:border-slate-800 hover:bg-slate-900/80 hover:text-slate-200",
      )}
    >
      <Icon className={cn("h-4 w-4", active ? "text-cyan-300" : "text-slate-600 group-hover:text-slate-400")} />
      {label}
    </Link>
  );
}

export function Sidebar() {
  const pathname = usePathname();
  const [session, setSession] = useState<SessionInfo | null>(null);

  useEffect(() => {
    api.me().then(setSession).catch(() => setSession(null));
  }, [pathname]);

  if (pathname === "/login") return null;

  const active = (href: string) => (href === "/" ? pathname === "/" : pathname.startsWith(href));

  return (
    <>
      <aside className="relative z-20 hidden w-64 shrink-0 flex-col border-r border-slate-800/80 bg-slate-950/85 backdrop-blur-xl md:flex">
        <div className="border-b border-slate-800/80 px-5 py-5">
          <Link href="/" className="flex items-center gap-3">
            <div className="relative flex h-10 w-10 items-center justify-center overflow-hidden rounded-md border border-cyan-400/30 bg-cyan-400/10">
              <Radio className="h-4 w-4 text-cyan-300" />
              <span className="absolute inset-x-0 bottom-0 h-0.5 bg-amber-400" />
            </div>
            <div>
              <div className="font-[family-name:var(--font-display)] text-sm font-semibold tracking-[0.08em] text-slate-50">
                NODEHARVEST
              </div>
              <div className="font-mono text-[9px] uppercase tracking-[0.18em] text-slate-600">
                Network operations
              </div>
            </div>
          </Link>
        </div>

        <nav className="flex flex-1 flex-col gap-5 overflow-y-auto p-3">
          {groups.map((group) => (
            <div key={group.label}>
              <p className="mb-1.5 px-3 font-mono text-[9px] uppercase tracking-[0.22em] text-slate-700">
                {group.label}
              </p>
              <div className="space-y-0.5">
                {group.items.map((item) => (
                  <NavItem key={item.href} {...item} active={active(item.href)} />
                ))}
              </div>
            </div>
          ))}
        </nav>

        <div className="border-t border-slate-800/80 p-3">
          {session?.authenticated ? (
            <div className="rounded-lg border border-slate-800 bg-slate-900/55 p-3">
              <div className="flex items-center justify-between gap-2">
                <div className="min-w-0">
                  <p className="truncate text-xs font-semibold text-slate-200">{session.principal.name}</p>
                  <p className="mt-0.5 font-mono text-[9px] uppercase tracking-wider text-slate-600">
                    {session.principal.tenant_id} · {session.principal.role}
                  </p>
                </div>
                <button
                  type="button"
                  aria-label="退出登录"
                  onClick={() => api.logout().then(() => window.location.assign("/login"))}
                  className="rounded-md p-2 text-slate-600 transition-colors hover:bg-slate-800 hover:text-rose-300"
                >
                  <LogOut className="h-4 w-4" />
                </button>
              </div>
            </div>
          ) : (
            <Link
              href="/login"
              className="flex items-center justify-between rounded-lg border border-amber-500/20 bg-amber-500/5 p-3 text-xs text-amber-200"
            >
              <span className="flex items-center gap-2">
                <LogIn className="h-4 w-4" /> 管理登录
              </span>
              <ShieldCheck className="h-4 w-4 text-amber-400/60" />
            </Link>
          )}
          <div className="mt-3 flex items-center gap-2 px-1 font-mono text-[9px] uppercase tracking-wider text-slate-700">
            <Gauge className="h-3 w-3" />
            Quality first · trace everything
          </div>
        </div>
      </aside>

      <div className="sticky top-0 z-30 border-b border-slate-800 bg-slate-950/95 px-3 py-2 backdrop-blur-xl md:hidden">
        <div className="mb-2 flex items-center justify-between">
          <Link href="/" className="flex items-center gap-2 text-xs font-semibold tracking-wider text-slate-100">
            <Radio className="h-4 w-4 text-cyan-300" /> NODEHARVEST
          </Link>
          <Link href="/login" aria-label="管理登录" className="text-slate-500">
            <ShieldCheck className="h-4 w-4" />
          </Link>
        </div>
        <nav className="flex gap-1 overflow-x-auto pb-1">
          {groups.flatMap((group) =>
            group.items.map((item) => (
              <NavItem key={item.href} {...item} active={active(item.href)} compact />
            )),
          )}
        </nav>
      </div>
    </>
  );
}
