"use client";

import * as React from "react";
import * as ProgressPrimitive from "@radix-ui/react-progress";
import { cn } from "@/lib/utils";

export function Progress({
  className,
  value,
  indicatorClassName,
  ...props
}: React.ComponentProps<typeof ProgressPrimitive.Root> & { indicatorClassName?: string }) {
  return (
    <ProgressPrimitive.Root
      data-slot="progress"
      className={cn("relative h-1.5 w-full overflow-hidden border border-border bg-muted", className)}
      value={value}
      {...props}
    >
      <ProgressPrimitive.Indicator
        data-slot="progress-indicator"
        className={cn("h-full w-full flex-1 bg-primary transition-transform", indicatorClassName)}
        style={{ transform: `translateX(-${100 - (value || 0)}%)` }}
      />
    </ProgressPrimitive.Root>
  );
}

/**
 * Segmented meter — reads as discrete blocks rather than a continuous bar,
 * which keeps the hard-edge language consistent in dense metric rows.
 */
export function Meter({
  value,
  segments = 20,
  className,
  tone = "primary",
  ...props
}: {
  value: number;
  segments?: number;
  className?: string;
  tone?: "primary" | "accent" | "success" | "destructive";
} & Omit<React.ComponentProps<"div">, "children">) {
  const filled = Math.round((Math.min(100, Math.max(0, value)) / 100) * segments);
  const toneClass = {
    primary: "bg-primary",
    accent: "bg-accent",
    success: "bg-success",
    destructive: "bg-destructive",
  }[tone];

  return (
    <div
      role="meter"
      aria-valuenow={Math.round(value)}
      aria-valuemin={0}
      aria-valuemax={100}
      className={cn("flex h-2 gap-px", className)}
      {...props}
    >
      {Array.from({ length: segments }, (_, index) => (
        <span
          key={index}
          className={cn("flex-1", index < filled ? toneClass : "bg-muted")}
        />
      ))}
    </div>
  );
}
