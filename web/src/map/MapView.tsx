"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Crosshair, Sparkles, Users } from "lucide-react";
import {
  describeError,
  getAgentTrace,
  getSessionAgents,
  getSessionReport,
  getSessionSnapshot,
  resolveSessionKey,
  startSessionAnalyze,
} from "./api";
import { PlaybackEngine } from "./playback/reducer";
import { TreeScene } from "./scene/TreeScene";
import { CityScene } from "./scene/CityScene";
import { type PanelBadge, type PanelDescriptor } from "./ui/Dock";
import { AgentsPanel } from "./ui/AgentsPanel";
import { Hud, type ChurnEntry, type HudSessionExtras, type HudTool } from "./ui/Hud";
import { Inspector } from "./ui/Inspector";
import { ReportPanel } from "./ui/ReportPanel";
import { Timeline } from "./ui/Timeline";
import { nearbyFiles } from "./nearby";
import { computeTreeLayout } from "./scene/treeLayout";
import type {
  AgentGraph,
  CityFile,
  CityMap,
  JudgeChoice,
  ReportStatus,
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
  /** Chat-origin linked/checkpoint meta for the shared session rail. */
  sessionExtras?: HudSessionExtras;
}

function evaluateHint(badge: PanelBadge | null): string {
  switch (badge) {
    case "running":
      return "The judge is reading the trace - about a minute";
    case "done":
      return "Evaluation ready";
    case "stale":
      return "Evaluation ready, but the session has grown since";
    case "failed":
      return "The last evaluation failed - open to retry";
    default:
      return "Evaluate this session with your local agent CLI";
  }
}

/**
 * Session map: tree / terrain replay: radial tree / terrain citymap +
 * Session replay on the citymap (timeline + spectrum HUD + dock panels).
 */
