import { Suspense } from "react";

export default function NodesLayout({ children }: { children: React.ReactNode }) {
  return <Suspense fallback={<p>加载中…</p>}>{children}</Suspense>;
}
