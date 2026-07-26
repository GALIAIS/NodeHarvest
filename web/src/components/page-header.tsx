import { cn } from "@/lib/utils";

export function PageHeader({
  eyebrow,
  title,
  description,
  actions,
  className,
}: {
  eyebrow?: string;
  title: string;
  description: string;
  actions?: React.ReactNode;
  className?: string;
}) {
  return (
    <header
      className={cn(
        "relative flex flex-col gap-4 border-b border-border bg-card/40 px-4 py-5 sm:px-6 lg:flex-row lg:items-end lg:justify-between lg:px-8",
        className,
      )}
    >
      {/* accent rail anchors the page title to the left edge of the content column */}
      <span className="pointer-events-none absolute inset-y-5 left-0 w-0.5 bg-primary lg:left-0" />
      <div className="min-w-0">
        {eyebrow && (
          <p className="mb-1.5 font-mono text-[10px] uppercase tracking-[0.26em] text-primary/80">
            {eyebrow}
          </p>
        )}
        <h1 className="font-display text-xl font-semibold tracking-tight text-foreground sm:text-2xl">
          {title}
        </h1>
        <p className="mt-1.5 max-w-3xl text-sm leading-relaxed text-muted-foreground">
          {description}
        </p>
      </div>
      {actions && <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}
    </header>
  );
}
