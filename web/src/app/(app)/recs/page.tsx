"use client";

import { Suspense, useCallback, useEffect, useMemo, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import Link from "next/link";
import FeaturePageHeader from "@/components/shell/feature-page-header";
import DataTable from "@/components/data-table/table";
import type { Columns } from "@/components/data-table/columns";
import { cn } from "@/lib/utils";
import { useSoftPoll } from "@/hooks/use-soft-poll";
import { useExclusiveFlagTab } from "@/hooks/use-flag-query-param";
import { recommendationTypeLabel } from "@/lib/recommendation-type-label";
import type {
  Recommendation,
  RecommendationsDashboard,
} from "@/lib/so/misc";

type StatusTab = "open" | "resolved" | "dismissed";

const REC_TABS = ["resolved", "dismissed"] as const;

export default function RecsPage() {
  return (
    <Suspense
      fallback={
        <div className="grid h-full place-items-center text-sm text-neutral-500">
          Loading…
        </div>
      }
    >
      <RecsPageInner />
    </Suspense>
  );
}

function RecsPageInner() {
  const router = useRouter();
  const search = useSearchParams();
  const sessionFilter = search.get("session") || "";
  const [data, setData] = useState<RecommendationsDashboard | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [tab, setTab] = useExclusiveFlagTab<StatusTab>("open", REC_TABS);

  const load = useCallback(async () => {
    try {
      const r = await fetch("/api/recommendations?view=dashboard");
      if (!r.ok) throw new Error(await r.text());
      setData((await r.json()) as RecommendationsDashboard);
      setError("");
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  useSoftPoll(
    useCallback(() => {
      void load();
    }, [load]),
    8000
  );

  const summary = data?.summary || { open: 0, resolved: 0, dismissed: 0 };

  const rows = useMemo(() => {
    let list = data?.items || [];
    if (sessionFilter) {
      list = list.filter(
        (r) =>
          r.session_id === sessionFilter ||
          (r.related_sessions || []).includes(sessionFilter)
      );
    }
    return list.filter((r) => {
      const s = String(r.status || "pending");
      if (tab === "open") return s === "pending";
      if (tab === "resolved") return s === "applied";
      return s === "dismissed";
    });
  }, [data, sessionFilter, tab]);

  return (
    <div className="flex h-full min-h-0 flex-col bg-white">
      <FeaturePageHeader title="Recommendations" />
      <div className="min-h-0 flex-1 overflow-auto">
        <div className="border-b border-neutral-100 px-4 py-3">
          <div className="grid gap-2 sm:grid-cols-3">
            <StatCard
              label="Open"
              value={summary.open}
              active={tab === "open"}
              onClick={() => setTab("open")}
              tone="open"
            />
            <StatCard
              label="Resolved"
              value={summary.resolved}
              active={tab === "resolved"}
              onClick={() => setTab("resolved")}
              tone="resolved"
            />
            <StatCard
              label="Dismissed"
              value={summary.dismissed}
              active={tab === "dismissed"}
              onClick={() => setTab("dismissed")}
              tone="dismissed"
            />
          </div>
        </div>

        <div className="space-y-3 p-4">
          {sessionFilter && (
            <p className="text-xs text-neutral-500">
              Filtered to session{" "}
              <span className="font-mono text-neutral-700">
                {sessionFilter.length > 20
                  ? sessionFilter.slice(0, 12) + "…"
                  : sessionFilter}
              </span>
              {" · "}
              <Link href="/recs" className="underline">
                Show all
              </Link>
            </p>
          )}

          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="flex items-center gap-1 rounded-md border border-neutral-200 p-0.5">
              {(
                [
                  ["open", "Open", summary.open],
                  ["resolved", "Resolved", summary.resolved],
                  ["dismissed", "Dismissed", summary.dismissed],
                ] as const
              ).map(([id, label, count]) => (
                <button
                  key={id}
                  type="button"
                  onClick={() => setTab(id)}
                  className={cn(
                    "rounded px-2.5 py-1 text-xs font-medium",
                    tab === id
                      ? "bg-neutral-900 text-white"
                      : "text-neutral-600 hover:bg-neutral-50"
                  )}
                >
                  {label}{" "}
                  <span className="tabular-nums opacity-80">{count}</span>
                </button>
              ))}
            </div>
          </div>

          {error && <p className="text-sm text-red-600">{error}</p>}

          <DataTable
            columns={REC_COLS}
            data={rows}
            isFetched={!loading}
            isLoading={loading}
            visibilityColumns={{ recommendation: true }}
            emptyTitle={
              tab === "open"
                ? "No open recommendations"
                : tab === "resolved"
                  ? "No resolved recommendations"
                  : "No dismissed recommendations"
            }
            emptyBody="Eval scores sync recommendations here automatically."
            onClick={(row: Recommendation) => {
              router.push(`/recs/${encodeURIComponent(row.id)}`);
            }}
          />
        </div>
      </div>
    </div>
  );
}

const REC_COLS: Columns<"recommendation", Recommendation> = {
  recommendation: {
    header: () => "Recommendation",
    width: "minmax(0, 1fr)",
    cell: ({ row }) => <RecRowContent row={row} />,
  },
};

function RecRowContent({ row }: { row: Recommendation }) {
  const severity = severityFor(row);
  const type = String(row.type || "rec");
  const when = relativeTime(row.created_at);
  const path = row.proposed_path
    ? shortPath(String(row.proposed_path))
    : null;
  const session = row.session_id || row.related_sessions?.[0];

  return (
    <div className="min-w-0 py-0.5">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-semibold text-neutral-900">
          {row.title || row.id}
        </span>
        <SeverityBadge severity={severity} />
        <span className="rounded border border-neutral-200 bg-white px-1.5 py-0.5 text-[10px] font-medium capitalize text-neutral-600">
          {recommendationTypeLabel(type)}
        </span>
      </div>
      <div className="mt-1 flex flex-wrap items-center gap-x-1.5 gap-y-0.5 text-[11px] text-neutral-500">
        <span className="font-mono">#{shortId(row.id)}</span>
        {when && (
          <>
            <span>·</span>
            <span>
              {statusVerb(row.status)} {when}
            </span>
          </>
        )}
        {session && (
          <>
            <span>·</span>
            <span>
              Detected in{" "}
              <span className="font-mono text-neutral-600">
                {session.length > 16 ? session.slice(0, 10) + "…" : session}
              </span>
            </span>
          </>
        )}
        {path && (
          <>
            <span>·</span>
            <span className="font-mono text-neutral-600">{path}</span>
          </>
        )}
        {row.rationale && (
          <>
            <span>·</span>
            <span className="line-clamp-1 max-w-md">{row.rationale}</span>
          </>
        )}
      </div>
    </div>
  );
}

function StatCard({
  label,
  value,
  active,
  onClick,
  tone,
}: {
  label: string;
  value: number;
  active?: boolean;
  onClick?: () => void;
  tone: "open" | "resolved" | "dismissed";
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "rounded-lg border px-3 py-2.5 text-left transition-colors",
        active
          ? "border-neutral-900 bg-neutral-50"
          : "border-neutral-200 bg-white hover:border-neutral-300"
      )}
    >
      <div className="text-[11px] text-neutral-500">{label}</div>
      <div
        className={cn(
          "mt-0.5 text-xl font-semibold tabular-nums",
          tone === "open" && "text-amber-700",
          tone === "resolved" && "text-emerald-700",
          tone === "dismissed" && "text-neutral-500"
        )}
      >
        {value}
      </div>
    </button>
  );
}

function SeverityBadge({ severity }: { severity: "critical" | "high" | "medium" | "low" }) {
  const tone =
    severity === "critical"
      ? "bg-red-50 text-red-800 border-red-200"
      : severity === "high"
        ? "bg-orange-50 text-orange-800 border-orange-200"
        : severity === "medium"
          ? "bg-amber-50 text-amber-900 border-amber-200"
          : "bg-neutral-100 text-neutral-600 border-neutral-200";
  return (
    <span
      className={cn(
        "rounded border px-1.5 py-0.5 text-[10px] font-semibold capitalize",
        tone
      )}
    >
      {severity}
    </span>
  );
}

function severityFor(row: Recommendation): "critical" | "high" | "medium" | "low" {
  const t = String(row.type || "").toLowerCase();
  if (t === "guardrail") return "critical";
  if (t === "skill") return "high";
  if (t === "docs" || t === "knowledge" || t === "graph") return "medium";
  return "low";
}

function statusVerb(status?: string) {
  const s = String(status || "pending");
  if (s === "applied") return "resolved";
  if (s === "dismissed") return "dismissed";
  return "opened";
}

function shortId(id: string) {
  if (id.length <= 14) return id;
  return id.slice(0, 12);
}

function shortPath(p: string) {
  const parts = p.replace(/\\/g, "/").split("/");
  if (parts.length <= 3) return p;
  return parts.slice(-3).join("/");
}

function relativeTime(iso?: string) {
  if (!iso) return "";
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return "";
  const sec = Math.round((Date.now() - t) / 1000);
  if (sec < 60) return "just now";
  if (sec < 3600) return `${Math.floor(sec / 60)} minutes ago`;
  if (sec < 86400) return `${Math.floor(sec / 3600)} hours ago`;
  if (sec < 86400 * 30) return `${Math.floor(sec / 86400)} days ago`;
  if (sec < 86400 * 365) return `${Math.floor(sec / (86400 * 30))} months ago`;
  return `${Math.floor(sec / (86400 * 365))} years ago`;
}
