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
    <section
      className="flex flex-col gap-3 rounded-lg border bg-card p-4 sm:flex-row sm:items-center"
      aria-label="分页"
    >
      <p className="mr-auto text-sm text-muted-foreground tabular-nums">
        第 {page} 页 · 当前 {count} 条{total !== undefined ? " / 共 " + total + " 条" : ""}
      </p>
      <div className="flex gap-2">
        <Button size="sm" variant="outline" onClick={onPrevious} disabled={disabled || page === 1}>
          <ChevronLeft />
          上一页
        </Button>
        <Button size="sm" variant="outline" onClick={onNext} disabled={disabled || !hasNext}>
          下一页
          <ChevronRight />
        </Button>
      </div>
    </section>
  );
}
