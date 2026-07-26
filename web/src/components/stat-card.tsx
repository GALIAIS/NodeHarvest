import { Card, CardContent } from "@/components/ui/card";
import { cn } from "@/lib/utils";

type Accent = "primary" | "accent" | "success" | "destructive";

const rail: Record<Accent, string> = {
  primary: "bg-primary",
  accent: "bg-accent",
  success: "bg-success",
  destructive: "bg-destructive",
};

const value: Record<Accent, string> = {
  primary: "text-primary",
  accent: "text-accent",
  success: "text-success",
  destructive: "text-destructive",
};

export function StatCard({
  label,
  value: metric,
  hint,
  accent = "primary",
  icon: Icon,
  className,
}: {
  label: string;
  value: string | number;
  hint?: string;
  accent?: Accent;
  icon?: React.ComponentType<{ className?: string }>;
  className?: string;
}) {
  return (
    <Card className={cn("relative overflow-hidden", className)}>
      {/* the metric's identity lives in a solid top rail rather than a gradient wash */}
      <span className={cn("absolute inset-x-0 top-0 h-0.5", rail[accent])} />
      <CardContent className="px-4 pt-4 pb-4">
        <div className="flex items-start justify-between gap-3">
          <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-muted-foreground">
            {label}
          </p>
          {Icon && <Icon className={cn("size-4 shrink-0 opacity-70", value[accent])} />}
        </div>
        <p
          className={cn(
            "mt-2 font-display text-3xl leading-none font-semibold tabular-nums",
            value[accent],
          )}
        >
          {metric}
        </p>
        {hint && <p className="mt-2 truncate text-xs text-muted-foreground">{hint}</p>}
      </CardContent>
    </Card>
  );
}
