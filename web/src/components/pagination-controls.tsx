"use client";

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
    <section aria-label="分页">
      <p>第 {page} 页 · 当前 {count} 条{total !== undefined ? " / 共 " + total + " 条" : ""}</p>
      <Button size="sm" variant="outline" onClick={onPrevious} disabled={disabled || page === 1}>
        上一页
      </Button>
      <Button size="sm" variant="secondary" onClick={onNext} disabled={disabled || !hasNext}>
        下一页
      </Button>
    </section>
  );
}
