"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type DragEvent,
  type PointerEvent as ReactPointerEvent,
} from "react";
import { ChevronsLeftRight, Sparkles } from "lucide-react";
import FeaturePageHeader from "@/components/shell/feature-page-header";
import { useProject } from "@/components/shell/project-context";
import { StellarGraphScene } from "@/graph/StellarGraphScene";
import { StageViewport } from "@/components/stage/StageViewport";
import { useLatestRef } from "@/hooks/use-latest-ref";
import { DEFAULT_GRAPH_DISPLAY, type GraphData, type GraphNode } from "@/graph/types";
import "@/map/map.css";
import "@/app/(app)/graph/graph.css";
import "@/app/(app)/sessions/[id]/session-split.css";
import "./memory.css";

type Episode = {
  id: number;
  session_id?: string;
  kind: string;
  title: string;
  text?: string;
  files?: string[];
  tokens?: number;
  pinned?: boolean;
  faded?: boolean;
  fading?: boolean;
  horizon?: string;
  tags?: string;
  tier?: string;
  created_at?: string;
};

type TimelineBucket = { when: string; items: Episode[] };

type Topic = { id: number; label: string; size: number; episode_ids: number[] };

type Status = {
  episodes?: number;
  vectors?: number;
  edges?: number;
  fading?: number;
  coverage?: number;
  live?: number;
  lifecycle?: string;
  knowledge_pct?: number;
  connected?: number;
  cleaned_pct?: number;
  activity?: { day: string; count: number }[];
  activity_peak?: number;
  distill_paused?: boolean;
  pending_distill?: string[];
  topics_detail?: Topic[];
  counts?: {
    episodic?: number;
    semantic?: number;
    procedural?: number;
    working?: number;
    short?: number;
    medium?: number;
    long?: number;
    tombstoned?: number;
    edges?: number;
    pins?: number;
    fading?: number;
  };
  economy?: {
    packs_served?: number;
    tokens_injected?: number;
    tokens_saved?: number;
    fallback_searches?: number;
  };
};

const SPLIT_KEY = "superopen-memory-split-v1";
const SPLIT_MIN = 22;
const SPLIT_MAX = 78;

function loadSplit(): number {
  try {
    const value = Number(localStorage.getItem(SPLIT_KEY));
    if (Number.isFinite(value) && value >= SPLIT_MIN && value <= SPLIT_MAX) {
      return value;
    }
  } catch {
    // optional
  }
  return 28;
}

function n(value: number | undefined): string {
  return (value ?? 0).toLocaleString("en-US");
}

function unwrapEpisode(body: unknown): Episode | null {
  if (Array.isArray(body)) {
    return unwrapEpisode(body[0]);
  }
  if (!body || typeof body !== "object") {
    return null;
  }
  const rec = body as Record<string, unknown>;
  if ("data" in rec && rec.data) {
    return unwrapEpisode(rec.data);
  }
  if (typeof rec.id === "number") {
    return rec as Episode;
  }
  return null;
}

function horizonRank(horizon?: string): number {
  switch (horizon) {
    case "long":
      return 0;
    case "medium":
      return 1;
    case "short":
      return 2;
    default:
      return 3;
  }
}

