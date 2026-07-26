import * as React from "react";
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const badgeVariants = cva(
  [
    "inline-flex w-fit shrink-0 items-center gap-1 border px-2 py-0.5",
    "font-mono text-[10px] uppercase leading-4 tracking-[0.1em]",
    "[&_svg]:pointer-events-none [&_svg:not([class*='size-'])]:size-3",
  ],
  {
    variants: {
      variant: {
        default: "border-primary/40 bg-primary/10 text-primary",
        secondary: "border-border bg-muted text-muted-foreground",
        success: "border-success/40 bg-success/10 text-success",
        warn: "border-accent/40 bg-accent/10 text-accent",
        danger: "border-destructive/40 bg-destructive/10 text-destructive",
        outline: "border-border bg-transparent text-foreground",
      },
    },
    defaultVariants: { variant: "default" },
  },
);

export function Badge({
  className,
  variant,
  asChild = false,
  ...props
}: React.ComponentProps<"span"> & VariantProps<typeof badgeVariants> & { asChild?: boolean }) {
  const Comp = asChild ? Slot : "span";
  return (
    <Comp data-slot="badge" className={cn(badgeVariants({ variant }), className)} {...props} />
  );
}

export { badgeVariants };
