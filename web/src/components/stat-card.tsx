import { Card, CardContent } from "@/components/ui/card";
import { cn } from "@/lib/utils";

export function StatCard({
  label,
  value,
  hint,
  accent = "cyan",
}: {
  label: string;
  value: string | number;
  hint?: string;
  accent?: "cyan" | "amber" | "emerald" | "rose";
}) {
  const ring = {
    cyan: "from-cyan-500/20 to-transparent",
    amber: "from-amber-400/20 to-transparent",
    emerald: "from-emerald-400/20 to-transparent",
    rose: "from-rose-400/20 to-transparent",
  }[accent];

  const valueColor = {
    cyan: "text-cyan-200",
    amber: "text-amber-200",
    emerald: "text-emerald-200",
    rose: "text-rose-200",
  }[accent];

  return (
    <Card className="relative overflow-hidden">
      <div className={cn("absolute inset-0 bg-gradient-to-br opacity-80", ring)} />
      <CardContent className="relative p-5">
        <div className="text-[11px] uppercase tracking-[0.14em] text-slate-500">
          {label}
        </div>
        <div className={cn("mt-2 font-[family-name:var(--font-display)] text-3xl font-semibold tabular-nums", valueColor)}>
          {value}
        </div>
        {hint && <div className="mt-1 text-xs text-slate-500">{hint}</div>}
      </CardContent>
    </Card>
  );
}
