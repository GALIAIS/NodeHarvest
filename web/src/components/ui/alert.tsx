import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const alertVariants = cva(
  [
    "relative grid w-full grid-cols-[0_1fr] items-start gap-y-1 border px-4 py-3 text-sm",
    "has-[>svg]:grid-cols-[calc(var(--spacing)*4)_1fr] has-[>svg]:gap-x-3",
    "[&>svg]:size-4 [&>svg]:translate-y-0.5",
    // left rail carries the severity instead of a tinted rounded pill
    "border-l-2",
  ],
  {
    variants: {
      variant: {
        default: "border-border border-l-primary bg-card text-foreground [&>svg]:text-primary",
        info: "border-primary/25 border-l-primary bg-primary/5 text-primary [&>svg]:text-primary",
        warn: "border-accent/25 border-l-accent bg-accent/5 text-accent [&>svg]:text-accent",
        danger:
          "border-destructive/30 border-l-destructive bg-destructive/5 text-destructive [&>svg]:text-destructive",
        success:
          "border-success/25 border-l-success bg-success/5 text-success [&>svg]:text-success",
      },
    },
    defaultVariants: { variant: "default" },
  },
);

export function Alert({
  className,
  variant,
  ...props
}: React.ComponentProps<"div"> & VariantProps<typeof alertVariants>) {
  return (
    <div
      data-slot="alert"
      role="alert"
      className={cn(alertVariants({ variant }), className)}
      {...props}
    />
  );
}

export function AlertTitle({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="alert-title"
      className={cn("col-start-2 min-h-4 font-medium tracking-tight", className)}
      {...props}
    />
  );
}

export function AlertDescription({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="alert-description"
      className={cn(
        "col-start-2 grid justify-items-start gap-1 text-xs leading-relaxed opacity-85",
        className,
      )}
      {...props}
    />
  );
}

export { alertVariants };
