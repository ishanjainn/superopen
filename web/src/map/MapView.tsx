"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { createPortal } from "react-dom";
import { Mountain, TreePine, Users } from "lucide-react";
import {
  describeError,
  getAgentTrace,
  getSessionAgents,
  getSessionSnapshot,
  resolveSessionKey,
} from "./api";
import { PlaybackEngine } from "./playback/reducer";
import { TreeScene } from "./scene/TreeScene";
import { TerrainScene } from "./scene/TerrainScene";
import { type PanelDescriptor } from "./ui/Dock";
import { AgentsPanel } from "./ui/AgentsPanel";
import { Hud, type ChurnEntry, type HudSessionExtras, type HudTool } from "./ui/Hud";
import { Inspector } from "./ui/Inspector";
import { Timeline } from "./ui/Timeline";
import { nearbyFiles } from "./nearby";
import { computeTreeLayout } from "./scene/treeLayout";
import type {
  AgentGraph,
  SessionFile,
  SessionMap,
  Trace,
} from "./types";
import "./map.css";

export type MapViewMode = "tree" | "terrain";

export interface MapViewProps {
  /** Superopen session id or map session key */
  sessionId: string;
  /** Embed in Sessions → Map tab (no full-page chrome). */
  embed?: boolean;
  className?: string;
  /** Chat-origin linked metadata for the shared session rail. */
  sessionExtras?: HudSessionExtras;
  /** Controlled playback index. Uncontrolled when omitted. */
  seq?: number;
  onSeqChange?: (seq: number, at?: number) => void;
  /** Chat-scroll timestamp (unix nano). Map jumps to the nearest event. */
  seekAtNano?: number;
  /** Mount the HUD into a shared top bar instead of the right overlay. */
  hudHost?: HTMLElement | null;
}

/**
 * Session map: tree or terrain replay with timeline, spectrum HUD, and dock panels.
 */
