import { Suspense } from "react";

export default function NodesLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return <Suspense fallback={<div className="p-8 text-muted-foreground">加载中…</div>}>{children}</Suspense>;
}
