"use client";

import type { LucideIcon } from "lucide-react";
import {
  Activity,
  BellRing,
  Bot,
  Boxes,
  FileClock,
  Gauge,
  KeyRound,
  ListTodo,
  LogIn,
  LogOut,
  RadioTower,
  Scale,
  Server,
  Settings2,
  ShieldCheck,
  UsersRound,
  Workflow,
} from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { api } from "@/lib/api";
import { useSession } from "@/components/session-provider";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
  SidebarSeparator,
  SidebarTrigger,
  useSidebar,
} from "@/components/ui/sidebar";
import { Separator } from "@/components/ui/separator";

type Route = {
  href: string;
  label: string;
  description: string;
  icon: LucideIcon;
};

const overviewRoutes: Route[] = [
  { href: "/", label: "仪表盘", description: "平台运行概览", icon: Gauge },
];

const monitorRoutes: Route[] = [
  { href: "/nodes", label: "节点库", description: "节点质量与可用性", icon: Server },
  { href: "/sources", label: "采集源", description: "来源健康与贡献", icon: RadioTower },
  { href: "/jobs", label: "任务中心", description: "任务、队列与事件", icon: ListTodo },
  { href: "/ai", label: "AI 可达", description: "AI 目标探测结果", icon: Bot },
  { href: "/alerts", label: "异常告警", description: "活动异常与处理", icon: BellRing },
];

const deliveryRoutes: Route[] = [
  { href: "/export", label: "订阅池", description: "订阅与节点导出", icon: Boxes },
  { href: "/sub-store", label: "订阅工坊", description: "Sub-Store 集成", icon: Workflow },
  { href: "/tokens", label: "Token", description: "订阅凭证管理", icon: KeyRound },
];

const adminRoutes: Route[] = [
  { href: "/users", label: "用户", description: "账户与角色", icon: UsersRound },
  { href: "/audit", label: "审计", description: "操作记录与导出", icon: FileClock },
  { href: "/system", label: "系统", description: "服务状态与配置", icon: Settings2 },
  { href: "/terms", label: "合规", description: "使用条款与限制", icon: Scale },
];

const allRoutes = [
  ...overviewRoutes,
  ...monitorRoutes,
  ...deliveryRoutes,
  ...adminRoutes,
];

function isRouteActive(pathname: string, href: string) {
  return href === "/" ? pathname === "/" : pathname.startsWith(href);
}

function currentRoute(pathname: string) {
  return (
    allRoutes.find((route) => isRouteActive(pathname, route.href)) ?? {
      label: "NodeHarvest",
      description: "节点采集与订阅分发",
    }
  );
}

function NavigationGroup({ label, routes }: { label: string; routes: Route[] }) {
  const pathname = usePathname();
  const { setOpenMobile } = useSidebar();

  return (
    <SidebarGroup>
      <SidebarGroupLabel>{label}</SidebarGroupLabel>
      <SidebarGroupContent>
        <SidebarMenu>
          {routes.map((route) => {
            const active = isRouteActive(pathname, route.href);
            const Icon = route.icon;
            return (
              <SidebarMenuItem key={route.href}>
                <SidebarMenuButton
                  asChild
                  isActive={active}
                  tooltip={route.label}
                >
                  <Link
                    href={route.href}
                    aria-current={active ? "page" : undefined}
                    onClick={() => setOpenMobile(false)}
                  >
                    <Icon />
                    <span>{route.label}</span>
                  </Link>
                </SidebarMenuButton>
              </SidebarMenuItem>
            );
          })}
        </SidebarMenu>
      </SidebarGroupContent>
    </SidebarGroup>
  );
}

export function AppSidebar() {
  const { authenticated, session } = useSession();
  const { setOpenMobile } = useSidebar();

  return (
    <Sidebar collapsible="icon">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton asChild size="lg" tooltip="NodeHarvest">
              <Link href="/" onClick={() => setOpenMobile(false)}>
                <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground">
                  <Activity className="size-4" />
                </div>
                <div className="grid flex-1 text-left text-sm leading-tight">
                  <span className="truncate font-semibold">NodeHarvest</span>
                  <span className="truncate text-xs">节点采集与订阅分发</span>
                </div>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarSeparator />
      <SidebarContent>
        <NavigationGroup label="概览" routes={overviewRoutes} />
        {authenticated && (
          <>
            <NavigationGroup label="监控" routes={monitorRoutes} />
            <NavigationGroup label="分发" routes={deliveryRoutes} />
            <NavigationGroup label="管理" routes={adminRoutes} />
          </>
        )}
      </SidebarContent>
      <SidebarSeparator />
      <SidebarFooter>
        <SidebarMenu>
          {authenticated && session ? (
            <>
              <SidebarMenuItem>
                <SidebarMenuButton size="lg" tooltip={session.principal.name}>
                  <ShieldCheck />
                  <div className="grid flex-1 text-left text-sm leading-tight">
                    <span className="truncate font-medium">{session.principal.name}</span>
                    <span className="truncate text-xs">
                      {session.principal.tenant_id + " · " + session.principal.role}
                    </span>
                  </div>
                </SidebarMenuButton>
              </SidebarMenuItem>
              <SidebarMenuItem>
                <SidebarMenuButton
                  tooltip="退出登录"
                  onClick={() => {
                    void api.logout().finally(() => window.location.assign("/login"));
                  }}
                >
                  <LogOut />
                  <span>退出登录</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </>
          ) : (
            <SidebarMenuItem>
              <SidebarMenuButton asChild tooltip="管理登录">
                <Link href="/login" onClick={() => setOpenMobile(false)}>
                  <LogIn />
                  <span>管理登录</span>
                </Link>
              </SidebarMenuButton>
            </SidebarMenuItem>
          )}
        </SidebarMenu>
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  );
}

export function AppHeader() {
  const pathname = usePathname();
  const route = currentRoute(pathname);

  return (
    <header className="sticky top-0 z-10 flex h-14 shrink-0 items-center gap-2 border-b bg-background px-4">
      <SidebarTrigger className="-ml-1" />
      <Separator
        orientation="vertical"
        className="mr-2 data-[orientation=vertical]:h-4"
      />
      <div className="min-w-0">
        <p className="truncate text-sm font-medium">{route.label}</p>
        <p className="hidden truncate text-xs text-muted-foreground sm:block">
          {route.description}
        </p>
      </div>
    </header>
  );
}
