"use client";

import { useEffect, useState } from "react";
import { ExternalLink } from "lucide-react";
import { api, type Source } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

export default function SourcesPage() {
  const [sources, setSources] = useState<Source[]>([]);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    api
      .sources()
      .then(setSources)
      .catch((e) => setErr(e instanceof Error ? e.message : "加载失败"));
  }, []);

  const enabled = sources.filter((s) => s.enabled).length;

  return (
    <div className="flex flex-1 flex-col">
      <header className="border-b border-slate-800/80 px-8 py-5">
        <h1 className="font-[family-name:var(--font-display)] text-xl font-semibold">
          订阅源
        </h1>
        <p className="mt-1 text-sm text-slate-500">
          配置来自 <code className="text-cyan-300">configs/config.yaml</code> · 启用{" "}
          {enabled}/{sources.length}
        </p>
      </header>

      <div className="space-y-4 p-8">
        {err && (
          <div className="rounded-lg border border-rose-500/30 bg-rose-500/10 px-4 py-3 text-sm text-rose-300">
            {err}
          </div>
        )}

        <div className="grid gap-3">
          {sources.map((s) => (
            <Card key={s.name + s.url}>
              <CardHeader className="flex-row items-start justify-between space-y-0 pb-2">
                <div>
                  <CardTitle className="flex items-center gap-2">
                    {s.name}
                    <Badge variant={s.enabled ? "success" : "secondary"}>
                      {s.enabled ? "enabled" : "disabled"}
                    </Badge>
                    <Badge variant="default">{s.type || "subscription"}</Badge>
                  </CardTitle>
                  <CardDescription className="mt-2 break-all font-mono text-[11px]">
                    {s.url}
                  </CardDescription>
                </div>
                <a
                  href={s.url}
                  target="_blank"
                  rel="noreferrer"
                  className="text-slate-500 hover:text-cyan-300"
                >
                  <ExternalLink className="h-4 w-4" />
                </a>
              </CardHeader>
              <CardContent className="text-xs text-slate-500">
                在配置文件中修改 enabled / 增删 URL 后重启后端生效
              </CardContent>
            </Card>
          ))}
        </div>
      </div>
    </div>
  );
}