export default function MapView({
  sessionId,
  embed = true,
  className,
  sessionExtras,
  seq,
  onSeqChange,
  seekAtNano,
  hudHost,
}: MapViewProps) {
  const [sessionKey, setSessionKey] = useState<string | undefined>();
  const [mainTrace, setMainTrace] = useState<Trace | undefined>();
  const [trace, setTrace] = useState<Trace | undefined>();
  const [sessionMap, setSessionMap] = useState<SessionMap | undefined>();
  const [internalSeq, setInternalSeq] = useState(0);
  const currentSeq = seq ?? internalSeq;
  const pushSeq = useCallback(
    (next: number) => {
      setInternalSeq(next);
      const ts = trace?.events[next]?.ts;
      const ms = ts ? Date.parse(ts) : NaN;
      const at = Number.isFinite(ms) ? ms * 1e6 : undefined;
      onSeqChange?.(next, at);
    },
    [onSeqChange, trace],
  );
  const [selectedPath, setSelectedPath] = useState<string | undefined>();
  const [view, setView] = useState<MapViewMode>("tree");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | undefined>();

  const [openSheet, setOpenSheet] = useState<string | null>(null);

  const [agentGraph, setAgentGraph] = useState<AgentGraph | undefined>();
  const [agentLoading, setAgentLoading] = useState(false);
  const [agentError, setAgentError] = useState<string | undefined>();
  const [loadingAgentID, setLoadingAgentID] = useState<string | undefined>();
  const [retryAgentID, setRetryAgentID] = useState<string | null>(null);
  const [currentAgentID, setCurrentAgentID] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      setLoading(true);
      setError(undefined);
      setMainTrace(undefined);
      setTrace(undefined);
      setSessionMap(undefined);
      pushSeq(0);
      setSelectedPath(undefined);
      setCurrentAgentID(null);
      setAgentGraph(undefined);
      setOpenSheet(null);
      try {
        const key = await resolveSessionKey(sessionId);
        const snap = await getSessionSnapshot(key);
        if (cancelled) return;
        setSessionKey(key);
        setMainTrace(snap.trace);
        setTrace(snap.trace);
        setSessionMap(snap.sessionMap);
        pushSeq(0);
      } catch (err) {
        if (!cancelled) setError(describeError(err, "loading the map"));
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [sessionId]);

  const loadAgents = useCallback(async () => {
    if (!sessionKey) return;
    setAgentLoading(true);
    setAgentError(undefined);
    setRetryAgentID(null);
    try {
      const graph = await getSessionAgents(sessionKey);
      setAgentGraph(graph);
    } catch (err) {
      setAgentError(describeError(err, "loading agents"));
      setRetryAgentID(null);
    } finally {
      setAgentLoading(false);
    }
  }, [sessionKey]);

  useEffect(() => {
    if (!sessionKey) return;
    void loadAgents();
  }, [sessionKey, loadAgents]);

  const engine = useMemo(() => new PlaybackEngine(trace, sessionMap), [trace, sessionMap]);
  const playback = useMemo(() => engine.snapshotAt(currentSeq), [engine, currentSeq]);

  useEffect(() => {
    if (seekAtNano == null || !trace?.events.length) return;
    let best = 0;
    let bestDelta = Infinity;
    const events = trace.events;
    const hasTs = events.some((event) => event.ts);
    if (!hasTs) {
      const ratio = Math.min(1, Math.max(0, seekAtNano));
      best = Math.round(ratio * (events.length - 1));
    } else {
      for (let i = 0; i < events.length; i++) {
        const stamp = events[i].ts ? Date.parse(events[i].ts as string) * 1e6 : i;
        const delta = Math.abs(stamp - seekAtNano);
        if (delta < bestDelta) {
          best = i;
          bestDelta = delta;
        }
      }
    }
    if (best !== currentSeq) pushSeq(best);
    // currentSeq is read for inequality only; chat-origin seeks should not loop.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [seekAtNano, trace, pushSeq]);

  const touchCounts = useMemo(() => {
    let edited = 0;
    let read = 0;
    let seen = 0;
    for (const touch of playback.touchByPath.values()) {
      if (touch === "edit") edited++;
      else if (touch === "read") read++;
      else seen++;
    }
    return { edited, read, seen };
  }, [playback]);

  const churn = useMemo(() => {
    const counts = new Map<string, number>();
    for (const event of trace?.events ?? []) {
      for (const target of event.targets) {
        if (target.touch === "edit" && target.path) {
          counts.set(target.path, (counts.get(target.path) ?? 0) + 1);
        }
      }
    }
    const rows: ChurnEntry[] = [];
    for (const [path, edits] of counts) {
      if (edits >= 3) rows.push({ path, edits });
    }
    rows.sort((a, b) => b.edits - a.edits || a.path.localeCompare(b.path));
    return rows;
  }, [trace]);

  const selectedFile = useMemo(
    () => sessionMap?.files.find((f) => f.path === selectedPath),
    [sessionMap, selectedPath]
  );
  const selectedTouch = selectedPath ? playback.touchByPath.get(selectedPath) : undefined;
  const selectedHistory = selectedPath
    ? playback.historyByPath.get(selectedPath) ?? []
    : [];

  const treeLayout = useMemo(
    () => (sessionMap && sessionMap.files.length > 0 && view === "tree" ? computeTreeLayout(sessionMap.files) : null),
    [sessionMap, view]
  );

  // Freeze the neighbor ring when the user picks a file on the map. Prev/Next
  // steps within that ring - recomputing from each new selection would bounce
  // between the two closest leaves forever.
  const [neighborRing, setNeighborRing] = useState<SessionFile[]>([]);

  const rebuildNeighborRing = useCallback(
    (path: string) => {
      if (!sessionMap) {
        setNeighborRing([]);
        return;
      }
      const origin = sessionMap.files.find((f) => f.path === path);
      if (!origin) {
        setNeighborRing([]);
        return;
      }
      setNeighborRing(nearbyFiles(sessionMap.files, origin, treeLayout));
    },
    [sessionMap, treeLayout]
  );

  const onSelect = useCallback(
    (path?: string) => {
      setSelectedPath(path);
      if (path) rebuildNeighborRing(path);
      else setNeighborRing([]);
    },
    [rebuildNeighborRing]
  );

  const onSelectNeighbor = useCallback((path: string) => {
    // Step inside the frozen ring - do not rebuild from the new leaf.
    setSelectedPath(path);
  }, []);

  // Closing the card is how you drop the selection - there is no other way
  // back to an unselected stage.
  const closeInspector = useCallback(() => {
    setSelectedPath(undefined);
    setNeighborRing([]);
  }, []);

  // Rebuild ring if layout or session map changes while a selection is active
  useEffect(() => {
    if (!selectedPath || !sessionMap) return;
    setNeighborRing((prev) => {
      if (prev.length === 0) return nearbyFiles(
        sessionMap.files,
        sessionMap.files.find((f) => f.path === selectedPath) ?? sessionMap.files[0],
        treeLayout
      );
      // Keep order; refresh SessionFile object refs from the current session map
      const byPath = new Map(sessionMap.files.map((f) => [f.path, f]));
      const next = prev.map((f) => byPath.get(f.path)).filter(Boolean) as SessionFile[];
      return next.length >= 2 ? next : prev;
    });
  }, [sessionMap, treeLayout]); // eslint-disable-line react-hooks/exhaustive-deps -- only refresh refs on layout/session map

  // Keyboard Prev/Next while a file is selected
  useEffect(() => {
    if (neighborRing.length < 2 || !selectedPath) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.target instanceof HTMLInputElement || event.target instanceof HTMLTextAreaElement) {
        return;
      }
      if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
      event.preventDefault();
      const idx = neighborRing.findIndex((n) => n.path === selectedPath);
      if (idx < 0) return;
      const next =
        event.key === "ArrowRight"
          ? neighborRing[(idx + 1) % neighborRing.length]
          : neighborRing[(idx - 1 + neighborRing.length) % neighborRing.length];
      onSelectNeighbor(next.path);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [neighborRing, selectedPath, onSelectNeighbor]);

  const onJumpTo = useCallback(
    (seq: number) => {
      pushSeq(seq);
      const event = trace?.events[seq];
      const path = event?.targets[0]?.path;
      if (path) setSelectedPath(path);
    },
    [trace, pushSeq]
  );

  const selectAgent = useCallback(
    async (agentID: string | null) => {
      if (!sessionKey || !mainTrace) return;
      if (agentID === null) {
        setCurrentAgentID(null);
        setTrace(mainTrace);
        pushSeq(0);
        setSelectedPath(undefined);
        return;
      }
      setLoadingAgentID(agentID);
      setAgentError(undefined);
      setRetryAgentID(null);
      try {
        const agentTrace = await getAgentTrace(sessionKey, agentID);
        setCurrentAgentID(agentID);
        setTrace(agentTrace);
        pushSeq(0);
        setSelectedPath(undefined);
      } catch (err) {
        setAgentError(describeError(err, "loading agent trace"));
        setRetryAgentID(agentID);
      } finally {
        setLoadingAgentID(undefined);
      }
    },
    [sessionKey, mainTrace]
  );

  const agentLabel = useMemo(() => {
    if (!currentAgentID || !agentGraph) return undefined;
    return agentGraph.agents.find((a) => a.id === currentAgentID)?.label;
  }, [currentAgentID, agentGraph]);

  const closeSheet = useCallback(() => setOpenSheet(null), []);

  const onToggle = useCallback((panel: PanelDescriptor) => {
    setOpenSheet((cur) => (cur === panel.id ? null : panel.id));
  }, []);

  // Inspect is not a dock sheet: it rides the stage as a card over the scene,
  // so the selected file stays in view beside its own facts.
  const panels: PanelDescriptor[] = useMemo(
    () => [
      {
        id: "agents",
        icon: Users,
        label: "Agents",
        hint: "Agent lenses",
        section: "session",
        presentation: "sheet",
        render: () => (
          <AgentsPanel
            graph={agentGraph}
            current={currentAgentID}
            loading={agentLoading}
            loadingAgentID={loadingAgentID}
            error={agentError}
            retryAgentID={retryAgentID}
            onSelect={(id) => void selectAgent(id)}
            onRetry={() => {
              if (retryAgentID) void selectAgent(retryAgentID);
              else void loadAgents();
            }}
            onClose={closeSheet}
          />
        ),
      },
    ],
    [
      closeSheet,
      agentGraph,
      currentAgentID,
      agentLoading,
      loadingAgentID,
      agentError,
      retryAgentID,
      selectAgent,
      loadAgents,
    ]
  );

  const hudTools: HudTool[] = useMemo(
    () =>
      panels.map((panel) => ({
        id: panel.id,
        label: panel.label || panel.id,
        hint: panel.hint,
        icon: panel.icon,
        active: openSheet === panel.id,
        badge: panel.badge ?? null,
        onClick: () => onToggle(panel),
      })),
    [panels, openSheet, onToggle]
  );

  const openPanel = useMemo(() => {
    if (!openSheet) return undefined;
    return panels.find((panel) => panel.id === openSheet)?.render();
  }, [panels, openSheet]);

  const hud = (
    <Hud
      trace={trace}
      sessionMap={sessionMap}
      agentLabel={agentLabel}
      view={view}
      onViewChange={setView}
      tools={hudTools}
      panel={openPanel}
      onClosePanel={closeSheet}
      sessionExtras={sessionExtras}
      editedNow={touchCounts.edited}
      readNow={touchCounts.read}
      seenNow={touchCounts.seen}
      churn={churn}
      onSelectFile={(path) => onSelect(path)}
      agentGraph={agentGraph}
      currentAgentID={currentAgentID}
      agentLoading={agentLoading}
      agentError={agentError}
      onSelectAgent={(id) => void selectAgent(id)}
      placement={hudHost ? "top" : "right"}
    />
  );

  return (
    <div className={`map-root ${className || ""}`.trim()} data-embed={embed ? "1" : undefined}>
      {hudHost ? createPortal(hud, hudHost) : null}
      <main className="app-frame rail-collapsed">
        <section className="stage">
          <div className="viewport">
            {view === "tree" ? (
              <TreeScene
                sessionMap={sessionMap}
                playback={playback}
                selectedPath={selectedPath}
                onSelect={onSelect}
              />
            ) : (
              <TerrainScene
                sessionMap={sessionMap}
                playback={playback}
                selectedPath={selectedPath}
                onSelect={onSelect}
              />
            )}

            <div className="graph-grid" aria-hidden />

            {hudHost ? null : hud}

            <div className="map-view-badge" role="group" aria-label="Scene view">
              <button
                type="button"
                aria-pressed={view === "tree"}
                className={view === "tree" ? "active" : undefined}
                onClick={() => setView("tree")}
                title="Tree"
              >
                <TreePine size={11} strokeWidth={2} />
              </button>
              <button
                type="button"
                aria-pressed={view === "terrain"}
                className={view === "terrain" ? "active" : undefined}
                onClick={() => setView("terrain")}
                title="Terrain"
              >
                <Mountain size={11} strokeWidth={2} />
              </button>
            </div>

            {selectedFile ? (
              <div className="map-inspector">
                <Inspector
                  file={selectedFile}
                  touch={selectedTouch}
                  history={selectedHistory}
                  neighbors={neighborRing}
                  onSelectNeighbor={onSelectNeighbor}
                  onClose={closeInspector}
                  onJumpTo={onJumpTo}
                />
              </div>
            ) : null}

            {loading && <div className="map-status">Loading map…</div>}
            {error && !loading && (
              <div className="map-status map-status-error">{error}</div>
            )}
          </div>

          <Timeline
            trace={trace}
            currentSeq={currentSeq}
            onChange={pushSeq}
            onSubagentMark={() => {
              setOpenSheet("agents");
            }}
          />
        </section>
      </main>
    </div>
  );
}
