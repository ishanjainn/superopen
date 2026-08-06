"use client";

import { cn } from "@/lib/utils";

/** Minimal sparkline (pass=high). */
export function Sparkline({
  values,
  className,
  stroke = "#171717",
}: {
  values: number[];
  className?: string;
  stroke?: string;
}) {
  if (!values.length) {
    return <span className={cn("text-[10px] tabular-nums text-neutral-400", className)}>0</span>;
  }
  const w = 64;
  const h = 20;
  const max = Math.max(...values, 1);
  const min = Math.min(...values, 0);
  const span = Math.max(max - min, 0.001);
  const pts = values
    .map((v, i) => {
      const x = values.length === 1 ? w / 2 : (i / (values.length - 1)) * w;
      const y = h - ((v - min) / span) * (h - 2) - 1;
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join(" ");
  return (
    <svg
      viewBox={`0 0 ${w} ${h}`}
      className={cn("inline-block h-5 w-16", className)}
      aria-hidden
    >
      <polyline
        fill="none"
        stroke={stroke}
        strokeWidth="1.5"
        strokeLinejoin="round"
        strokeLinecap="round"
        points={pts}
      />
    </svg>
  );
}

type SeriesPoint = { label: string; value: number };

/** Vertical bar chart for executions / denials over time. */
export function BarSeries({
  data,
  className,
  barClassName = "bg-neutral-900",
  empty = "No data in range",
}: {
  data: SeriesPoint[];
  className?: string;
  barClassName?: string;
  empty?: string;
}) {
  if (!data.length || data.every((d) => !d.value)) {
    return (
      <div
        className={cn(
          "flex h-40 items-center justify-center rounded-lg border border-dashed border-neutral-200 text-xs text-neutral-400",
          className
        )}
      >
        {empty}
      </div>
    );
  }
  const max = Math.max(...data.map((d) => d.value), 1);
  return (
    <div className={cn("flex h-40 flex-col", className)}>
      <div className="flex min-h-0 flex-1 items-end gap-1 px-1">
        {data.map((d) => (
          <div
            key={d.label}
            className="flex min-w-0 flex-1 flex-col items-center justify-end"
            title={`${d.label}: ${d.value}`}
          >
            <div
              className={cn("w-full max-w-[1.25rem] rounded-t-sm", barClassName)}
              style={{ height: `${Math.max((d.value / max) * 100, d.value ? 4 : 0)}%` }}
            />
          </div>
        ))}
      </div>
      <div className="mt-1 flex gap-1 border-t border-neutral-100 pt-1">
        {data.map((d, i) => (
          <div
            key={d.label}
            className="min-w-0 flex-1 truncate text-center text-[9px] text-neutral-400"
          >
            {i === 0 || i === data.length - 1 || i === Math.floor(data.length / 2)
              ? d.label.slice(5)
              : ""}
          </div>
        ))}
      </div>
    </div>
  );
}

/** Area-ish pass-rate line using SVG. */
export function PassRateChart({
  data,
  className,
}: {
  data: { label: string; value: number }[];
  className?: string;
}) {
  if (!data.length) {
    return (
      <div
        className={cn(
          "flex h-40 items-center justify-center rounded-lg border border-dashed border-neutral-200 text-xs text-neutral-400",
          className
        )}
      >
        No pass-rate data yet
      </div>
    );
  }
  const w = 320;
  const h = 120;
  const pad = 8;
  const pts = data.map((d, i) => {
    const x =
      pad +
      (data.length === 1 ? (w - pad * 2) / 2 : (i / (data.length - 1)) * (w - pad * 2));
    const y = pad + (1 - Math.min(Math.max(d.value, 0), 1)) * (h - pad * 2);
    return [x, y] as const;
  });
  const line = pts.map(([x, y]) => `${x},${y}`).join(" ");
  const area = `${pad},${h - pad} ${line} ${w - pad},${h - pad}`;
  return (
    <div className={cn("h-40 w-full", className)}>
      <svg viewBox={`0 0 ${w} ${h}`} className="h-full w-full" preserveAspectRatio="none">
        <polygon fill="#f5f5f5" points={area} />
        <polyline
          fill="none"
          stroke="#171717"
          strokeWidth="2"
          strokeLinejoin="round"
          points={line}
        />
      </svg>
    </div>
  );
}
