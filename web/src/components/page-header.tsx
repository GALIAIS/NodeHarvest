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
        "flex flex-col gap-4 border-b border-slate-800/80 px-4 py-5 sm:px-6 lg:flex-row lg:items-end lg:justify-between lg:px-8",
        className,
      )}
    >
      <div>
        {eyebrow && (
          <p className="mb-1 font-mono text-[10px] uppercase tracking-[0.24em] text-cyan-400/80">
            {eyebrow}
          </p>
        )}
        <h1 className="font-[family-name:var(--font-display)] text-xl font-semibold tracking-tight text-slate-50 sm:text-2xl">
          {title}
        </h1>
        <p className="mt-1 max-w-3xl text-sm leading-relaxed text-slate-500">{description}</p>
      </div>
      {actions && <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}
    </header>
  );
}
