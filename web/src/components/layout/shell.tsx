import { SessionProvider } from "@/components/session-provider";
import { TooltipProvider } from "@/components/ui/tooltip";
import { Sidebar } from "./sidebar";

export function Shell({ children }: { children: React.ReactNode }) {
  return (
    <SessionProvider>
      <TooltipProvider>
        <div className="telemetry-grid relative flex min-h-screen flex-col bg-background text-foreground md:flex-row">
          {/* Two faint corner washes keep the flat background from reading as dead space. */}
          <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(ellipse_at_top_left,_color-mix(in_oklab,var(--primary)_9%,transparent),_transparent_55%),radial-gradient(ellipse_at_bottom_right,_color-mix(in_oklab,var(--accent)_7%,transparent),_transparent_50%)]" />
          <Sidebar />
          <main className="relative z-10 flex min-w-0 flex-1 flex-col">{children}</main>
        </div>
      </TooltipProvider>
    </SessionProvider>
  );
}
