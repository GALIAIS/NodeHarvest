"use client";

import { ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";

export function PaginationControls({
  page,
  total,
  count,
  hasNext,
  onPrevious,
  onNext,
  disabled = false,
}: {
  page: number;
  total?: number;
  count: number;
  hasNext: boolean;
  onPrevious: () => void;
  onNext: () => void;
  disabled?: boolean;
}) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 border border-border bg-card px-3 py-2">
      <p className="font-mono text-[10px] tabular-nums text-muted-foreground">
        第 {page} 页 · 当前 {count} 条{total !== undefined ? ` / 共 ${total} 条` : ""}
      </p>
      <div className="flex gap-2">
        <Button size="sm" variant="outline" onClick={onPrevious} disabled={disabled || page === 1}>
          <ChevronLeft className="size-3.5" /> 上一页
        </Button>
        <Button size="sm" variant="secondary" onClick={onNext} disabled={disabled || !hasNext}>
          下一页 <ChevronRight className="size-3.5" />
        </Button>
      </div>
    </div>
  );
}
