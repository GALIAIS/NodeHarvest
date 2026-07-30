import { SessionProvider } from "@/components/session-provider";
import { AccessGate } from "@/components/access-gate";
import { LiveProvider } from "@/components/live-provider";
import { Sidebar } from "./sidebar";

export function Shell({ children }: { children: React.ReactNode }) {
  return (
    <SessionProvider>
      <LiveProvider>
        <Sidebar />
        <main>
          <AccessGate>{children}</AccessGate>
        </main>
      </LiveProvider>
    </SessionProvider>
  );
}
