import type { DailyMetric } from "@/lib/api";

const WIDTH = 720;
const HEIGHT = 210;
const TOP = 12;
const BOTTOM = 26;

type Scale = (index: number, value: number) => [number, number];

function makeScale(values: number[]): Scale {
  const min = Math.min(...values);
  const max = Math.max(...values);
  const span = max - min || 1;
  const usable = HEIGHT - TOP - BOTTOM;
  return (index, value) => [
    values.length === 1 ? WIDTH / 2 : (index / (values.length - 1)) * WIDTH,
    TOP + (1 - (value - min) / span) * usable,
  ];
}

/**
 * Step path rather than a smoothed polyline: each sample holds its value until
 * the next one, which keeps the chart consistent with the squared-off UI.
 */
function stepPath(values: number[], scale: Scale) {
  return values
    .map((value, index) => {
      const [x, y] = scale(index, value);
      if (index === 0) return `M ${x.toFixed(1)} ${y.toFixed(1)}`;
      const [prevX] = scale(index - 1, values[index - 1]);
      const midX = (prevX + x) / 2;
      return `L ${midX.toFixed(1)} ${y.toFixed(1)} L ${x.toFixed(1)} ${y.toFixed(1)}`;
    })
    .join(" ");
}

export function TrendChart({ data }: { data: DailyMetric[] }) {
  if (data.length === 0) {
    return (
      <div className="flex h-52 items-center justify-center border border-dashed border-border text-sm text-muted-foreground">
        完成一次质量任务后生成趋势
      </div>
    );
  }

  const scores = data.map((row) => row.avg_score);
  const latencies = data.map((row) => row.p95_latency_ms);
  const scoreScale = makeScale(scores);
  const latencyScale = makeScale(latencies);
  const scorePath = stepPath(scores, scoreScale);
  const areaPath = `${scorePath} L ${WIDTH} ${HEIGHT - BOTTOM} L 0 ${HEIGHT - BOTTOM} Z`;
  const labelEvery = Math.ceil(data.length / 5);

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap gap-5 font-mono text-[10px] uppercase tracking-[0.16em]">
        <span className="flex items-center gap-2 text-primary">
          <span className="h-0.5 w-5 bg-primary" /> 平均评分
        </span>
        <span className="flex items-center gap-2 text-accent">
          <span className="h-0.5 w-5 border-t-2 border-dashed border-accent" /> P95 延迟
        </span>
      </div>
      <svg
        viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
        role="img"
        aria-label="近三十日平均评分与 P95 延迟趋势"
        className="h-52 w-full overflow-visible"
        preserveAspectRatio="none"
      >
        <defs>
          <linearGradient id="trend-score-area" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="var(--primary)" stopOpacity=".22" />
            <stop offset="100%" stopColor="var(--primary)" stopOpacity="0" />
          </linearGradient>
        </defs>

        {[TOP, 60, 105, 150, HEIGHT - BOTTOM].map((y) => (
          <line
            key={y}
            x1="0"
            x2={WIDTH}
            y1={y}
            y2={y}
            stroke="var(--border)"
            strokeWidth="1"
            shapeRendering="crispEdges"
          />
        ))}

        <path d={areaPath} fill="url(#trend-score-area)" stroke="none" />
        <path
          d={scorePath}
          fill="none"
          stroke="var(--primary)"
          strokeWidth="2"
          strokeLinejoin="miter"
          strokeLinecap="butt"
        />
        <path
          d={stepPath(latencies, latencyScale)}
          fill="none"
          stroke="var(--accent)"
          strokeWidth="1.5"
          strokeDasharray="6 5"
          strokeLinejoin="miter"
          strokeLinecap="butt"
        />

        {data.map((row, index) => {
          if (index !== 0 && index !== data.length - 1 && index % labelEvery !== 0) return null;
          const [x] = scoreScale(index, row.avg_score);
          return (
            <text
              key={`${row.day}-${index}`}
              x={x}
              y={HEIGHT - 8}
              textAnchor={index === 0 ? "start" : index === data.length - 1 ? "end" : "middle"}
              fill="var(--muted-foreground)"
              fontSize="10"
              fontFamily="var(--font-mono-face), monospace"
            >
              {row.day.slice(5)}
            </text>
          );
        })}
      </svg>
    </div>
  );
}
