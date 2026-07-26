import * as React from "react";
import { cn } from "@/lib/utils";

export function Card({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="card"
      className={cn("border border-border bg-card text-card-foreground", className)}
      {...props}
    />
  );
}

export function CardHeader({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="card-header"
      className={cn("flex flex-col gap-1.5 px-5 pt-4 pb-3", className)}
      {...props}
    />
  );
}

export function CardTitle({ className, ...props }: React.ComponentProps<"h3">) {
  return (
    <h3
      data-slot="card-title"
      className={cn("text-sm font-semibold tracking-wide text-foreground", className)}
      {...props}
    />
  );
}

export function CardDescription({ className, ...props }: React.ComponentProps<"p">) {
  return (
    <p
      data-slot="card-description"
      className={cn("text-xs leading-relaxed text-muted-foreground", className)}
      {...props}
    />
  );
}

export function CardAction({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="card-action"
      className={cn("flex shrink-0 items-center gap-2", className)}
      {...props}
    />
  );
}

export function CardContent({ className, ...props }: React.ComponentProps<"div">) {
  return <div data-slot="card-content" className={cn("px-5 pb-5", className)} {...props} />;
}

export function CardFooter({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="card-footer"
      className={cn("flex items-center gap-2 border-t border-border px-5 py-3", className)}
      {...props}
    />
  );
}

/**
 * Empty state for card/list bodies — mirrors TableEmpty's proportions so empty
 * surfaces read identically whether they are tables or card stacks.
 */
export function CardEmpty({
  icon: Icon,
  className,
  children,
}: {
  icon?: React.ComponentType<{ className?: string }>;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <div className={cn("px-4 py-16 text-center text-sm text-muted-foreground", className)}>
      {Icon && <Icon className="mx-auto mb-3 size-5 opacity-60" />}
      {children}
    </div>
  );
}

/**
 * Section label used above dense groups: mono caption plus a hairline rule that
 * runs to the edge of the container.
 */
export function SectionRule({
  label,
  className,
  children,
}: {
  label: string;
  className?: string;
  children?: React.ReactNode;
}) {
  return (
    <div className={cn("flex items-center gap-3", className)}>
      <span className="font-mono text-[10px] uppercase tracking-[0.22em] text-muted-foreground">
        {label}
      </span>
      <span className="h-px flex-1 bg-border" />
      {children}
    </div>
  );
}
