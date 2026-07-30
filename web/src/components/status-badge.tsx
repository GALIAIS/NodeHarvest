import type { ComponentProps } from "react";
import { Badge } from "@/components/ui/badge";

type BadgeVariant = ComponentProps<typeof Badge>["variant"];

const variants: Record<string, BadgeVariant> = {
  active: "default",
  alive: "default",
  completed: "default",
  enabled: "default",
  ok: "default",
  passed: "default",
  ready: "default",
  success: "default",
  critical: "destructive",
  dead: "destructive",
  error: "destructive",
  failed: "destructive",
  unhealthy: "destructive",
  cancelled: "secondary",
  disabled: "secondary",
  inactive: "secondary",
  pending: "secondary",
  queued: "secondary",
  running: "secondary",
  stopped: "secondary",
};

export function StatusBadge({
  status,
  children,
}: {
  status: string;
  children?: React.ReactNode;
}) {
  return (
    <Badge variant={variants[status.toLowerCase()] ?? "outline"}>
      {children ?? status}
    </Badge>
  );
}