export default function MapView({
  sessionId,
  embed = true,
  className,
  sessionExtras,
}: MapViewProps) {
  const [sessionKey, setSessionKey] = useState<string | undefined>();
  const [mainTrace, setMainTrace] = useState<Trace | undefined>();
  const [trace, setTrace] = useState<Trace | undefined>();
  const [city, setCity] = useState<CityMap | undefined>();
  const [currentSeq, setCurrentSeq] = useState(0);
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

  const [reportStatus, setReportStatus] = useState<ReportStatus | undefined>();
  const [analyzing, setAnalyzing] = useState(false);
  const reportPollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      setLoading(true);
      setError(undefined);
      setMainTrace(undefined);
      setTrace(undefined);
      setCity(undefined);
      setCurrentSeq(0);
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
        setCity(snap.city);
        setCurrentSeq(0);
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

  const refreshReport = useCallback(async () => {
    if (!sessionKey) return;
    try {
      const status = await getSessionReport(sessionKey);
      setReportStatus(status);
      setAnalyzing(status.state === "running");
    } catch {
      /* report optional */
    }
  }, [sessionKey]);

  useEffect(() => {
    if (!sessionKey) return;
    void refreshReport();
  }, [sessionKey, refreshReport]);

  useEffect(() => {
    if (reportPollRef.current) {
      clearInterval(reportPollRef.current);
      reportPollRef.current = null;
    }
    if (!sessionKey || reportStatus?.state !== "running") return;
    reportPollRef.current = setInterval(() => {
      void refreshReport();
    }, 4000);
    return () => {
      if (reportPollRef.current) clearInterval(reportPollRef.current);
    };
  }, [sessionKey, reportStatus?.state, refreshReport]);

  const engine = useMemo(() => new PlaybackEngine(trace, city), [trace, city]);
  const playback = useMemo(() => engine.snapshotAt(currentSeq), [engine, currentSeq]);

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
    () => city?.files.find((f) => f.path === selectedPath),
    [city, selectedPath]
  );
  const selectedTouch = selectedPath ? playback.touchByPath.get(selectedPath) : undefined;
  const selectedHistory = selectedPath
    ? playback.historyByPath.get(selectedPath) ?? []
    : [];

  const treeLayout = useMemo(
    () => (city && city.files.length > 0 && view === "tree" ? computeTreeLayout(city.files) : null),
    [city, view]
  );

  // Freeze the neighbor ring when the user picks a file on the map. Prev/Next
  // steps within that ring - recomputing from each new selection would bounce
  // between the two closest leaves forever.
  const [neighborRing, setNeighborRing] = useState<CityFile[]>([]);

  const rebuildNeighborRing = useCallback(
    (path: string) => {
      if (!city) {
        setNeighborRing([]);
        return;
      }
      const origin = city.files.find((f) => f.path === path);
      if (!origin) {
        setNeighborRing([]);
        return;
      }
      setNeighborRing(nearbyFiles(city.files, origin, treeLayout));
    },
    [city, treeLayout]
  );

  const onSelect = useCallback(
    (path?: string) => {
      setSelectedPath(path);
      if (path) {
        rebuildNeighborRing(path);
        setOpenSheet("inspect");
      } else {
        setNeighborRing([]);
      }
    },
    [rebuildNeighborRing]
  );

  const onSelectNeighbor = useCallback((path: string) => {
    // Step inside the frozen ring - do not rebuild from the new leaf.
    setSelectedPath(path);
    setOpenSheet("inspect");
  }, []);

  // Rebuild ring if layout/city changes while a selection is active
  useEffect(() => {
    if (!selectedPath || !city) return;
    setNeighborRing((prev) => {
      if (prev.length === 0) return nearbyFiles(
        city.files,
        city.files.find((f) => f.path === selectedPath) ?? city.files[0],
        treeLayout
      );
      // Keep order; refresh CityFile object refs from current city
      const byPath = new Map(city.files.map((f) => [f.path, f]));
      const next = prev.map((f) => byPath.get(f.path)).filter(Boolean) as CityFile[];
      return next.length >= 2 ? next : prev;
    });
  }, [city, treeLayout]); // eslint-disable-line react-hooks/exhaustive-deps -- only refresh refs on layout/city

  // Keyboard Prev/Next while Inspect is open
  useEffect(() => {
    if (openSheet !== "inspect" || neighborRing.length < 2 || !selectedPath) return;
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
  }, [openSheet, neighborRing, selectedPath, onSelectNeighbor]);

  const onJumpTo = useCallback(
    (seq: number) => {
      setCurrentSeq(seq);
      const event = trace?.events[seq];
      const path = event?.targets[0]?.path;
      if (path) setSelectedPath(path);
    },
    [trace]
  );

  const selectAgent = useCallback(
    async (agentID: string | null) => {
      if (!sessionKey || !mainTrace) return;
      if (agentID === null) {
        setCurrentAgentID(null);
        setTrace(mainTrace);
        setCurrentSeq(0);
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
        setCurrentSeq(0);
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

  const onAnalyze = useCallback(
    async (choice: JudgeChoice) => {
      if (!sessionKey) return;
      setAnalyzing(true);
      try {
        const status = await startSessionAnalyze(sessionKey, choice);
        setReportStatus(status);
      } catch (err) {
        setReportStatus({
          state: "failed",
          stale: false,
          error: describeError(err, "starting evaluation"),
          judgeAvailable: true,
          judgeClis: ["heuristic"],
        });
        setAnalyzing(false);
      }
    },
    [sessionKey]
  );

  const evalBadge: PanelBadge | null = useMemo(() => {
    if (!reportStatus) return null;
    if (reportStatus.state === "running" || analyzing) return "running";
    if (reportStatus.state === "failed") return "failed";
    if (reportStatus.state === "done" && reportStatus.stale) return "stale";
    if (reportStatus.state === "done") return "done";
    return null;
  }, [reportStatus, analyzing]);

  const agentLabel = useMemo(() => {
    if (!currentAgentID || !agentGraph) return undefined;
    return agentGraph.agents.find((a) => a.id === currentAgentID)?.label;
  }, [currentAgentID, agentGraph]);

  const closeSheet = useCallback(() => setOpenSheet(null), []);

  const onToggle = useCallback((panel: PanelDescriptor) => {
    setOpenSheet((cur) => (cur === panel.id ? null : panel.id));
  }, []);

  const panels: PanelDescriptor[] = useMemo(
    () => [
      {
        id: "inspect",
        icon: Crosshair,
        label: "Inspect",
        hint: "Inspect selected file",
        section: "session",
        presentation: "sheet",
        render: () => (
          <Inspector
            file={selectedFile}
            touch={selectedTouch}
            history={selectedHistory}
            neighbors={neighborRing}
            onSelectNeighbor={onSelectNeighbor}
            onClose={closeSheet}
            onJumpTo={onJumpTo}
          />
        ),
      },
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
      {
        id: "evaluate",
        icon: Sparkles,
        label: "Evaluate",
        hint: evaluateHint(evalBadge),
        section: "session",
        presentation: "sheet",
        badge: evalBadge,
        render: () => (
          <ReportPanel
            status={reportStatus}
            analyzing={analyzing}
            locked={false}
            onAnalyze={(choice) => void onAnalyze(choice)}
            onClose={closeSheet}
            onJumpTo={onJumpTo}
          />
        ),
      },
    ],
    [
      selectedFile,
      selectedTouch,
      selectedHistory,
      neighborRing,
      onSelectNeighbor,
      closeSheet,
      onJumpTo,
      agentGraph,
      currentAgentID,
      agentLoading,
      loadingAgentID,
      agentError,
      retryAgentID,
      selectAgent,
      loadAgents,
      evalBadge,
      reportStatus,
      analyzing,
      onAnalyze,
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

  return (
    <div className={`map-root ${className || ""}`.trim()} data-embed={embed ? "1" : undefined}>
      <main className="app-frame rail-collapsed">
        <section className="stage">
          <div className="viewport">
            {view === "tree" ? (
              <TreeScene
                city={city}
                playback={playback}
                selectedPath={selectedPath}
                onSelect={onSelect}
              />
            ) : (
              <CityScene
                city={city}
                playback={playback}
                selectedPath={selectedPath}
                onSelect={onSelect}
              />
            )}

            <Hud
              trace={trace}
              city={city}
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
              onOpenAgents={() => {
                setOpenSheet("agents");
              }}
            />

            {loading && <div className="map-status">Loading map…</div>}
            {error && !loading && (
              <div className="map-status map-status-error">{error}</div>
            )}
          </div>

          <Timeline
            trace={trace}
            currentSeq={currentSeq}
            onChange={setCurrentSeq}
            onSubagentMark={() => {
              setOpenSheet("agents");
            }}
          />
        </section>
      </main>
    </div>
  );
}