export default function MemoryPage() {
  const { projectId } = useProject();
  const [data, setData] = useState<GraphData | null>(null);
  const [status, setStatus] = useState<Status>({});
  const [timeline, setTimeline] = useState<TimelineBucket[]>([]);
  const [selected, setSelected] = useState<Episode | null>(null);
  const [inspectError, setInspectError] = useState("");
  const [view, setView] = useState<"knowledge" | "moments" | "skills">("knowledge");
  const [searchResults, setSearchResults] = useState<Episode[]>([]);
  const [query, setQuery] = useState("");
  const [searched, setSearched] = useState(false);
  const [searching, setSearching] = useState(false);
  const [hot, setHot] = useState(false);
  const [busy, setBusy] = useState("");
  const [listPct, setListPct] = useState(28);
  const listPctRef = useLatestRef(listPct);

  useEffect(() => {
    const timer = window.setTimeout(() => setListPct(loadSplit()), 0);
    return () => window.clearTimeout(timer);
  }, []);

  const reload = useCallback(async () => {
    const [layoutRes, statusRes, timeRes] = await Promise.all([
      fetch("/api/memory/layout"),
      fetch("/api/memory/status"),
      fetch("/api/memory/timeline"),
    ]);
    const layout = (await layoutRes.json()) as GraphData;
    layout.nodes ??= [];
    layout.edges ??= [];
    setData(layout);
    setStatus((await statusRes.json()) as Status);
    const tl = (await timeRes.json()) as { buckets?: TimelineBucket[]; items?: Episode[] };
    const buckets = (tl.buckets ?? []).filter((b) => Array.isArray(b.items));
    if (buckets.length > 0) {
      setTimeline(buckets);
    } else if (Array.isArray(tl.items) && tl.items.length > 0) {
      setTimeline([{ when: "recent", items: tl.items }]);
    } else {
      setTimeline([]);
    }
  }, []);

  useEffect(() => {
    const timer = window.setTimeout(() => void reload(), 0);
    return () => window.clearTimeout(timer);
  }, [reload, projectId]);

  const closeInspect = useCallback(() => {
    setSelected(null);
    setInspectError("");
  }, []);

  const inspect = useCallback(async (id: number) => {
    setInspectError("");
    setSelected((prev) => (prev?.id === id ? prev : { id, kind: "", title: "" }));
    const res = await fetch(`/api/memory/${id}`);
    const body = (await res.json()) as unknown;
    if (!res.ok) {
      const rec = body && typeof body === "object" ? (body as { error?: string }) : {};
      setInspectError(String(rec.error || "get failed"));
      return;
    }
    const episode = unwrapEpisode(body);
    if (!episode) {
      setInspectError("memory not found");
      return;
    }
    setSelected(episode);
  }, []);

  const act = useCallback(
    async (path: string, body: Record<string, unknown>) => {
      setBusy(path);
      await fetch(path, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(body),
      });
      setBusy("");
      await reload();
      if (selected?.id) await inspect(selected.id);
    },
    [inspect, reload, selected],
  );

  const onDrop = async (event: DragEvent) => {
    event.preventDefault();
    setHot(false);
    const file = event.dataTransfer.files?.[0];
    if (!file) return;
    const form = new FormData();
    form.set("file", file);
    setBusy("teach");
    await fetch("/api/memory/teach", { method: "POST", body: form });
    setBusy("");
    await reload();
  };

  const search = async () => {
    const q = query.trim();
    if (!q) {
      setSearchResults([]);
      setSearched(false);
      return;
    }
    setSearching(true);
    setSearched(true);
    try {
      const url = new URL("/api/memory/search", window.location.origin);
      url.searchParams.set("q", q);
      const res = await fetch(url.toString());
      const body = (await res.json()) as { items?: Episode[] };
      setSearchResults(body.items ?? []);
    } finally {
      setSearching(false);
    }
  };

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
      listPctRef.current = pct;
      setListPct(pct);
    };
    const onUp = () => {
      gutter.releasePointerCapture(event.pointerId);
      gutter.removeEventListener("pointermove", onMove);
      gutter.removeEventListener("pointerup", onUp);
      try {
        localStorage.setItem(SPLIT_KEY, String(Math.round(listPctRef.current)));
      } catch {
        // optional
      }
    };
    gutter.addEventListener("pointermove", onMove);
    gutter.addEventListener("pointerup", onUp);
  }, [listPctRef]);

  const highlighted = useMemo(() => {
    if (selected?.id) {
      return new Set([selected.id]);
    }
    if (searched && searchResults.length > 0) {
      return new Set(searchResults.map((item) => item.id));
    }
    return null;
  }, [searched, searchResults, selected]);

  const allEpisodes = useMemo(
    () => timeline.flatMap((bucket) => bucket.items),
    [timeline],
  );
  const teachings = useMemo(
    () => allEpisodes.filter((item) => item.kind === "teaching"),
    [allEpisodes],
  );
  const knowledge = useMemo(
    () =>
      allEpisodes
        .filter(
          (item) =>
            item.kind === "session" &&
            !item.faded &&
            (item.horizon == null ||
              item.horizon === "" ||
              item.horizon === "short" ||
              item.horizon === "medium" ||
              item.horizon === "long"),
        )
        .slice()
        .sort((a, b) => horizonRank(a.horizon) - horizonRank(b.horizon)),
    [allEpisodes],
  );
  const moments = useMemo(
    () => allEpisodes.filter((item) => item.kind === "prompt" && !item.faded),
    [allEpisodes],
  );
  const listItems = view === "skills" ? teachings : view === "moments" ? moments : knowledge;

  const empty = (data?.nodes.length ?? 0) === 0;
  const counts = status.counts ?? {};
  const pending = status.pending_distill ?? [];
  const paused = Boolean(status.distill_paused);
  const distillLine = paused ? "paused" : pending.length > 0 ? `${pending.length} pending` : "idle";

  return (
    <div className="memory-workspace">
      <FeaturePageHeader title="Memory" />

      <div className="memory-vitals">
        <span><b>{n(counts.long)}</b> long</span>
        <span><b>{n(counts.medium)}</b> medium</span>
        <span><b>{n(counts.short)}</b> short</span>
        <span><b>{n(counts.working)}</b> diary</span>
        <span><b>{n(counts.tombstoned)}</b> faded</span>
      </div>

      <div className="memory-body-row">
        <div className="memory-split graph-root">
          <aside className="memory-pane memory-left" style={{ flexGrow: listPct, flexShrink: 1, flexBasis: 0 }}>
            <div className="memory-search-row">
              <input
                className="memory-search"
                placeholder="Find a memory"
                value={query}
                onChange={(event) => {
                  setQuery(event.target.value);
                  if (!event.target.value.trim()) {
                    setSearchResults([]);
                    setSearched(false);
                  }
                }}
                onKeyDown={(event) => {
                  if (event.key === "Enter") void search();
                }}
              />
              <button type="button" disabled={searching} onClick={() => void search()}>
                {searching ? "…" : "Search"}
              </button>
            </div>
            <div className="memory-filters">
              <button type="button" className={view === "knowledge" ? "active" : ""} onClick={() => setView("knowledge")}>
                Knowledge
              </button>
              <button type="button" className={view === "skills" ? "active" : ""} onClick={() => setView("skills")}>
                Skills
              </button>
              <button type="button" className={view === "moments" ? "active" : ""} onClick={() => setView("moments")}>
                Moments
              </button>
            </div>
            <div className="memory-scroll">
              {searched ? (
                <section>
                  <h2 className="memory-h">Results · {searchResults.length}</h2>
                  {searchResults.length === 0 ? (
                    <p className="memory-group">0 memories</p>
                  ) : (
                    searchResults.map((item) => (
                      <button
                        key={item.id}
                        type="button"
                        className={selected?.id === item.id ? "memory-item active" : "memory-item"}
                        onClick={() => void inspect(item.id)}
                      >
                        {item.title}
                        <span className="memory-meta">#{item.id} {item.horizon || item.kind}</span>
                      </button>
                    ))
                  )}
                </section>
              ) : (
                <section>
                  <h2 className="memory-h">{view === "skills" ? "Skills" : view === "moments" ? "Moments" : "Knowledge"} · {listItems.length}</h2>
                  {listItems.length === 0 ? (
                    <p className="memory-group">None yet</p>
                  ) : (
                    listItems.map((item) => (
                      <button
                        key={item.id}
                        type="button"
                        className={selected?.id === item.id ? "memory-item active" : "memory-item"}
                        onClick={() => void inspect(item.id)}
                      >
                        {item.title}
                        <span className="memory-chip">{item.horizon || item.kind}</span>
                        <span className="memory-meta">#{item.id}{item.fading ? " · fading" : ""}{item.faded ? " · faded" : ""}</span>
                      </button>
                    ))
                  )}
                </section>
              )}

              {(status.topics_detail?.length ?? 0) > 0 ? (
                <section>
                  <h2 className="memory-h">Topics</h2>
                  {(status.topics_detail ?? []).map((topic) => (
                    <button
                      key={topic.id}
                      type="button"
                      className="memory-item"
                      onClick={() => {
                        const id = topic.episode_ids?.[0];
                        if (id) void inspect(id);
                      }}
                    >
                      {topic.label}
                      <span className="memory-meta">{topic.size} memories</span>
                    </button>
                  ))}
                </section>
              ) : null}

              <section>
                <h2 className="memory-h">Teachings</h2>
                <div
                  className={hot ? "memory-drop hot" : "memory-drop"}
                  onDragOver={(event) => {
                    event.preventDefault();
                    setHot(true);
                  }}
                  onDragLeave={() => setHot(false)}
                  onDrop={(event) => void onDrop(event)}
                >
                  Drop txt / md / csv / pdf
                </div>
                {teachings.map((item) => (
                  <button
                    key={item.id}
                    type="button"
                    className="memory-item"
                    onClick={() => void inspect(item.id)}
                  >
                    {item.title}
                  </button>
                ))}
              </section>
            </div>
          </aside>

          <div
            className="session-split-gutter"
            role="separator"
            aria-orientation="vertical"
            aria-label="Resize list and graph"
            onPointerDown={onSplitPointer}
          >
            <span className="session-split-gutter-handle" aria-hidden>
              <ChevronsLeftRight className="size-3.5" />
            </span>
          </div>

          <StageViewport className="memory-stage" style={{ flexGrow: 100 - listPct, flexShrink: 1, flexBasis: 0 }}>
            {empty ? (
              <div className="memory-empty">
                <div>
                  <Sparkles className="mx-auto mb-3 size-6 text-neutral-400" />
                  <p>No memories yet. Finalize a session or drop a teaching.</p>
                </div>
              </div>
            ) : (
              <StellarGraphScene
                className="session-map"
                data={data!}
                highlightedIds={highlighted}
                focusIds={highlighted}
                showLabels
                display={DEFAULT_GRAPH_DISPLAY}
                onNodeClick={(node: GraphNode) => void inspect(node.id)}
                onBackgroundClick={closeInspect}
              />
            )}
            {selected || inspectError ? (
              <aside className="memory-inspector">
                <div className="memory-inspector-head">
                  <strong>{selected?.title || (selected?.id ? `#${selected.id}` : "Memory")}</strong>
                  <button type="button" className="memory-inspector-close" aria-label="Close" onClick={closeInspect}>
                    ×
                  </button>
                </div>
                {selected ? (
                  <p className="memory-inspector-meta">
                    #{selected.id}
                    {selected.horizon || selected.kind ? ` · ${selected.horizon || selected.kind}` : ""}
                    {selected.fading ? " · fading" : ""}
                    {selected.faded ? " · faded" : ""}
                  </p>
                ) : null}
                {selected?.horizon ? <span className="memory-chip">{selected.horizon}</span> : null}
                {inspectError ? <p className="memory-error">{inspectError}</p> : null}
                {selected?.text ? <p className="memory-inspector-body">{selected.text}</p> : null}
                {selected?.id && !inspectError ? (
                  <div className="memory-actions">
                    <button type="button" onClick={() => void act("/api/memory/pin", { id: selected.id })}>
                      {selected.pinned ? "Pinned" : "Pin"}
                    </button>
                    {selected.fading || selected.faded ? (
                      <button type="button" onClick={() => void act("/api/memory/rescue", { id: selected.id })}>
                        Restore
                      </button>
                    ) : (
                      <button type="button" onClick={() => void act("/api/memory/fade", { id: selected.id })}>
                        Forget
                      </button>
                    )}
                  </div>
                ) : null}
              </aside>
            ) : null}
          </StageViewport>
        </div>

        <aside className="memory-rail">
          <section>
            <h2 className="memory-h">Distill</h2>
            <p className="memory-lifecycle">{distillLine}</p>
            <div className="memory-controls">
              {pending.length > 0 ? (
                <button type="button" disabled={busy !== ""} onClick={() => void act("/api/memory/distill", { action: "consolidate" })}>
                  Retry pending
                </button>
              ) : null}
              <button
                type="button"
                disabled={busy !== ""}
                onClick={() => void act("/api/memory/distill", { action: paused ? "resume" : "pause" })}
              >
                {paused ? "Resume auto-distill" : "Pause auto-distill"}
              </button>
            </div>
          </section>
        </aside>
      </div>
    </div>
  );
}
