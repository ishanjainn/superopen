"use client";

import { Suspense, useCallback, useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from "react";
import dynamic from "next/dynamic";
import { useParams, useSearchParams } from "next/navigation";
import { ChevronsLeftRight, RefreshCw } from "lucide-react";
import SessionTimeline, { type SessionMeta, type Span } from "@/components/session-timeline";
import FeaturePageHeader, { FeatureBackLink } from "@/components/shell/feature-page-header";
import { useBreadcrumbCrumb } from "@/components/shell/breadcrumb-context";
import { useProject } from "@/components/shell/project-context";
import "./session-split.css";

const MapView = dynamic(() => import("@/map"), {
  ssr: false,
  loading: () => (
    <div className="grid h-full place-items-center text-sm text-neutral-500">
      Loading session map…
    </div>
  ),
});

const SPLIT_KEY = "superopen-session-split-v3";
const SPLIT_MIN = 22;
const SPLIT_MAX = 78;

export default function SessionDetailPage() {
  return (
    <Suspense
      fallback={
        <div className="grid h-full place-items-center text-sm text-neutral-500">
          Loading session…
        </div>
      }
    >
      <SessionDetail />
    </Suspense>
  );
}

function loadSplit(): number {
  try {
    const value = Number(localStorage.getItem(SPLIT_KEY));
    if (Number.isFinite(value) && value >= SPLIT_MIN && value <= SPLIT_MAX) {
      return value;
    }
  } catch {
    // Storage is optional.
  }
  return 50;
}

function SessionDetail() {
  const { id: encodedID } = useParams<{ id: string }>();
  const id = decodeURIComponent(encodedID);
  const { projectId, setProjectId } = useProject();

  // The sessions list links to a session with the project that owns it. Adopt
  // it, or a link into another repo loads whatever the selector last held.
  const linkedProject = useSearchParams().get("project") || "";
  useEffect(() => {
    if (linkedProject && linkedProject !== projectId) setProjectId(linkedProject);
  }, [linkedProject, projectId, setProjectId]);
  const [meta, setMeta] = useState<SessionMeta | null>(null);
  const [spans, setSpans] = useState<Span[]>([]);
  const [footprint, setFootprint] = useState<
    { files?: { path: string; state: string; count: number }[] } | undefined
  >();
  const [subagents, setSubagents] = useState<SessionMeta[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [seq, setSeq] = useState(0);
  const [mapSeekNano, setMapSeekNano] = useState<number | undefined>();
  const [chatSeek, setChatSeek] = useState<{ at: number; nonce: number } | undefined>();
  const [chatPct, setChatPct] = useState(50);
  const [hudHost, setHudHost] = useState<HTMLElement | null>(null);
  const syncSource = useRef<"idle" | "chat" | "map">("idle");
  const chatSeekNonce = useRef(0);

  const title = meta?.title || meta?.prompt_preview || id;
  useBreadcrumbCrumb(loading ? null : String(title).slice(0, 96));

  useEffect(() => {
    const timer = window.setTimeout(() => setChatPct(loadSplit()), 0);
    return () => window.clearTimeout(timer);
  }, []);

  const load = useCallback(
    async (quiet = false) => {
      if (!quiet) setLoading(true);
      try {
        const url = new URL(
          `/api/sessions/${encodeURIComponent(id)}`,
          window.location.origin,
        );
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
    },
    [id, projectId],
  );

  useEffect(() => {
    void load();
  }, [load]);

  const onVisibleAt = useCallback((at: number) => {
    if (syncSource.current === "map") {
      syncSource.current = "idle";
      return;
    }
    syncSource.current = "chat";
    setMapSeekNano(at);
    window.setTimeout(() => {
      if (syncSource.current === "chat") syncSource.current = "idle";
    }, 80);
  }, []);

  const onSeqChange = useCallback((next: number, at?: number) => {
    setSeq(next);
    if (syncSource.current === "chat") {
      syncSource.current = "idle";
      return;
    }
    if (at == null) return;
    syncSource.current = "map";
    chatSeekNonce.current += 1;
    setChatSeek({ at, nonce: chatSeekNonce.current });
    window.setTimeout(() => {
      if (syncSource.current === "map") syncSource.current = "idle";
    }, 80);
  }, []);

  const chatPctRef = useRef(50);
  chatPctRef.current = chatPct;

  const onSplitPointer = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    const gutter = event.currentTarget;
    const split = gutter.parentElement;
    if (!split) return;
    gutter.setPointerCapture(event.pointerId);
    const onMove = (move: globalThis.PointerEvent) => {
      const box = split.getBoundingClientRect();
      if (box.width <= 0) return;
      const pct = Math.min(
        SPLIT_MAX,
        Math.max(SPLIT_MIN, ((move.clientX - box.left) / box.width) * 100),
      );
      chatPctRef.current = pct;
      setChatPct(pct);
    };
    const onUp = () => {
      gutter.releasePointerCapture(event.pointerId);
      gutter.removeEventListener("pointermove", onMove);
      gutter.removeEventListener("pointerup", onUp);
      try {
        localStorage.setItem(SPLIT_KEY, String(Math.round(chatPctRef.current)));
      } catch {
        // Storage is optional.
      }
    };
    gutter.addEventListener("pointermove", onMove);
    gutter.addEventListener("pointerup", onUp);
  }, []);

  if (loading) {
    return (
      <div className="grid h-full place-items-center text-sm text-neutral-500">
        Loading session…
      </div>
    );
  }

  return (
    <div className="session-workspace">
      <FeaturePageHeader
        title={String(title)}
        leading={<FeatureBackLink href="/sessions" label="Back to sessions" />}
        meta={
          <span title={id}>{id.length > 12 ? `${id.slice(0, 8)}…` : id}</span>
        }
        actions={
          <button
            type="button"
            onClick={() => {
              setRefreshing(true);
              void load(true);
            }}
            disabled={refreshing}
            aria-label="Refresh session"
            title="Refresh session"
            className="inline-flex size-7 items-center justify-center rounded-md border border-neutral-200 text-neutral-600 hover:bg-neutral-50 hover:text-neutral-900 disabled:opacity-50"
          >
            <RefreshCw className={`size-3.5 ${refreshing ? "animate-spin" : ""}`} />
          </button>
        }
      />
      {error ? (
        <div className="m-6 rounded-lg bg-red-50 p-4 text-sm text-red-700">{error}</div>
      ) : (
        <>
          <div
            ref={setHudHost}
            className="session-chrome"
            aria-label="Session chrome"
          />
          <div className="session-split">
            <div
              className="session-split-chat"
              style={{ flexGrow: chatPct, flexShrink: 1, flexBasis: 0 }}
            >
              <SessionTimeline
                meta={meta || { id }}
                spans={spans}
                footprint={footprint}
                subagents={subagents}
                showRail={false}
                onVisibleAt={onVisibleAt}
                seekAt={chatSeek?.at}
                seekNonce={chatSeek?.nonce}
              />
            </div>
            <div
              className="session-split-gutter"
              role="separator"
              aria-orientation="vertical"
              aria-label="Resize chat and map"
              onPointerDown={onSplitPointer}
            >
              <span className="session-split-gutter-handle" aria-hidden>
                <ChevronsLeftRight className="size-3.5" />
              </span>
            </div>
            <div
              className="session-split-map"
              style={{ flexGrow: 100 - chatPct, flexShrink: 1, flexBasis: 0 }}
            >
              <MapView
                sessionId={id}
                seq={seq}
                onSeqChange={onSeqChange}
                seekAtNano={mapSeekNano}
                hudHost={hudHost}
                sessionExtras={{
                  branch: meta?.branch,
                  commits: meta?.commits,
                  pullRequests: meta?.pull_requests,
                  attribution: meta?.attribution?.display,
                  tokens: meta?.tokens,
                  costUsd: meta?.cost_usd,
                }}
              />
            </div>
          </div>
        </>
      )}
    </div>
  );
}
