"use client";

import { Suspense, useCallback, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import FeaturePageHeader from "@/components/shell/feature-page-header";
import { HarnessSingleDocPage } from "@/components/harness-single-doc-page";
import { BarSeries, PassRateChart, Sparkline } from "@/components/harness/charts";
import DataTable from "@/components/data-table/table";
import type { Columns } from "@/components/data-table/columns";
import { useProject } from "@/components/shell/project-context";
import { cn } from "@/lib/utils";
import type { EvalsDashboard, EvalRun } from "@/lib/so/evals";
import { useSoftPoll } from "@/hooks/use-soft-poll";
import { useFlagQueryParam } from "@/hooks/use-flag-query-param";
import { harnessItemHref } from "@/lib/so/harness-yaml";

type PageTab = "dashboard" | "evaluators";
type TableTab = "evaluators" | "logs";
type LogFilter = "all" | "failing" | "good" | "ok";

export default function EvaluationsPage() {
  return (
    <Suspense
      fallback={
        <div className="grid h-full place-items-center text-sm text-neutral-500">
          Loading evaluations…
        </div>
      }
    >
      <EvaluationsInner />
    </Suspense>
  );
}

function EvaluationsInner() {
  const { projectId } = useProject();
  const [evaluatorsOn, setEvaluatorsOn] = useFlagQueryParam("evaluators");
  const tab: PageTab = evaluatorsOn ? "evaluators" : "dashboard";
  const setTab = (next: PageTab) => setEvaluatorsOn(next === "evaluators");

  return (
    <div className="flex h-full min-h-0 flex-col bg-white">
      <FeaturePageHeader
        title="Evaluations"
        actions={
          <div className="flex items-center gap-1 rounded-md border border-neutral-200 p-0.5">
            {(
              [
                ["dashboard", "Dashboard"],
                ["evaluators", "Evaluators"],
              ] as const
            ).map(([id, label]) => (
              <button
                key={id}
                type="button"
                onClick={() => setTab(id)}
                className={cn(
                  "rounded px-2.5 py-1 text-xs font-medium transition-colors",
                  tab === id
                    ? "bg-neutral-900 text-white"
                    : "text-neutral-600 hover:bg-neutral-50 hover:text-neutral-900"
                )}
              >
                {label}
              </button>
            ))}
          </div>
        }
      />
      {tab === "dashboard" ? (
        <EvalsDashboardView
          projectId={projectId}
          onOpenEvaluators={() => setTab("evaluators")}
        />
      ) : (
        <div className="min-h-0 flex-1">
          <HarnessSingleDocPage kind="evaluations" embedded />
        </div>
      )}
    </div>
  );
}

function EvalsDashboardView({
  projectId,
  onOpenEvaluators,
}: {
  projectId: string;
  onOpenEvaluators: () => void;
}) {
  const router = useRouter();
  const [data, setData] = useState<EvalsDashboard | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [tableTab, setTableTab] = useState<TableTab>("evaluators");
  const [logFilter, setLogFilter] = useState<LogFilter>("all");

  const load = useCallback(async (opts?: { quiet?: boolean }) => {
    if (!opts?.quiet) setLoading(true);
    setError("");
    try {
      const params = new URLSearchParams();
      if (projectId) params.set("project", projectId);
      const r = await fetch(`/api/evals?${params.toString()}`);
      if (!r.ok) throw new Error(await r.text());
      setData((await r.json()) as EvalsDashboard);
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e));
    } finally {
      if (!opts?.quiet) setLoading(false);
    }
  }, [projectId]);

  useEffect(() => {
    void load();
  }, [load]);

  useSoftPoll(
    useCallback(() => {
      void load({ quiet: true });
    }, [load]),
    10000
  );

  const logs = useMemo(() => {
    const list = data?.runs || [];
    if (logFilter === "failing") {
      return list.filter(
        (r) =>
          r.badge === "poor" ||
          r.failure_points.some((point) => point.severity !== "info")
      );
    }
    if (logFilter === "good") return list.filter((r) => r.badge === "good");
    if (logFilter === "ok") return list.filter((r) => r.badge === "ok");
    return list;
  }, [data, logFilter]);

  if (loading) {
    return (
      <div className="grid flex-1 place-items-center text-sm text-neutral-500">
        Loading eval runs…
      </div>
    );
  }

  if (error) {
    return (
      <div className="p-6 text-sm text-red-600">
        {error}
        <button type="button" className="ml-3 underline" onClick={() => void load()}>
          Retry
        </button>
      </div>
    );
  }

  const s = data?.summary || {
    total: 0,
    executions: 0,
    good: 0,
    ok: 0,
    poor: 0,
    unknown: 0,
    with_failures: 0,
    avg_score: null,
    pass_rate: null,
    evaluator_count: 0,
    total_tokens: 0,
    total_cost_usd: 0,
    avg_tokens: null,
  };

  if (s.total === 0 && s.evaluator_count === 0) {
    return <EmptyDashboard onOpenEvaluators={onOpenEvaluators} />;
  }

  const evaluators = data?.evaluators || [];
  const series = data?.series || [];
  const executionSeries = data?.execution_series || [];

  return (
    <div className="min-h-0 flex-1 overflow-auto">
      <div className="border-b border-neutral-100 px-4 py-3">
        <div className="space-y-2">
          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-6">
            <StatCard label="Executions" value={s.executions ?? s.total} />
            <StatCard label="Sessions" value={s.total} />
            <StatCard
              label="Pass rate"
              value={s.pass_rate == null ? "—" : `${Math.round(s.pass_rate * 100)}%`}
              tone="good"
            />
            <StatCard label="Failures" value={s.with_failures} tone="poor" />
            <StatCard label="Unknown" value={s.unknown ?? 0} tone="ok" />
            <StatCard
              label="Avg score"
              value={s.avg_score == null ? "—" : `${Math.round(s.avg_score * 100)}%`}
            />
          </div>
          <div className="grid gap-2 sm:grid-cols-2">
            <StatCard label="Tokens" value={fmtTokens(s.total_tokens)} />
            <StatCard label="Cost" value={fmtCost(s.total_cost_usd || 0)} />
          </div>
        </div>
      </div>

      <div className="space-y-6 p-4">
        <div className="grid gap-4 lg:grid-cols-2">
          <ChartCard title="Executions" subtitle="Runs per day">
            <BarSeries
              data={executionSeries.map((d) => ({ label: d.date, value: d.runs }))}
              name="Runs"
              color="#525252"
            />
          </ChartCard>
          <ChartCard title="Pass rate" subtitle="Good / total by day">
            <PassRateChart
              data={series.map((d) => ({ label: d.date, value: d.pass_rate }))}
            />
          </ChartCard>
          <ChartCard title="Token usage" subtitle="Tokens on scored sessions per day">
            <BarSeries
              data={series.map((d) => ({ label: d.date, value: d.tokens }))}
              name="Tokens"
              color="#0284c7"
              valueFormatter={fmtTokens}
              empty="No token data yet"
            />
          </ChartCard>
          <ChartCard title="Cost" subtitle="Estimated USD per day">
            <BarSeries
              data={series.map((d) => ({
                label: d.date,
                value: Math.round(d.cost_usd * 10000) / 10000,
              }))}
              name="Cost"
              color="#7c3aed"
              valueFormatter={fmtCost}
              allowDecimals
              empty="No cost data yet"
            />
          </ChartCard>
        </div>

        <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_240px]">
          <div className="min-w-0">
            <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
              <div className="flex items-center gap-1 rounded-md border border-neutral-200 p-0.5">
                {(
                  [
                    ["evaluators", "Evaluators"],
                    ["logs", "Logs"],
                  ] as const
                ).map(([id, label]) => (
                  <button
                    key={id}
                    type="button"
                    onClick={() => setTableTab(id)}
                    className={cn(
                      "rounded px-2.5 py-1 text-xs font-medium",
                      tableTab === id
                        ? "bg-neutral-900 text-white"
                        : "text-neutral-600 hover:bg-neutral-50"
                    )}
                  >
                    {label}
                  </button>
                ))}
              </div>
              <button
                type="button"
                onClick={onOpenEvaluators}
                className="text-[11px] text-neutral-500 underline-offset-2 hover:underline"
              >
                Manage
              </button>
            </div>

            {tableTab === "evaluators" ? (
              <DataTable
                columns={EVAL_COLS}
                data={evaluators}
                isFetched
                isLoading={false}
                visibilityColumns={{
                  label: true,
                  kind: true,
                  executions: true,
                  pass_rate: true,
                  trend: true,
                }}
                emptyTitle="No evaluator stats yet"
                emptyBody="Configure evaluators and run so eval to populate this table."
                onClick={(row: EvalRow) => {
                  if (row.kind === "check") {
                    router.push(harnessItemHref("evaluations", "check", row.label));
                  }
                }}
              />
            ) : (
              <div className="space-y-2">
                <div className="flex flex-wrap gap-1">
                  {(
                    [
                      ["all", "All"],
                      ["failing", "Failed"],
                      ["good", "Passed"],
                      ["ok", "Warn"],
                    ] as const
                  ).map(([id, label]) => (
                    <button
                      key={id}
                      type="button"
                      onClick={() => setLogFilter(id)}
                      className={cn(
                        "rounded border px-2 py-0.5 text-[11px]",
                        logFilter === id
                          ? "border-neutral-900 bg-neutral-900 text-white"
                          : "border-neutral-200 text-neutral-600 hover:bg-neutral-50"
                      )}
                    >
                      {label}
                    </button>
                  ))}
                </div>
                <DataTable
                  columns={LOG_COLS}
                  data={logs}
                  isFetched
                  isLoading={false}
                  visibilityColumns={{
                    status: true,
                    session: true,
                    scope: true,
                    score: true,
                    tokens: true,
                    cost: true,
                    at: true,
                    failures: true,
                  }}
                  emptyTitle="No eval logs"
                  emptyBody="Run so eval or Map → Evaluate to see recent results."
                  onClick={(row: EvalRun) => {
                    router.push(`/sessions/${encodeURIComponent(row.session_id)}`);
                  }}
                />
              </div>
            )}
          </div>

          <aside className="space-y-4">
            <div>
              <h3 className="text-[11px] font-semibold uppercase tracking-wide text-neutral-500">
                Top failure reasons
              </h3>
              {(data?.failure_reasons || []).length === 0 ? (
                <p className="mt-2 text-xs text-neutral-400">No failures recorded yet.</p>
              ) : (
                <ul className="mt-2 space-y-1.5">
                  {(data?.failure_reasons || []).map((r) => (
                    <li
                      key={r.reason}
                      className="flex items-start justify-between gap-2 rounded-md border border-neutral-200 px-2.5 py-1.5 text-xs"
                    >
                      <span className="min-w-0 text-neutral-700 line-clamp-2">
                        {r.reason}
                      </span>
                      <span className="shrink-0 font-medium tabular-nums text-neutral-900">
                        {r.count}
                      </span>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </aside>
        </div>
      </div>
    </div>
  );
}

type EvalRow = EvalsDashboard["evaluators"][number];

const EVAL_COLS: Columns<"label" | "kind" | "executions" | "pass_rate" | "trend", EvalRow> = {
  label: {
    header: () => "Evaluation",
    width: "minmax(10rem, 1.6fr)",
    cell: ({ row }) => (
      <span className="line-clamp-2 text-sm text-neutral-800">{row.label}</span>
    ),
  },
  kind: {
    header: () => "Type",
    width: "7rem",
    cell: ({ row }) => (
      <span className="rounded-md border border-neutral-200 bg-white px-2 py-0.5 text-[10px] text-neutral-600">
        {row.kind.replace("_", " ")}
      </span>
    ),
  },
  executions: {
    header: () => "Executions",
    width: "6.5rem",
    cell: ({ row }) => (
      <span className="tabular-nums text-sm">{row.executions}</span>
    ),
  },
  pass_rate: {
    header: () => "Pass rate",
    width: "6.5rem",
    cell: ({ row }) => (
      <span className="tabular-nums text-sm">
        {row.pass_rate == null ? "—" : `${Math.round(row.pass_rate * 100)}%`}
      </span>
    ),
  },
  trend: {
    header: () => "Trend",
    width: "5.5rem",
    cell: ({ row }) =>
      row.trend?.length ? (
        <Sparkline values={row.trend} />
      ) : (
        <span className="text-neutral-400">—</span>
      ),
  },
};

const LOG_COLS: Columns<
  "status" | "session" | "scope" | "score" | "tokens" | "cost" | "at" | "failures",
  EvalRun
> = {
  status: {
    header: () => "Status",
    width: "5.5rem",
    cell: ({ row }) => (
      <span
        className={cn(
          "rounded border px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide",
          badgeClass(row.badge)
        )}
      >
        {row.badge === "good" ? "passed" : row.badge === "poor" ? "failed" : row.badge}
      </span>
    ),
  },
  session: {
    header: () => "Session",
    width: "minmax(8rem, 1.4fr)",
    cell: ({ row }) => (
      <span className="line-clamp-1 text-sm text-neutral-800">
        {row.title || row.session_id.slice(0, 8)}
      </span>
    ),
  },
  scope: {
    header: () => "Coverage",
    width: "6.5rem",
    cell: ({ row }) => (
      <span className="text-[11px] text-neutral-600">
        {row.scope === "complete" ? "Whole chat" : "Snapshot"}
      </span>
    ),
  },
  score: {
    header: () => "Score",
    width: "4.5rem",
    cell: ({ row }) => (
      <span className="tabular-nums text-sm">
        {row.evidence_status === "insufficient" || typeof row.score !== "number"
          ? "—"
          : `${Math.round(row.score * 100)}%`}
      </span>
    ),
  },
  tokens: {
    header: () => "Tokens",
    width: "5rem",
    cell: ({ row }) => (
      <span className="tabular-nums text-sm">
        {fmtTokens(row.tokens || 0)}
      </span>
    ),
  },
  cost: {
    header: () => "Cost",
    width: "4.5rem",
    cell: ({ row }) => (
      <span className="tabular-nums text-sm">
        {fmtCost(row.cost_usd || 0)}
      </span>
    ),
  },
  at: {
    header: () => "When",
    width: "7.5rem",
    cell: ({ row }) => (
      <span className="text-[11px] text-neutral-500">
        {row.at ? new Date(row.at).toLocaleString() : "-"}
      </span>
    ),
  },
  failures: {
    header: () => "Failures",
    width: "minmax(8rem, 1.2fr)",
    cell: ({ row }) => {
      const n = row.failure_points.length;
      if (!n) return <span className="text-neutral-400">-</span>;
      return (
        <span className="line-clamp-2 text-xs text-neutral-700">
          {row.failure_points[0]?.label}
          {n > 1 ? ` (+${n - 1})` : ""}
        </span>
      );
    },
  },
};

function ChartCard({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle: string;
  children: React.ReactNode;
}) {
  return (
    <div className="rounded-lg border border-neutral-200 bg-white p-3">
      <div className="mb-2">
        <h3 className="text-sm font-medium text-neutral-900">{title}</h3>
        <p className="text-[11px] text-neutral-400">{subtitle}</p>
      </div>
      {children}
    </div>
  );
}

function StatCard({
  label,
  value,
  tone,
}: {
  label: string;
  value: number | string;
  tone?: "good" | "ok" | "poor";
}) {
  return (
    <div className="rounded-lg border border-neutral-200 px-3 py-2.5">
      <div className="text-[11px] text-neutral-500">{label}</div>
      <div
        className={cn(
          "mt-0.5 text-xl font-semibold tabular-nums",
          tone === "good" && "text-emerald-700",
          tone === "ok" && "text-amber-700",
          tone === "poor" && "text-red-700",
          !tone && "text-neutral-900"
        )}
      >
        {value}
      </div>
    </div>
  );
}

function badgeClass(badge: string) {
  if (badge === "good") return "bg-emerald-50 text-emerald-800 border-emerald-200";
  if (badge === "ok") return "bg-amber-50 text-amber-900 border-amber-200";
  if (badge === "unknown") return "bg-neutral-100 text-neutral-600 border-neutral-200";
  return "bg-red-50 text-red-800 border-red-200";
}

function fmtTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 10_000) return `${Math.round(n / 1000)}k`;
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`;
  return String(n);
}

function fmtCost(n: number): string {
  if (!n) return "$0";
  if (n < 0.01) return `$${n.toFixed(4)}`;
  return `$${n.toFixed(2)}`;
}

function EmptyDashboard({ onOpenEvaluators }: { onOpenEvaluators: () => void }) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-3 px-6 py-16 text-center">
      <h2 className="text-base font-semibold text-neutral-900">No eval runs yet</h2>
      <p className="max-w-md text-sm text-neutral-500">
        Score a session to populate this dashboard. Pass rate, executions, tokens, cost, and
        per-evaluator stats will show up here.
      </p>
      <div className="mt-2 rounded-lg border border-neutral-200 bg-neutral-50 px-4 py-3 text-left text-xs text-neutral-600">
        <p className="font-medium text-neutral-800">Try</p>
        <ol className="mt-1 list-decimal space-y-1 pl-4 font-mono text-[11px]">
          <li>so eval &lt;session-id&gt;</li>
          <li>Sessions → open a chat → Evaluate</li>
        </ol>
        <div className="mt-3 flex gap-2">
          <button
            type="button"
            onClick={onOpenEvaluators}
            className="rounded-md border border-neutral-300 bg-white px-2.5 py-1 text-[11px] text-neutral-700"
          >
            Open Evaluators
          </button>
        </div>
      </div>
    </div>
  );
}
