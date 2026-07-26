"use client";

import * as React from "react";
import * as CheckboxPrimitive from "@radix-ui/react-checkbox";
import { Check } from "lucide-react";
import { cn } from "@/lib/utils";

export function Checkbox({
  className,
  ...props
}: React.ComponentProps<typeof CheckboxPrimitive.Root>) {
  return (
    <CheckboxPrimitive.Root
      data-slot="checkbox"
      className={cn(
        "peer size-4 shrink-0 border border-input bg-popover transition-colors outline-none",
        "hover:border-primary/60",
        "focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/40",
        "data-[state=checked]:border-primary data-[state=checked]:bg-primary data-[state=checked]:text-primary-foreground",
        "disabled:cursor-not-allowed disabled:opacity-50",
        className,
      )}
      {...props}
    >
      <CheckboxPrimitive.Indicator
        data-slot="checkbox-indicator"
        className="flex items-center justify-center text-current"
      >
        <Check className="size-3 stroke-3" />
      </CheckboxPrimitive.Indicator>
    </CheckboxPrimitive.Root>
  );
}

/**
 * Checkbox rendered as a bordered chip — used in dense filter bars where a bare
 * box would float without an anchor.
 */
export function CheckboxChip({
  checked,
  onCheckedChange,
  children,
  className,
  ...props
}: React.ComponentProps<typeof CheckboxPrimitive.Root> & { children: React.ReactNode }) {
  const id = React.useId();
  return (
    <label
      htmlFor={id}
      className={cn(
        "inline-flex h-10 cursor-pointer items-center gap-2 border border-border bg-card px-3",
        "text-xs text-muted-foreground transition-colors select-none",
        "hover:border-input hover:text-foreground",
        "has-[[data-state=checked]]:border-primary/45 has-[[data-state=checked]]:bg-primary/5 has-[[data-state=checked]]:text-primary",
        className,
      )}
    >
      <Checkbox id={id} checked={checked} onCheckedChange={onCheckedChange} {...props} />
      {children}
    </label>
  );
}
