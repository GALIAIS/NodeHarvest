"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  Activity,
  Bot,
  Download,
  Gauge,
  LayoutDashboard,
  Network,
  Radio,
  Server,
} from "lucide-react";
import { cn } from "@/lib/utils";

const items = [
  { href: "/", label: "仪表盘", icon: LayoutDashboard },
  { href: "/nodes", label: "节点库", icon: Network },
  { href: "/jobs", label: "任务中心", icon: Activity },
  { href: "/sources", label: "订阅源", icon: Server },
  { href: "/ai", label: "AI 可达", icon: Bot },
  { href: "/export", label: "导出", icon: Download },
];

export function Sidebar() {
  const pathname = usePathname();

  return (
    <aside className="flex w-60 shrink-0 flex-col border-r border-slate-800/80 bg-slate-950/80">
      <div className="border-b border-slate-800/80 px-5 py-5">
        <div className="flex items-center gap-2">
          <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-cyan-500/15 ring-1 ring-cyan-400/30">
            <Radio className="h-4 w-4 text-cyan-300" />
          </div>
          <div>
            <div className="font-[family-name:var(--font-display)] text-sm font-semibold tracking-wide text-slate-50">
              Node Hunter
            </div>
            <div className="text-[11px] text-slate-500">高质量节点观测台</div>
          </div>
        </div>
      </div>

      <nav className="flex flex-1 flex-col gap-1 p-3">
        {items.map((item) => {
          const active =
            item.href === "/"
              ? pathname === "/"
              : pathname.startsWith(item.href);
          const Icon = item.icon;
          return (
            <Link
              key={item.href}
              href={item.href}
              className={cn(
                "flex items-center gap-2.5 rounded-lg px-3 py-2.5 text-sm transition-colors",
                active
                  ? "bg-cyan-500/10 text-cyan-200 ring-1 ring-cyan-500/20"
                  : "text-slate-400 hover:bg-slate-900 hover:text-slate-200"
              )}
            >
              <Icon className="h-4 w-4" />
              {item.label}
            </Link>
          );
        })}
      </nav>

      <div className="border-t border-slate-800/80 p-4">
        <div className="rounded-lg border border-slate-800 bg-slate-900/60 p-3">
          <div className="mb-1 flex items-center gap-1.5 text-[11px] uppercase tracking-wider text-slate-500">
            <Gauge className="h-3 w-3" />
            Quality First
          </div>
          <p className="text-xs leading-relaxed text-slate-400">
            多轮延迟 · 抖动 · TLS · AI 启发评分，只留高质量节点。
          </p>
        </div>
      </div>
    </aside>
  );
}
