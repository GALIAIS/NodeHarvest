import * as React from "react";
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-400/60 disabled:pointer-events-none disabled:opacity-50",
  {
    variants: {
      variant: {
        default:
          "bg-cyan-500 text-slate-950 hover:bg-cyan-400 shadow-[0_0_20px_-6px_rgba(34,211,238,0.7)]",
        secondary:
          "bg-slate-800 text-slate-100 border border-slate-700 hover:bg-slate-700",
        outline:
          "border border-cyan-500/40 bg-transparent text-cyan-200 hover:bg-cyan-500/10",
        ghost: "hover:bg-slate-800 text-slate-200",
        danger:
          "bg-rose-600 text-white hover:bg-rose-500",
        amber:
          "bg-amber-400 text-slate-950 hover:bg-amber-300 shadow-[0_0_20px_-6px_rgba(251,191,36,0.7)]",
      },
      size: {
        default: "h-10 px-4 py-2",
        sm: "h-8 rounded-md px-3 text-xs",
        lg: "h-11 rounded-md px-6",
        icon: "h-10 w-10",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
);

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, ...props }, ref) => {
    const Comp = asChild ? Slot : "button";
    return (
      <Comp
        className={cn(buttonVariants({ variant, size, className }))}
        ref={ref}
        {...props}
      />
    );
  }
);
Button.displayName = "Button";
