"use client";

import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

/**
 * Tooltip wrapper for controls that may be disabled.
 *
 * A `title` attribute on a disabled button never appears: buttons carry
 * `disabled:pointer-events-none`, so no hover or focus event ever reaches them.
 * Wrapping in a focusable span keeps the explanation reachable — including by
 * keyboard, which a title attribute never was.
 */
export function Hint({
  content,
  side = "top",
  disabled,
  className,
  children,
}: {
  content: React.ReactNode;
  side?: "top" | "right" | "bottom" | "left";
  /** When false the children render bare, so enabled controls keep normal focus order. */
  disabled?: boolean;
  className?: string;
  children: React.ReactNode;
}) {
  if (!disabled) return <>{children}</>;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span tabIndex={0} className={cn("inline-flex", className)}>
          {children}
        </span>
      </TooltipTrigger>
      <TooltipContent side={side}>{content}</TooltipContent>
    </Tooltip>
  );
}
