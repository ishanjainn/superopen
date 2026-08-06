"use client";

import { Suspense, useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import FeaturePageHeader from "@/components/shell/feature-page-header";
import { HarnessSingleDocPage } from "@/components/harness-single-doc-page";
import { BarSeries, Sparkline } from "@/components/harness/charts";
import DataTable from "@/components/data-table/table";
import type { Columns } from "@/components/data-table/columns";
import { useProject } from "@/components/shell/project-context";
import { cn } from "@/lib/utils";
import type {
  GuardrailsDashboard,
  GuardLogStatus,
} from "@/lib/so/guardrails-dashboard";
import { useSoftPoll } from "@/hooks/use-soft-poll";
import { useFlagQueryParam } from "@/hooks/use-flag-query-param";
import { harnessItemHref, type HarnessItemKind } from "@/lib/so/harness-yaml";

type PageTab = "dashboard" | "guards";
type TableTab = "guards" | "logs";
type LogFilter = "all" | GuardLogStatus;

export default function GuardrailsPage() {
  return (
    <Suspense
      fallback={
        <div className="grid h-full place-items-center text-sm text-neutral-500">
          Loading guardrails…
        </div>
      }
    >
      <GuardrailsInner />
    </Suspense>
  );
}

function GuardrailsInner() {
  const { projectId } = useProject();
  const [guardsOn, setGuardsOn] = useFlagQueryParam("guards");
  const tab: PageTab = guardsOn ? "guards" : "dashboard";
  const setTab = (next: PageTab) => setGuardsOn(next === "guards");

  return (
    <div className="flex h-full min-h-0 flex-col bg-white">
      <FeaturePageHeader
        title="Guardrails"
        actions={
          <div className="flex items-center gap-1 rounded-md border border-neutral-200 p-0.5">
            {(
              [
                ["dashboard", "Dashboard"],
                ["guards", "Guards"],
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
        <GuardrailsDashboardView
          projectId={projectId}
          onOpenGuards={() => setTab("guards")}
        />
      ) : (
        <div className="min-h-0 flex-1">
          <HarnessSingleDocPage kind="guardrails" embedded />
        </div>
      )}
    </div>
  );
}

function GuardrailsDashboardView({
  projectId,
  onOpenGuards,
}: {
  projectId: string;
  onOpenGuards: () => void;
}) {
  const router = useRouter();
  const [data, setData] = useState<GuardrailsDashboard | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [tableTab, setTableTab] = useState<TableTab>("guards");
  const [logFilter, setLogFilter] = useState<LogFilter>("all");

  const load = useCallback(async (opts?: { quiet?: boolean }) => {
    if (!opts?.quiet) setLoading(true);
    setError("");
    try {
      const params = new URLSearchParams();
      if (projectId && projectId !== "all") params.set("project", projectId);
      const r = await fetch(`/api/guardrails/dashboard?${params.toString()}`);
      if (!r.ok) throw new Error(await r.text());
      setData((await r.json()) as GuardrailsDashboard);
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
    const list = data?.logs || [];
    if (logFilter === "all") {
      // Hide soft policy mismatches from default All noise.
      return list.filter((l) => l.status !== "skipped");
    }
    return list.filter((l) => l.status === logFilter);
  }, [data, logFilter]);

  if (loading) {
    return (
      <div className="grid flex-1 place-items-center text-sm text-neutral-500">
        Loading guardrails…
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
    total_guards: 0,
    hard_stops: 0,
    advisory: 0,
    denials: 0,
    denials_7d: 0,
    triggers: 0,
    passes: 0,
    skips: 0,
  };

  if (s.total_guards === 0) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center gap-3 px-6 py-16 text-center">
        <h2 className="text-base font-semibold text-neutral-900">No guards yet</h2>
        <p className="max-w-md text-sm text-neutral-500">
          Add denied commands, sensitive paths, or advisory rules. Usage and denials from
          hooks will show up on this dashboard.
        </p>
        <button
          type="button"
          onClick={onOpenGuards}
          className="mt-2 rounded-md bg-neutral-900 px-3 py-1.5 text-xs text-white"
        >
          Open Guards
        </button>
      </div>
    );
  }

  const guards = data?.guards || [];
  const series = data?.series || [];

  return (
    <div className="min-h-0 flex-1 overflow-auto">
      <div className="border-b border-neutral-100 px-4 py-3">
        <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
          <StatCard label="Triggers" value={s.triggers} tone="ok" />
          <StatCard label="Denials" value={s.denials} tone="poor" />
          <StatCard label="Passed" value={s.passes} tone="good" />
          <StatCard label="Skipped" value={s.skips} />
        </div>
      </div>

      <div className="space-y-6 p-4">
        <div className="grid gap-4 lg:grid-cols-2">
          <div className="rounded-lg border border-neutral-200 p-3">
            <h3 className="text-sm font-medium text-neutral-900">Triggers over time</h3>
            <p className="mb-2 text-[11px] text-neutral-400">Denies + hard stops per day</p>
            <BarSeries
              data={series.map((d) => ({ label: d.date, value: d.triggers }))}
              barClassName="bg-amber-700"
              empty="No triggers recorded yet"
            />
          </div>
          <div className="rounded-lg border border-neutral-200 p-3">
            <h3 className="text-sm font-medium text-neutral-900">Denials over time</h3>
            <p className="mb-2 text-[11px] text-neutral-400">Hard policy denies per day</p>
            <BarSeries
              data={series.map((d) => ({ label: d.date, value: d.denials }))}
              barClassName="bg-red-700"
              empty="No denials recorded yet"
            />
          </div>
        </div>

        <div>
          <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
            <div className="flex items-center gap-1 rounded-md border border-neutral-200 p-0.5">
              {(
                [
                  ["guards", "Guards"],
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
              onClick={onOpenGuards}
              className="text-[11px] text-neutral-500 underline-offset-2 hover:underline"
            >
              Manage
            </button>
          </div>

          {tableTab === "guards" ? (
            <DataTable
              columns={GUARD_DASH_COLS}
              data={guards}
              isFetched
              isLoading={false}
              visibilityColumns={{
                kind: true,
                label: true,
                detail: true,
                triggers: true,
                denials: true,
                passes: true,
                trend: true,
              }}
              emptyTitle="No guards"
              emptyBody="Add guards from the Guards tab."
              onClick={(row: GuardrailsDashboard["guards"][number]) => {
                router.push(
                  harnessItemHref("guardrails", row.kind as HarnessItemKind, row.label)
                );
              }}
            />
          ) : (
            <div className="space-y-2">
              <div className="flex flex-wrap gap-1">
                {(
                  [
                    ["all", "All"],
                    ["denied", "Denied"],
                    ["triggered", "Triggered"],
                    ["passed", "Passed"],
                    ["skipped", "Policy mismatch"],
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
                  guard: true,
                  detail: true,
                  at: true,
                  session: true,
                }}
                emptyTitle="No guard logs"
                emptyBody="Hook denials and policy events will appear here."
              />
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

type GuardRow = GuardrailsDashboard["guards"][number];
type LogRow = GuardrailsDashboard["logs"][number];

const GUARD_DASH_COLS: Columns<
  "kind" | "label" | "detail" | "triggers" | "denials" | "passes" | "trend",
  GuardRow
> = {
  kind: {
    header: () => "Type",
    width: "7rem",
    cell: ({ row }) => (
      <span className="rounded-md border border-neutral-200 bg-white px-2 py-0.5 text-[10px] capitalize text-neutral-700">
        {row.kind}
      </span>
    ),
  },
  label: {
    header: () => "Guard",
    width: "minmax(10rem, 1.4fr)",
    cell: ({ row }) => (
      <span className="block truncate font-mono text-xs text-neutral-800">{row.label}</span>
    ),
  },
  detail: {
    header: () => "What it does",
    width: "minmax(10rem, 1.6fr)",
    cell: ({ row }) => (
      <span className="line-clamp-2 text-sm text-neutral-700">{row.detail}</span>
    ),
  },
  triggers: {
    header: () => "Triggers",
    width: "5.5rem",
    cell: ({ row }) => (
      <span className="tabular-nums text-sm">{row.triggers}</span>
    ),
  },
  denials: {
    header: () => "Denials",
    width: "5.5rem",
    cell: ({ row }) => (
      <span className="tabular-nums text-sm">{row.denials}</span>
    ),
  },
  passes: {
    header: () => "Passed",
    width: "5rem",
    cell: ({ row }) => (
      <span className="tabular-nums text-sm">{row.passes}</span>
    ),
  },
  trend: {
    header: () => "Trend",
    width: "5.5rem",
    cell: ({ row }) => (
      <Sparkline values={row.trend?.length ? row.trend : [0]} stroke="#b91c1c" />
    ),
  },
};

const LOG_COLS: Columns<"status" | "guard" | "detail" | "at" | "session", LogRow> = {
  status: {
    header: () => "Status",
    width: "6.5rem",
    cell: ({ row }) => <StatusBadge status={row.status} />,
  },
  guard: {
    header: () => "Guard",
    width: "minmax(8rem, 1.2fr)",
    cell: ({ row }) => (
      <span className="block truncate font-mono text-xs">{row.guard}</span>
    ),
  },
  detail: {
    header: () => "Detail",
    width: "minmax(10rem, 1.6fr)",
    cell: ({ row }) => (
      <span className="line-clamp-2 text-sm text-neutral-700">{row.detail || "-"}</span>
    ),
  },
  at: {
    header: () => "When",
    width: "8rem",
    cell: ({ row }) => (
      <span className="text-[11px] text-neutral-500">
        {row.at ? new Date(row.at).toLocaleString() : "-"}
      </span>
    ),
  },
  session: {
    header: () => "Session",
    width: "6rem",
    cell: ({ row }) =>
      row.session_id ? (
        <Link
          href={`/sessions/${encodeURIComponent(row.session_id)}`}
          className="font-mono text-[11px] underline"
        >
          {row.session_id.slice(0, 8)}
        </Link>
      ) : (
        <span className="text-neutral-400">-</span>
      ),
  },
};

function StatusBadge({ status }: { status: GuardLogStatus }) {
  const tone =
    status === "denied" || status === "triggered"
      ? "bg-red-50 text-red-700 border-red-200"
      : status === "passed"
        ? "bg-emerald-50 text-emerald-700 border-emerald-200"
        : "bg-neutral-100 text-neutral-600 border-neutral-200";
  return (
    <span
      className={cn(
        "rounded border px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide",
        tone
      )}
    >
      {status}
    </span>
  );
}

function StatCard({
  label,
  value,
  tone,
}: {
  label: string;
  value: number;
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
