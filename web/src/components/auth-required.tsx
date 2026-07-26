"use client";

import Link from "next/link";
import { LockKeyhole, ShieldCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { cn } from "@/lib/utils";

/**
 * Shown where a section needs credentials the visitor does not have. This is a
 * normal state, not a failure — it deliberately avoids the destructive Alert
 * styling used for real backend errors.
 */
export function AuthRequired({
  title = "需要登录",
  description,
  className,
  compact,
}: {
  title?: string;
  description?: string;
  className?: string;
  compact?: boolean;
}) {
  const body = (
    <div
      className={cn(
        "flex flex-col items-center gap-3 px-6 text-center",
        compact ? "py-10" : "py-16",
      )}
    >
      <span className="corner-ticks relative flex size-10 items-center justify-center border border-accent/40 bg-accent/10">
        <LockKeyhole className="size-4 text-accent" />
      </span>
      <div className="space-y-1.5">
        <p className="font-display text-sm font-semibold text-foreground">{title}</p>
        <p className="mx-auto max-w-md text-xs leading-relaxed text-muted-foreground">
          {description ?? "该视图属于管理面，登录后即可查看。公开的节点、采集源与订阅数据无需登录。"}
        </p>
      </div>
      <Button variant="outline" size="sm" asChild className="mt-1">
        <Link href="/login">
          <ShieldCheck className="size-3.5" /> 前往登录
        </Link>
      </Button>
    </div>
  );

  if (compact) return body;
  return <Card className={cn("border-dashed", className)}>{body}</Card>;
}
