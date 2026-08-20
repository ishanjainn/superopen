"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type DragEvent,
  type PointerEvent as ReactPointerEvent,
} from "react";
import Link from "next/link";
import { ChevronsLeftRight, Sparkles } from "lucide-react";
import FeaturePageHeader from "@/components/shell/feature-page-header";
import { useProject } from "@/components/shell/project-context";
import { StellarGraphScene } from "@/graph/StellarGraphScene";
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

export default function MemoryPage() {
  const { projectId } = useProject();
  const [data, setData] = useState<GraphData | null>(null);
  const [status, setStatus] = useState<Status>({});
  const [timeline, setTimeline] = useState<TimelineBucket[]>([]);
  const [selected, setSelected] = useState<Episode | null>(null);
  const [searchResults, setSearchResults] = useState<Episode[]>([]);
  const [query, setQuery] = useState("");
  const [hot, setHot] = useState(false);
  const [busy, setBusy] = useState("");
  const [listPct, setListPct] = useState(28);
  const listPctRef = useRef(28);
  listPctRef.current = listPct;

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
    const tl = (await timeRes.json()) as { buckets?: TimelineBucket[] };
    const buckets = tl.buckets ?? [];
    setTimeline(buckets);
  }, []);

  useEffect(() => {
    const timer = window.setTimeout(() => void reload(), 0);
    return () => window.clearTimeout(timer);
  }, [reload, projectId]);

  const inspect = useCallback(async (id: number) => {
    const res = await fetch(`/api/memory/${id}`);
    if (!res.ok) return;
    setSelected((await res.json()) as Episode);
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
    const url = new URL("/api/memory/search", window.location.origin);
    if (query.trim()) url.searchParams.set("q", query.trim());
    const res = await fetch(url.toString());
    const body = (await res.json()) as { items?: Episode[] };
    setSearchResults(body.items ?? []);
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
  }, []);

  const highlighted = useMemo(() => {
    if (!selected?.id) return null;
    return new Set([selected.id]);
  }, [selected]);

  const allEpisodes = useMemo(
    () => timeline.flatMap((bucket) => bucket.items),
    [timeline],
  );
  const teachings = useMemo(
    () => allEpisodes.filter((item) => item.kind === "teaching"),
    [allEpisodes],
  );
  const sessions = useMemo(
    () => allEpisodes.filter((item) => item.kind === "session"),
    [allEpisodes],
  );

  const empty = (data?.nodes.length ?? 0) === 0;
  const counts = status.counts ?? {};
  const lifecycle = status.lifecycle || (status.distill_paused ? "resting" : "awake");
  const connected = status.connected ?? 0;
  const knowledgePct = status.knowledge_pct ?? 0;
  const cleanedPct = status.cleaned_pct ?? 0;
  const peak = status.activity_peak || 1;

  return (
    <div className="memory-workspace">
      <FeaturePageHeader title="Memory" />

      <div className="memory-vitals">
        <span><b>{n(counts.episodic)}</b> moments</span>
        <span><b>{n(counts.semantic)}</b> knowledge</span>
        <span><b>{n(counts.procedural)}</b> skills</span>
        <span><b>{n(counts.edges)}</b> connections</span>
        <span><b>{n(counts.tombstoned)}</b> forgetting        </span>
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
                  if (!event.target.value.trim()) setSearchResults([]);
                }}
                onKeyDown={(event) => {
                  if (event.key === "Enter") void search();
                }}
              />
              <button type="button" onClick={() => void search()}>Search</button>
            </div>
            <div className="memory-scroll">
              {query.trim() && searchResults.length > 0 ? (
                <section>
                  <h2 className="memory-h">Results</h2>
                  {searchResults.map((item) => (
                    <button
                      key={item.id}
                      type="button"
                      className={selected?.id === item.id ? "memory-item active" : "memory-item"}
                      onClick={() => void inspect(item.id)}
                    >
                      {item.title}
                      <span className="memory-meta">#{item.id} {item.kind}</span>
                    </button>
                  ))}
                </section>
              ) : (
                <section>
                  <h2 className="memory-h">When</h2>
                  {timeline.map((bucket) => (
                    <div key={bucket.when}>
                      <p className="memory-group">{bucket.when} · {bucket.items.length}</p>
                      {bucket.items.map((item) => (
                        <button
                          key={item.id}
                          type="button"
                          className={selected?.id === item.id ? "memory-item active" : "memory-item"}
                          onClick={() => void inspect(item.id)}
                        >
                          {item.title}
                          <span className="memory-meta">#{item.id} {item.kind}{item.fading ? " · fading" : ""}</span>
                        </button>
                      ))}
                    </div>
                  ))}
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

              {sessions.length > 0 ? (
                <section>
                  <h2 className="memory-h">Sessions</h2>
                  {sessions.map((item) => (
                    <button
                      key={item.id}
                      type="button"
                      className={selected?.id === item.id ? "memory-item active" : "memory-item"}
                      onClick={() => void inspect(item.id)}
                    >
                      {item.title}
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

          <div className="memory-stage" style={{ flexGrow: 100 - listPct, flexShrink: 1, flexBasis: 0 }}>
            {empty ? (
              <div className="memory-empty">
                <div>
                  <Sparkles className="mx-auto mb-3 size-6 text-neutral-400" />
                  <p>No memories yet. Finalize a session or drop a teaching.</p>
                </div>
              </div>
            ) : (
              <StellarGraphScene
                className="session-map session-map-night"
                stage="night"
                flat
                data={data!}
                highlightedIds={highlighted}
                focusIds={highlighted}
                showLabels
                display={DEFAULT_GRAPH_DISPLAY}
                onNodeClick={(node: GraphNode) => void inspect(node.id)}
                onBackgroundClick={() => setSelected(null)}
              />
            )}
            <div className="graph-grid" aria-hidden />
            {selected ? (
              <aside className="memory-inspector" aria-label="Selected memory">
                <div className="memory-inspector-head">
                  <strong>{selected.title}</strong>
                  <button type="button" className="memory-inspector-close" onClick={() => setSelected(null)} aria-label="Close">
                    ×
                  </button>
                </div>
                <p className="memory-inspector-meta">
                  #{selected.id} · {selected.kind}
                  {selected.tier && selected.tier !== selected.kind ? ` · ${selected.tier}` : ""}
                  {selected.fading ? " · fading" : ""}
                  {selected.tokens ? ` · ${selected.tokens} tokens` : ""}
                </p>
                <p className="memory-inspector-body">{selected.text || selected.title}</p>
                <div className="memory-inspector-links">
                  {selected.session_id ? (
                    <Link href={`/sessions/${selected.session_id}`}>Open session</Link>
                  ) : null}
                  {(selected.files ?? []).slice(0, 3).map((file) => (
                    <Link key={file} href="/graph" title={file}>
                      {file}
                    </Link>
                  ))}
                </div>
                {selected.tags ? <p className="memory-inspector-tags">{selected.tags}</p> : null}
                <div className="memory-actions">
                  <button type="button" onClick={() => void act("/api/memory/pin", { id: selected.id })}>
                    {selected.pinned ? "Pinned" : "Pin"}
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      void navigator.clipboard.writeText(selected.text || selected.title);
                    }}
                  >
                    Copy
                  </button>
                  {selected.fading || selected.faded ? (
                    <button type="button" onClick={() => void act("/api/memory/rescue", { id: selected.id })}>
                      Cancel fading
                    </button>
                  ) : (
                    <button type="button" onClick={() => void act("/api/memory/fade", { id: selected.id })}>
                      Let it fade
                    </button>
                  )}
                </div>
              </aside>
            ) : null}
          </div>
        </div>

        <aside className="memory-rail">
          <section>
            <h2 className="memory-h">Subconscious</h2>
            <p className="memory-lifecycle">{lifecycle}</p>
            <div className="memory-controls">
              <button type="button" disabled={busy !== ""} onClick={() => void act("/api/memory/distill", { action: "consolidate" })}>
                Sort memories now
              </button>
              <button type="button" disabled={busy !== ""} onClick={() => void act("/api/memory/distill", { action: "sleep" })}>
                Let it rest
              </button>
              <button type="button" disabled={busy !== ""} onClick={() => void act("/api/memory/distill", { action: "resume" })}>
                Wake
              </button>
              <button type="button" disabled={busy !== ""} onClick={() => void act("/api/memory/distill", { action: "restart" })}>
                Restart the subconscious
              </button>
              <button type="button" disabled={busy !== ""} onClick={() => void act("/api/memory/distill", { action: "pause" })}>
                Rest the subconscious
              </button>
            </div>
          </section>

          <section>
            <h2 className="memory-h">Working for you</h2>
            <p className="memory-stat">{n(status.economy?.packs_served)} memory packs served</p>
            <p className="memory-stat">{n(status.economy?.tokens_injected)} free tokens injected</p>
            <p className="memory-stat">{n(status.economy?.fallback_searches)} fallback searches</p>
            <p className="memory-stat">~{n(status.economy?.tokens_saved)} tokens saved (lower bound)</p>
          </section>

          <section>
            <h2 className="memory-h">Health</h2>
            <div className="memory-bar">
              <span>Turned into knowledge</span>
              <b>{knowledgePct.toFixed(1)}%</b>
              <i style={{ width: `${Math.min(100, knowledgePct)}%` }} />
            </div>
            <div className="memory-bar">
              <span>How connected</span>
              <b>{connected.toFixed(2)}x</b>
              <i style={{ width: `${Math.min(100, (connected / 3) * 100)}%` }} />
            </div>
            <div className="memory-bar">
              <span>Cleaned up</span>
              <b>{cleanedPct.toFixed(1)}%</b>
              <i style={{ width: `${Math.min(100, cleanedPct)}%` }} />
            </div>
          </section>

          <section>
            <h2 className="memory-h">Live activity</h2>
            <div className="memory-spark" aria-hidden>
              {(status.activity ?? []).map((bucket) => (
                <span key={bucket.day} style={{ height: `${Math.max(8, (bucket.count / peak) * 100)}%` }} />
              ))}
            </div>
            <p className="memory-stat">Moments {n(counts.episodic)}</p>
            <p className="memory-stat">Knowledge {n(counts.semantic)}</p>
            <p className="memory-stat">Skills {n(counts.procedural)}</p>
            <p className="memory-stat">Pinned {n(counts.pins)}</p>
            <p className="memory-stat">Being forgotten {n(counts.fading)}</p>
            <p className="memory-stat">Working {n(counts.working)}</p>
          </section>
        </aside>
      </div>
    </div>
  );
}
