"use client";

import * as React from "react";
import * as SwitchPrimitive from "@radix-ui/react-switch";
import { cn } from "@/lib/utils";

export function Switch({
  className,
  ...props
}: React.ComponentProps<typeof SwitchPrimitive.Root>) {
  return (
    <SwitchPrimitive.Root
      data-slot="switch"
      className={cn(
        "peer inline-flex h-5 w-9 shrink-0 items-center border border-input bg-muted p-px transition-colors outline-none",
        "focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/40",
        "data-[state=checked]:border-primary data-[state=checked]:bg-primary/25",
        "disabled:cursor-not-allowed disabled:opacity-50",
        className,
      )}
      {...props}
    >
      <SwitchPrimitive.Thumb
        data-slot="switch-thumb"
        className={cn(
          "pointer-events-none block size-4 bg-muted-foreground transition-transform",
          "data-[state=unchecked]:translate-x-0 data-[state=checked]:translate-x-4",
          "data-[state=checked]:bg-primary",
        )}
      />
    </SwitchPrimitive.Root>
  );
}
