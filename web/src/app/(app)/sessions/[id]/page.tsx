"use client";

import { useCallback, useEffect, useState, type ReactNode, Suspense } from "react";
import dynamic from "next/dynamic";
import { useParams } from "next/navigation";
import Link from "next/link";
import { Map as MapIcon } from "lucide-react";
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
  const [evalResult, setEvalResult] = useState<Record<string, unknown> | null>(
    null
  );
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);

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
      setEvalResult(
        data.eval && typeof data.eval === "object" ? data.eval : null
      );
      setError("");
    } catch (e: any) {
      setError(String(e.message || e));
    } finally {
      if (!opts?.quiet) setLoading(false);
    }
  }, [id, project]);

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
          <div className="flex items-center gap-0.5 rounded-md bg-neutral-100 p-0.5">
            <TabButton active={tab === "chat"} onClick={() => setTab("chat")}>
              Chat
            </TabButton>
            <TabButton active={tab === "map"} onClick={() => setTab("map")}>
              <MapIcon className="mr-1 inline size-3" />
              Map
            </TabButton>
          </div>
        }
      />

      {tab === "chat" && !loading && !error && evalResult && (
        <div className="shrink-0 border-b border-neutral-100 bg-neutral-50/80 px-4 py-2 text-xs text-neutral-600">
          Eval{" "}
          <span className="font-medium text-neutral-900">
            {String(
              (evalResult as { badge?: string }).badge ||
                (evalResult as { score?: number }).score ||
                "scored"
            )}
          </span>
          {" · "}
          <Link href="/evaluations" className="underline underline-offset-2">
            Evaluations
          </Link>
          {" · replay via Map / "}
          <span className="font-mono">so sessions</span>
        </div>
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
