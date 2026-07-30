import { SessionProvider } from "@/components/session-provider";
import { AccessGate } from "@/components/access-gate";
import { LiveProvider } from "@/components/live-provider";
import { Sidebar } from "./sidebar";

export function Shell({ children }: { children: React.ReactNode }) {
  return (
    <SessionProvider>
      <LiveProvider>
        <div className="min-h-screen bg-background">
          <Sidebar />
          <main className="container mx-auto max-w-7xl space-y-6 px-4 py-6 md:px-6">
            <AccessGate>{children}</AccessGate>
          </main>
        </div>
      </LiveProvider>
    </SessionProvider>
  );
}
