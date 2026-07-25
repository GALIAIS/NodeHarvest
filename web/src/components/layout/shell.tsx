import { Sidebar } from "./sidebar";

export function Shell({ children }: { children: React.ReactNode }) {
  return (
    <div className="relative flex min-h-screen bg-[#070b14] text-slate-100">
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(ellipse_at_top,_rgba(34,211,238,0.08),_transparent_50%),radial-gradient(ellipse_at_bottom_right,_rgba(251,191,36,0.06),_transparent_45%)]" />
      <div
        className="pointer-events-none absolute inset-0 opacity-[0.035]"
        style={{
          backgroundImage:
            "linear-gradient(rgba(148,163,184,0.5) 1px, transparent 1px), linear-gradient(90deg, rgba(148,163,184,0.5) 1px, transparent 1px)",
          backgroundSize: "48px 48px",
        }}
      />
      <Sidebar />
      <main className="relative z-10 flex min-w-0 flex-1 flex-col">
        {children}
      </main>
    </div>
  );
}
