"use client";

import { cn } from "@/lib/utils";
import {
  Bar,
  BarChart,
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

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

const gridColor = "rgb(var(--color-neutral-200))";
const axisColor = "rgb(var(--color-neutral-500))";
const tooltipSurface = "rgb(var(--color-neutral-50))";
const tooltipText = "rgb(var(--color-neutral-900))";

export function chartDateLabel(value: string): string {
  const date = new Date(`${value}T00:00:00`);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

export function chartFullDate(value: string): string {
  const date = new Date(`${value}T00:00:00`);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

export function compactChartNumber(value: number): string {
  const compact = Math.abs(value) >= 1_000;
  return new Intl.NumberFormat(undefined, {
    notation: compact ? "compact" : "standard",
    maximumFractionDigits: compact ? 1 : Math.abs(value) >= 100 ? 0 : 2,
  }).format(value);
}

const tooltipStyle = {
  backgroundColor: tooltipSurface,
  border: `1px solid ${gridColor}`,
  borderRadius: "8px",
  color: tooltipText,
  fontSize: "12px",
};

/** Vertical bar chart for executions / denials over time. */
export function BarSeries({
  data,
  className,
  color = "#525252",
  name = "Value",
  valueFormatter = compactChartNumber,
  allowDecimals = false,
  empty = "No data in range",
}: {
  data: SeriesPoint[];
  className?: string;
  color?: string;
  name?: string;
  valueFormatter?: (value: number) => string;
  allowDecimals?: boolean;
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
  return (
    <div className={cn("h-48 w-full", className)}>
      <ResponsiveContainer width="100%" height="100%">
        <BarChart data={data} margin={{ top: 8, right: 8, bottom: 0, left: 0 }}>
          <CartesianGrid vertical={false} stroke={gridColor} strokeDasharray="3 3" />
          <XAxis
            dataKey="label"
            tickFormatter={chartDateLabel}
            tick={{ fill: axisColor, fontSize: 10 }}
            axisLine={{ stroke: gridColor }}
            tickLine={false}
            minTickGap={24}
          />
          <YAxis
            tickFormatter={valueFormatter}
            tick={{ fill: axisColor, fontSize: 10 }}
            axisLine={false}
            tickLine={false}
            width={48}
            domain={[0, "auto"]}
            allowDecimals={allowDecimals}
          />
          <Tooltip
            cursor={{ fill: "rgb(var(--color-neutral-100))" }}
            contentStyle={tooltipStyle}
            labelStyle={{ color: tooltipText, fontWeight: 600 }}
            labelFormatter={(label) => chartFullDate(String(label))}
            formatter={(value) => [valueFormatter(Number(value)), name]}
          />
          <Bar dataKey="value" name={name} fill={color} radius={[3, 3, 0, 0]} maxBarSize={28} />
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}

/** Pass-rate trend over time. */
export function PassRateChart({
  data,
  className,
}: {
  data: { label: string; value: number | null }[];
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
  return (
    <div className={cn("h-48 w-full", className)}>
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={data} margin={{ top: 8, right: 10, bottom: 0, left: 0 }}>
          <CartesianGrid vertical={false} stroke={gridColor} strokeDasharray="3 3" />
          <XAxis
            dataKey="label"
            tickFormatter={chartDateLabel}
            tick={{ fill: axisColor, fontSize: 10 }}
            axisLine={{ stroke: gridColor }}
            tickLine={false}
            minTickGap={24}
          />
          <YAxis
            domain={[0, 1]}
            ticks={[0, 0.25, 0.5, 0.75, 1]}
            tickFormatter={(value) => `${Math.round(Number(value) * 100)}%`}
            tick={{ fill: axisColor, fontSize: 10 }}
            axisLine={false}
            tickLine={false}
            width={42}
          />
          <Tooltip
            cursor={{ stroke: axisColor, strokeDasharray: "3 3" }}
            contentStyle={tooltipStyle}
            labelStyle={{ color: tooltipText, fontWeight: 600 }}
            labelFormatter={(label) => chartFullDate(String(label))}
            formatter={(value) => [
              `${Math.round(Number(value) * 100)}%`,
              "Pass rate",
            ]}
          />
          <Line
            type="monotone"
            dataKey="value"
            name="Pass rate"
            stroke="#059669"
            strokeWidth={2}
            connectNulls={false}
            dot={data.length <= 12 ? { r: 3, fill: "#059669" } : false}
            activeDot={{ r: 5 }}
          />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}
