import * as React from "react";
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const buttonVariants = cva(
  [
    "inline-flex shrink-0 items-center justify-center gap-2 whitespace-nowrap border font-medium",
    "transition-[background-color,border-color,color,box-shadow,transform] duration-150",
    "outline-none focus-visible:ring-2 focus-visible:ring-ring/60",
    "disabled:pointer-events-none disabled:opacity-45",
    "[&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
    // press into the offset shadow instead of animating a soft glow
    "active:translate-x-px active:translate-y-px active:shadow-none",
  ],
  {
    variants: {
      variant: {
        default:
          "border-primary bg-primary text-primary-foreground shadow-hard-sm hover:bg-primary/85 hover:border-primary/85",
        accent:
          "border-accent bg-accent text-accent-foreground shadow-hard-sm hover:bg-accent/85 hover:border-accent/85",
        secondary:
          "border-input bg-secondary text-secondary-foreground hover:border-primary/40 hover:bg-secondary/70",
        outline:
          "border-primary/45 bg-transparent text-primary hover:border-primary hover:bg-primary/10",
        ghost:
          "border-transparent bg-transparent text-muted-foreground hover:border-border hover:bg-muted hover:text-foreground",
        destructive:
          "border-destructive bg-destructive text-destructive-foreground shadow-hard-sm hover:bg-destructive/85 hover:border-destructive/85",
      },
      size: {
        default: "h-10 px-4 text-sm",
        sm: "h-8 px-3 text-xs",
        lg: "h-11 px-6 text-sm",
        icon: "size-10",
        "icon-sm": "size-8",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  },
);

export interface ButtonProps
  extends React.ComponentProps<"button">,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
}

export function Button({ className, variant, size, asChild = false, ...props }: ButtonProps) {
  const Comp = asChild ? Slot : "button";
  return (
    <Comp
      data-slot="button"
      className={cn(buttonVariants({ variant, size }), className)}
      {...props}
    />
  );
}

export { buttonVariants };
