"use client";

import { Suspense, useCallback, useEffect, useState } from "react";
import dynamic from "next/dynamic";
import { useParams } from "next/navigation";
import { Map as MapIcon, RefreshCw } from "lucide-react";
import SessionTimeline, { type SessionMeta, type Span } from "@/components/session-timeline";
import FeaturePageHeader, { FeatureBackLink } from "@/components/shell/feature-page-header";
import { useBreadcrumbCrumb } from "@/components/shell/breadcrumb-context";
import { useProject } from "@/components/shell/project-context";
import { useFlagQueryParam } from "@/hooks/use-flag-query-param";

const MapView = dynamic(() => import("@/map"), {
  ssr: false,
  loading: () => <div className="grid h-full place-items-center text-sm text-neutral-500">Loading session map…</div>,
});

export default function SessionDetailPage() {
  return (
    <Suspense fallback={<div className="grid h-full place-items-center text-sm text-neutral-500">Loading session…</div>}>
      <SessionDetail />
    </Suspense>
  );
}

function SessionDetail() {
  const { id: encodedID } = useParams<{ id: string }>();
  const id = decodeURIComponent(encodedID);
  const { projectId } = useProject();
  const [mapOn, setMapOn] = useFlagQueryParam("map");
  const [meta, setMeta] = useState<SessionMeta | null>(null);
  const [spans, setSpans] = useState<Span[]>([]);
  const [footprint, setFootprint] = useState<{ files?: { path: string; state: string; count: number }[] } | undefined>();
  const [subagents, setSubagents] = useState<SessionMeta[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);

  const title = meta?.title || meta?.prompt_preview || id;
  useBreadcrumbCrumb(loading ? null : String(title).slice(0, 96));

  const load = useCallback(async (quiet = false) => {
    if (!quiet) setLoading(true);
    try {
      const url = new URL(`/api/sessions/${encodeURIComponent(id)}`, window.location.origin);
      if (projectId) url.searchParams.set("project", projectId);
      const response = await fetch(url.toString());
      if (!response.ok) throw new Error(await response.text());
      const body = await response.json();
      setMeta(body.meta || { id });
      setSpans(Array.isArray(body.transcript) ? body.transcript : []);
      setFootprint(body.footprint || undefined);
      setSubagents(Array.isArray(body.subagents) ? body.subagents : []);
      setError("");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not load session");
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [id, projectId]);

  useEffect(() => {
    void load();
  }, [load]);

  if (loading) return <div className="grid h-full place-items-center text-sm text-neutral-500">Loading session…</div>;

  return (
    <div className="flex h-full min-h-0 flex-col bg-white">
      <FeaturePageHeader
        title={String(title)}
        leading={<FeatureBackLink href="/sessions" label="Back to sessions" />}
        actions={
          <div className="flex gap-2">
            <button
              type="button"
              onClick={() => setMapOn(!mapOn)}
              className="flex items-center gap-1.5 rounded-md border border-neutral-200 px-2.5 py-1.5 text-xs"
            >
              <MapIcon className="size-3.5" />
              {mapOn ? "Timeline" : "Session map"}
            </button>
            <button
              type="button"
              onClick={() => {
                setRefreshing(true);
                void load(true);
              }}
              disabled={refreshing}
              className="flex items-center gap-1.5 rounded-md border border-neutral-200 px-2.5 py-1.5 text-xs disabled:opacity-50"
            >
              <RefreshCw className={`size-3.5 ${refreshing ? "animate-spin" : ""}`} />
              Refresh
            </button>
          </div>
        }
      />
      {error ? (
        <div className="m-6 rounded-lg bg-red-50 p-4 text-sm text-red-700">{error}</div>
      ) : mapOn ? (
        <div className="min-h-0 flex-1">
          <MapView sessionId={id} />
        </div>
      ) : (
        <div className="min-h-0 flex-1 overflow-auto">
          <SessionTimeline
            meta={meta || { id }}
            spans={spans}
            footprint={footprint}
            subagents={subagents}
          />
        </div>
      )}
    </div>
  );
}
