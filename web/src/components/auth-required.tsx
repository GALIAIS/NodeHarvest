"use client";

import Link from "next/link";
import { LockKeyhole, ShieldCheck, ShieldX } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { cn } from "@/lib/utils";

/**
 * Shown where a section needs credentials the visitor does not have. This is a
 * normal state, not a failure — it deliberately avoids the destructive Alert
 * styling used for real backend errors.
 *
 * `reason` separates the two cases: an anonymous visitor gets a sign-in call to
 * action, while someone already signed in with too low a role must not be sent
 * back to the login page — that would just loop them.
 */
export function AuthRequired({
  reason = "anonymous",
  title,
  description,
  className,
  compact,
}: {
  reason?: "anonymous" | "forbidden";
  title?: string;
  description?: string;
  className?: string;
  compact?: boolean;
}) {
  const forbidden = reason === "forbidden";
  const Icon = forbidden ? ShieldX : LockKeyhole;

  const body = (
    <div
      className={cn(
        "flex flex-col items-center gap-3 px-6 text-center",
        compact ? "py-10" : "py-16",
        compact && className,
      )}
    >
      <span
        className={cn(
          "corner-ticks relative flex size-10 items-center justify-center border",
          forbidden ? "border-destructive/40 bg-destructive/10" : "border-accent/40 bg-accent/10",
        )}
      >
        <Icon className={cn("size-4", forbidden ? "text-destructive" : "text-accent")} />
      </span>
      <div className="space-y-1.5">
        <p className="font-display text-sm font-semibold text-foreground">
          {title ?? (forbidden ? "当前角色无权查看" : "需要登录")}
        </p>
        <p className="mx-auto max-w-md text-xs leading-relaxed text-muted-foreground">
          {description ??
            (forbidden
              ? "该操作需要更高权限，请联系管理员调整角色。"
              : "该视图属于管理面，登录后即可查看。公开的节点、采集源与订阅数据无需登录。")}
        </p>
      </div>
      {!forbidden && (
        <Button variant="outline" size="sm" asChild className="mt-1">
          <Link href="/login">
            <ShieldCheck className="size-3.5" /> 前往登录
          </Link>
        </Button>
      )}
    </div>
  );

  if (compact) return body;
  return <Card className={cn("border-dashed", className)}>{body}</Card>;
}
