"use client";

import { useCallback, useEffect, useState, type ReactNode, Suspense } from "react";
import dynamic from "next/dynamic";
import { useParams } from "next/navigation";
import Link from "next/link";
import { Map as MapIcon, RefreshCw, ClipboardCheck } from "lucide-react";
import SessionTimeline, {
  type SessionMeta,
  type Span,
  type RestoreCheckpoint,
} from "@/components/session-timeline";
import FeaturePageHeader, {
  FeatureBackLink,
} from "@/components/shell/feature-page-header";
import { useBreadcrumbCrumb } from "@/components/shell/breadcrumb-context";
import { useProject } from "@/components/shell/project-context";
import { cn } from "@/lib/utils";
import { useSoftPoll } from "@/hooks/use-soft-poll";
import { useFlagQueryParam } from "@/hooks/use-flag-query-param";

type NestedSession = {
  id: string;
  title?: string;
  prompt_preview?: string;
  vendor?: string;
  model?: string;
  status?: string;
  started_at?: string;
  ended_at?: string;
  tokens?: number;
  cost_usd?: number;
  is_subagent?: boolean;
  parent_id?: string;
};
type ReviewFinding = {
  fingerprint: string; kind: string; summary: string; vendor: string;
  confidence?: number; verified?: boolean; evidence?: string[];
};

const MapView = dynamic(() => import("@/map"), {
  ssr: false,
  loading: () => (
    <div className="grid h-full place-items-center text-sm text-neutral-500">
      Loading map…
    </div>
  ),
});

type Tab = "chat" | "map";

function projectFromURL(): string {
  if (typeof window === "undefined") return "";
  try {
    return new URL(window.location.href).searchParams.get("project") || "";
  } catch {
    return "";
  }
}

export default function SessionDetailPage() {
  return (
    <Suspense
      fallback={
        <div className="grid h-full place-items-center text-sm text-neutral-500">
          Loading session…
        </div>
      }
    >
      <SessionDetailInner />
    </Suspense>
  );
}

