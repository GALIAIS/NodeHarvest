"use client";

import { usePathname } from "next/navigation";
import { SessionProvider } from "@/components/session-provider";
import { AccessGate } from "@/components/access-gate";
import { LiveProvider } from "@/components/live-provider";
import { AppHeader, AppSidebar } from "./sidebar";
import {
  SidebarInset,
  SidebarProvider,
} from "@/components/ui/sidebar";

export function Shell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();

  return (
    <SessionProvider>
      <LiveProvider>
        {pathname === "/login" ? (
          <main className="min-h-svh bg-muted/40 p-4 md:p-6">
            <div className="mx-auto w-full max-w-md space-y-6">
              <AccessGate>{children}</AccessGate>
            </div>
          </main>
        ) : (
          <SidebarProvider>
            <AppSidebar />
            <SidebarInset>
              <AppHeader />
              <div className="mx-auto w-full max-w-screen-2xl flex-1 space-y-6 p-4 md:p-6">
                <AccessGate>{children}</AccessGate>
              </div>
            </SidebarInset>
          </SidebarProvider>
        )}
      </LiveProvider>
    </SessionProvider>
  );
}
