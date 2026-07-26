import type { DailyMetric } from "@/lib/api";

function points(values: number[], width: number, height: number, top = 12, bottom = 24) {
  if (values.length === 0) return "";
  const min = Math.min(...values);
  const max = Math.max(...values);
  const span = max - min || 1;
  const usable = height - top - bottom;
  return values
    .map((value, index) => {
      const x = values.length === 1 ? width / 2 : (index / (values.length - 1)) * width;
      const y = top + (1 - (value - min) / span) * usable;
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join(" ");
}

export function TrendChart({ data }: { data: DailyMetric[] }) {
  const width = 720;
  const height = 210;
  const scores = data.map((row) => row.avg_score);
  const latencies = data.map((row) => row.p95_latency_ms);
  const scorePoints = points(scores, width, height);
  const latencyPoints = points(latencies, width, height);

  if (data.length === 0) {
    return (
      <div className="flex h-52 items-center justify-center rounded-lg border border-dashed border-slate-800 text-sm text-slate-600">
        完成一次质量任务后生成趋势
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap gap-4 font-mono text-[10px] uppercase tracking-widest">
        <span className="flex items-center gap-2 text-cyan-300">
          <i className="h-0.5 w-5 bg-cyan-400" /> 平均评分
        </span>
        <span className="flex items-center gap-2 text-amber-300">
          <i className="h-0.5 w-5 bg-amber-400" /> P95 延迟
        </span>
      </div>
      <svg
        viewBox={`0 0 ${width} ${height}`}
        role="img"
        aria-label="近三十日平均评分与 P95 延迟趋势"
        className="h-52 w-full overflow-visible"
      >
        <defs>
          <linearGradient id="score-area" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="#22d3ee" stopOpacity=".26" />
            <stop offset="100%" stopColor="#22d3ee" stopOpacity="0" />
          </linearGradient>
        </defs>
        {[42, 84, 126, 168].map((y) => (
          <line key={y} x1="0" x2={width} y1={y} y2={y} stroke="#1e293b" strokeDasharray="4 8" />
        ))}
        <polyline
          points={`0,186 ${scorePoints} ${width},186`}
          fill="url(#score-area)"
          stroke="none"
        />
        <polyline
          points={scorePoints}
          fill="none"
          stroke="#22d3ee"
          strokeWidth="3"
          strokeLinejoin="round"
          strokeLinecap="round"
        />
        <polyline
          points={latencyPoints}
          fill="none"
          stroke="#fbbf24"
          strokeWidth="2"
          strokeDasharray="7 6"
          strokeLinejoin="round"
          strokeLinecap="round"
        />
        {data.map((row, index) => {
          if (index !== 0 && index !== data.length - 1 && index % Math.ceil(data.length / 5) !== 0) {
            return null;
          }
          const x = data.length === 1 ? width / 2 : (index / (data.length - 1)) * width;
          return (
            <text
              key={`${row.day}-${index}`}
              x={x}
              y="205"
              textAnchor={index === 0 ? "start" : index === data.length - 1 ? "end" : "middle"}
              fill="#64748b"
              fontSize="10"
              fontFamily="monospace"
            >
              {row.day.slice(5)}
            </text>
          );
        })}
      </svg>
    </div>
  );
}