function SessionDetailInner() {
  const params = useParams<{ id: string }>();
  const id = decodeURIComponent(params.id);
  const { projectId: globalProject } = useProject();
  const [project, setProject] = useState("");
  const [mapOn, setMapOn] = useFlagQueryParam("map");
  const tab: Tab = mapOn ? "map" : "chat";
  const setTab = (next: Tab) => setMapOn(next === "map");
  const [meta, setMeta] = useState<SessionMeta | null>(null);
  const [spans, setSpans] = useState<Span[]>([]);
  const [footprint, setFootprint] = useState<any>(null);
  const [checkpoints, setCheckpoints] = useState<RestoreCheckpoint[]>([]);
  const [subagents, setSubagents] = useState<NestedSession[]>([]);
  const [findings, setFindings] = useState<ReviewFinding[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [evaluating, setEvaluating] = useState(false);
  const [evalStatus, setEvalStatus] = useState("");

  useEffect(() => {
    const fromUrl = projectFromURL();
    setProject(fromUrl || globalProject);
  }, [globalProject]);

  const sessionTitle = meta?.title || meta?.prompt_preview || id;

  useBreadcrumbCrumb(
    loading ? null : String(sessionTitle).slice(0, 96) || null
  );

  const loadDetail = useCallback(async (opts?: { quiet?: boolean }) => {
    if (!opts?.quiet) setLoading(true);
    const qs = project ? `?project=${encodeURIComponent(project)}` : "";
    try {
      const r = await fetch(`/api/sessions/${encodeURIComponent(id)}${qs}`);
      if (!r.ok) throw new Error(await r.text());
      const data = await r.json();
      setMeta(data.meta || { id });
      setSpans(Array.isArray(data.transcript) ? data.transcript : []);
      setFootprint(data.footprint || null);
      setCheckpoints(Array.isArray(data.checkpoints) ? data.checkpoints : []);
      setSubagents(Array.isArray(data.subagents) ? data.subagents : []);
      setFindings(Array.isArray(data.findings) ? data.findings : []);
      setError("");
    } catch (e: any) {
      setError(String(e.message || e));
    } finally {
      if (!opts?.quiet) setLoading(false);
    }
  }, [id, project]);

  const refreshDetail = useCallback(async () => {
    setRefreshing(true);
    try {
      await loadDetail({ quiet: true });
    } finally {
      setRefreshing(false);
    }
  }, [loadDetail]);

  const runEvaluation = useCallback(async () => {
    setEvaluating(true);
    setEvalStatus("");
    setError("");
    try {
      const params = new URLSearchParams();
      if (project && project !== "all") params.set("project", project);
      const r = await fetch(`/api/evals?${params.toString()}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ session_id: id, force: true }),
      });
      if (!r.ok) throw new Error(await r.text());
      const payload = (await r.json()) as {
        result?: { badge?: string; evidence_status?: string };
        reused?: boolean;
        scope?: "complete" | "snapshot";
        skip_reason?: string;
      };
      const badge = payload.result?.badge || "complete";
      if (payload.reused) {
        setEvalStatus("Already has a final whole-chat evaluation.");
      } else if (payload.result?.evidence_status === "insufficient") {
        setEvalStatus("Evaluation finished, but telemetry was insufficient.");
      } else if (payload.scope === "snapshot") {
        setEvalStatus(`Snapshot evaluated: ${badge}. Chat is still active.`);
      } else {
        setEvalStatus(`Complete chat evaluated: ${badge}.`);
      }
      await loadDetail({ quiet: true });
    } catch (e: any) {
      setError(String(e.message || e));
    } finally {
      setEvaluating(false);
    }
  }, [id, loadDetail, project]);

  useEffect(() => {
    void loadDetail();
  }, [loadDetail]);

  useSoftPoll(
    useCallback(() => {
      void loadDetail({ quiet: true });
    }, [loadDetail]),
    8000
  );

  return (
    <div className="flex h-full min-h-0 flex-col">
      <FeaturePageHeader
        title={loading ? "Loading…" : String(sessionTitle).slice(0, 96)}
        leading={
          <FeatureBackLink href="/sessions" label="Back to sessions" />
        }
        actions={
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => void runEvaluation()}
              disabled={evaluating || loading}
              aria-label="Evaluate session"
              title="Run evaluation now (bypasses auto cooldown)"
              className="inline-flex h-8 items-center gap-1.5 rounded-md border border-neutral-200 bg-white px-2.5 text-xs font-medium text-neutral-700 hover:bg-neutral-50 disabled:cursor-wait disabled:opacity-60"
            >
              <ClipboardCheck
                className={cn("size-3.5", evaluating && "animate-pulse")}
              />
              {evaluating ? "Evaluating…" : "Evaluate"}
            </button>
            <button
              type="button"
              onClick={() => void refreshDetail()}
              disabled={refreshing}
              aria-label="Refresh session"
              title="Refresh session"
              className="inline-flex h-8 items-center gap-1.5 rounded-md border border-neutral-200 bg-white px-2.5 text-xs font-medium text-neutral-700 hover:bg-neutral-50 disabled:cursor-wait disabled:opacity-60"
            >
              <RefreshCw
                className={cn("size-3.5", refreshing && "animate-spin")}
              />
              Refresh
            </button>
            <div className="flex items-center gap-0.5 rounded-md bg-neutral-100 p-0.5">
              <TabButton active={tab === "chat"} onClick={() => setTab("chat")}>
                Chat
              </TabButton>
              <TabButton active={tab === "map"} onClick={() => setTab("map")}>
                <MapIcon className="mr-1 inline size-3" />
                Map
              </TabButton>
            </div>
          </div>
        }
      />

      {evalStatus && (
        <p className="shrink-0 border-b border-neutral-100 bg-neutral-50 px-4 py-2 text-xs text-neutral-600">
          {evalStatus}
        </p>
      )}

      {loading && tab === "chat" && (
        <p className="p-6 text-sm text-neutral-500">Loading session…</p>
      )}
      {error && tab === "chat" && (
        <p className="p-6 text-sm text-red-600">{error}</p>
      )}

      {!loading && !error && meta && tab === "chat" && (
        <div className="flex min-h-0 flex-1 flex-col">
          {(meta as NestedSession).parent_id && (
            <div className="shrink-0 border-b border-neutral-200 bg-amber-50/60 px-4 py-2 text-xs text-amber-900">
              Nested under{" "}
              <Link
                href={`/sessions/${encodeURIComponent(
                  String((meta as NestedSession).parent_id)
                )}${
                  project ? `?project=${encodeURIComponent(project)}` : ""
                }`}
                className="font-medium underline underline-offset-2 hover:text-amber-950"
              >
                parent session
              </Link>
            </div>
          )}
          {findings.length > 0 && (
            <details className="shrink-0 border-b border-neutral-200 bg-neutral-50 px-4 py-2">
              <summary className="cursor-pointer text-xs font-medium text-neutral-700">
                Review evidence · {findings.length} finding{findings.length === 1 ? "" : "s"}
              </summary>
              <div className="mt-2 grid gap-2 sm:grid-cols-2">
                {findings.map((finding) => (
                  <div key={finding.fingerprint} className="rounded border border-neutral-200 bg-white p-2 text-xs">
                    <div className="flex gap-2 text-[10px] uppercase tracking-wide text-neutral-500">
                      <span>{finding.kind}</span><span>{finding.vendor}</span>
                      {finding.verified && <span>verified</span>}
                    </div>
                    <p className="mt-1 text-neutral-700">{finding.summary}</p>
                  </div>
                ))}
              </div>
            </details>
          )}
          <div className="min-h-0 flex-1">
            <SessionTimeline
              meta={meta}
              spans={spans}
              footprint={footprint}
              restoreCheckpoints={checkpoints}
              subagents={subagents}
              project={project}
            />
          </div>
        </div>
      )}

      {tab === "map" && (
        <div className="relative min-h-0 flex-1 bg-white">
          <MapView
            sessionId={id}
            embed
            className="absolute inset-0"
            sessionExtras={
              meta
                ? {
                    branch: meta.branch,
                    commits: meta.commits,
                    pullRequests: meta.pull_requests,
                    attribution: meta.attribution?.display,
                    checkpoints: checkpoints.map((cp) => ({
                      id: cp.id,
                      label: cp.label,
                      files: cp.files,
                    })),
                  }
                : undefined
            }
          />
        </div>
      )}
    </div>
  );
}

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "rounded px-2.5 py-1 text-xs font-medium transition-colors",
        active
          ? "bg-white text-neutral-900 shadow-sm"
          : "text-neutral-500 hover:text-neutral-800"
      )}
    >
      {children}
    </button>
  );
}
