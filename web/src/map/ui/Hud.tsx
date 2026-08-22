import { memo, useEffect, useRef, useState, type ReactNode } from "react";
import { ChevronDown, Mountain, TreePine, Users, type LucideIcon } from "lucide-react";
import {
  SessionRail,
  type SessionRailTool,
} from "@/components/session-rail";
import type { ActionCounts, AgentGraph, MetricObservability, SessionMap, Trace } from "../types";

export interface ChurnEntry {
  path: string;
  edits: number;
}

export type MapSceneView = "tree" | "terrain";

export interface HudTool {
  id: string;
  label: string;
  hint: string;
  icon: LucideIcon;
  active?: boolean;
  badge?: "running" | "done" | "stale" | "failed" | null;
  onClick: () => void;
}

/** Optional Chat-origin session meta shown in the shared rail. */
export interface HudSessionExtras {
  branch?: string;
  commits?: { sha?: string; message?: string }[];
  pullRequests?: { url?: string; number?: number; title?: string }[];
  attribution?: string;
  tokens?: number;
  costUsd?: number;
}

interface HudProps {
  trace?: Trace;
  sessionMap?: SessionMap;
  agentLabel?: string;
  view: MapSceneView;
  onViewChange: (view: MapSceneView) => void;
  tools?: HudTool[];
  panel?: ReactNode;
  onClosePanel?: () => void;
  sessionExtras?: HudSessionExtras;
  editedNow: number;
  readNow: number;
  seenNow: number;
  churn: ChurnEntry[];
  onSelectFile: (path: string) => void;
  agentGraph?: AgentGraph;
  currentAgentID?: string | null;
  agentLoading?: boolean;
  agentError?: string;
  onSelectAgent?: (agentID: string | null) => void;
  locked?: boolean;
  placement?: "right" | "top";
}

const CHURN_PANEL_ROWS = 8;

export const Hud = memo(function Hud({
  trace,
  sessionMap,
  agentLabel,
  view,
  onViewChange,
  tools = [],
  panel,
  onClosePanel,
  sessionExtras,
  editedNow,
  readNow,
  seenNow,
  churn,
  onSelectFile,
  agentGraph,
  currentAgentID = null,
  agentLoading = false,
  agentError,
  onSelectAgent,
  locked = false,
  placement = "right",
}: HudProps) {
  const stats = trace?.stats;
  const readFinal = stats ? stats.fovea - stats.edited : 0;
  const unvisitedNow = stats
    ? Math.max(0, stats.filesInRepo - editedNow - readNow - seenNow)
    : 0;
  const unvisitedFinal = stats
    ? Math.max(0, stats.filesInRepo - stats.fovea - stats.parafovea)
    : 0;
  const ghostCount = sessionMap
    ? sessionMap.files.reduce((n, file) => n + (file.ghost ? 1 : 0), 0)
    : 0;
  const errorCount = stats ? countActions(stats.errors) : 0;
  const showReview = stats
    ? errorCount > 0 || stats.churnFiles > 0 || stats.actions.edit > 0
    : false;
  const panelOpen = Boolean(panel);

  const [churnOpen, setChurnOpen] = useState(false);
  const [agentsOpen, setAgentsOpen] = useState(false);
  const [traceSeen, setTraceSeen] = useState(trace);
  if (trace !== traceSeen) {
    setTraceSeen(trace);
    setChurnOpen(false);
  }
  if (panelOpen && (churnOpen || agentsOpen)) {
    setChurnOpen(false);
    setAgentsOpen(false);
  }
  const churnPanelRef = useRef<HTMLDivElement | null>(null);
  const churnToggleRef = useRef<HTMLButtonElement | null>(null);
  const agentsRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!churnOpen) return;
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Node;
      if (
        churnPanelRef.current?.contains(target) ||
        churnToggleRef.current?.contains(target)
      )
        return;
      setChurnOpen(false);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setChurnOpen(false);
    };
    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [churnOpen]);

  useEffect(() => {
    if (!agentsOpen) return;
    const onPointerDown = (event: PointerEvent) => {
      if (agentsRef.current?.contains(event.target as Node)) return;
      setAgentsOpen(false);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setAgentsOpen(false);
    };
    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [agentsOpen]);

  if (!sessionMap) {
    return <div className="hud" aria-hidden />;
  }

  const railTools: SessionRailTool[] = tools.map((tool) => ({
    id: tool.id,
    label: tool.label,
    hint: tool.hint,
    icon: tool.icon,
    active: tool.active,
    badge: tool.badge,
    onClick: tool.onClick,
  }));

  // A session that spans repos gets a district per root; naming them is more
  // use than a revision line that only ever reads "worktree".
  const roots = sessionMap.roots?.length
    ? sessionMap.roots
    : [
        {
          prefix: "",
          name: sessionMap.repo.root.split("/").pop() || "repo",
          path: sessionMap.repo.root,
          git: true,
          files: sessionMap.files.length,
        },
      ];
  const rootsHint = roots
    .map(
      (root) =>
        `${root.name}${root.git ? "" : " (plain directory, not a checkout)"} - ${root.files} files - ${root.path}`,
    )
    .join("\n");

  const commits = sessionExtras?.commits || [];
  const prs = sessionExtras?.pullRequests || [];
  const showLinked =
    Boolean(sessionExtras?.branch) ||
    Boolean(sessionExtras?.attribution) ||
    commits.length > 0 ||
    prs.length > 0;

  return (
    <div className={placement === "top" ? "hud hud-top" : "hud"} aria-hidden={false}>
      <SessionRail
        bare
        layout={placement === "top" ? "horizontal" : "vertical"}
        aria-label="Session map title block"
        locked={locked}
        tools={placement === "top" ? [] : railTools}
        panel={panel}
        onClosePanel={onClosePanel}
        viewToggle={
          placement === "top" ? undefined : (
            <div className="tb-seg" role="group" aria-label="Scene view">
              <button
                type="button"
                aria-pressed={view === "tree"}
                className={view === "tree" ? "active" : undefined}
                onClick={() => onViewChange("tree")}
                disabled={locked}
              >
                <TreePine size={15} strokeWidth={1.75} />
                Tree
              </button>
              <button
                type="button"
                aria-pressed={view === "terrain"}
                className={view === "terrain" ? "active" : undefined}
                onClick={() => onViewChange("terrain")}
                disabled={locked}
              >
                <Mountain size={15} strokeWidth={1.75} />
                Terrain
              </button>
            </div>
          )
        }
      >
        <div className="tb-band">
          <div className="tb-cell tb-shrink tb-roots" data-hint={rootsHint}>
            <span className="tb-label">
              {roots.length > 1 ? "Repos" : "Repo"}
            </span>
            <span className="tb-value tb-mono tb-roots-value">
              <span
                className={roots[0].git ? "tb-root" : "tb-root tb-root-dir"}
              >
                {roots[0].name}
              </span>
              {roots.length > 1 ? (
                <span className="tb-root-more">+{roots.length - 1}</span>
              ) : null}
            </span>
          </div>

          {showLinked ? (
            <div className="tb-cell tb-shrink">
              <span className="tb-label">Linked</span>
              <span className="tb-value tb-mono tb-inline">
                {sessionExtras?.branch ? (
                  <span>branch {sessionExtras.branch}</span>
                ) : null}
                {sessionExtras?.attribution ? (
                  <span>{sessionExtras.attribution}</span>
                ) : null}
                {commits.slice(0, 3).map((c) => (
                  <span key={c.sha || c.message}>
                    {(c.sha || "").slice(0, 7)}
                    {c.message ? ` · ${c.message.slice(0, 28)}` : ""}
                  </span>
                ))}
                {prs.slice(0, 2).map((pr) => (
                  <span key={pr.url || String(pr.number)}>
                    {pr.title || `PR #${pr.number}`}
                  </span>
                ))}
              </span>
            </div>
          ) : null}

          <div className="tb-cell tb-shrink">
            <span className="tb-label">Model</span>
            <span className="tb-value tb-mono">
              {trace?.session.model || "-"}
            </span>
          </div>

          <div className="tb-cell tb-shrink tb-agents" ref={agentsRef}>
            <span className="tb-label">Agents</span>
            <button
              type="button"
              className="tb-agents-btn"
              aria-expanded={agentsOpen}
              aria-haspopup="listbox"
              disabled={!onSelectAgent || locked}
              title="Agent lenses"
              onClick={() => setAgentsOpen((value) => !value)}
            >
              <Users size={13} strokeWidth={2} />
              <span>{agentLabel || "Main"}</span>
              {stats && stats.subagents > 0 ? (
                <span className="tb-faint">{stats.subagents}</span>
              ) : null}
              <ChevronDown size={12} strokeWidth={2} />
            </button>
            {agentsOpen ? (
              <div className="tb-agents-pop" role="listbox" aria-label="Agent lenses">
                  <p className="tb-agents-label">Lenses</p>
                  {agentError ? (
                    <p className="tb-agents-empty">{agentError}</p>
                  ) : null}
                  {agentLoading && !agentGraph ? (
                    <p className="tb-agents-empty">Loading agents…</p>
                  ) : (
                    <ul className="tb-jump-list">
                      <li>
                        <button
                          type="button"
                          className={currentAgentID == null ? "tb-jump-row active" : "tb-jump-row"}
                          aria-pressed={currentAgentID == null}
                          onClick={() => {
                            onSelectAgent?.(null);
                            setAgentsOpen(false);
                          }}
                        >
                          <span className="tb-jump-row-text">Main</span>
                          <span className="tb-filter-count">
                            {agentGraph?.agents.find((agent) => agent.kind === "main")?.traceEventCount ??
                              0}
                          </span>
                        </button>
                      </li>
                      {(agentGraph?.agents.filter((agent) => agent.kind !== "main") ?? []).map((agent) => (
                        <li key={agent.id}>
                          <button
                            type="button"
                            className={
                              currentAgentID === agent.id ? "tb-jump-row active" : "tb-jump-row"
                            }
                            aria-pressed={currentAgentID === agent.id}
                            disabled={agent.traceAvailability !== "available" || locked}
                            onClick={() => {
                              onSelectAgent?.(agent.id);
                              setAgentsOpen(false);
                            }}
                          >
                            <span className="tb-jump-row-text">{agent.label}</span>
                            <span className="tb-filter-count">{agent.traceEventCount}</span>
                          </button>
                        </li>
                      ))}
                    </ul>
                  )}
              </div>
            ) : null}
          </div>

          <div className="tb-cell tb-shrink">
            <span className="tb-label">Tokens</span>
            <span className="tb-value tb-mono">
              {fmtTokens(sessionExtras?.tokens)}
            </span>
          </div>

          <div className="tb-cell tb-shrink">
            <span className="tb-label">Cost</span>
            <span className="tb-value tb-mono">
              {fmtCost(sessionExtras?.costUsd)}
            </span>
          </div>

          {stats ? (
            <>
              <div className="tb-cell tb-spectrum tb-grow">
                <span className="tb-label">Coverage</span>
                <div className="tb-spectrum-row">
                  <TitleStat
                    kind="edit"
                    label="Edited"
                    now={editedNow}
                    final={stats.edited}
                    hint="Files the agent changed"
                  />
                  <TitleStat
                    kind="read"
                    label="Read"
                    now={readNow}
                    final={readFinal}
                    hint="Files the agent opened and read, but never changed"
                  />
                  <TitleStat
                    kind="hit"
                    label="Seen"
                    now={seenNow}
                    final={stats.parafovea}
                    hint="Files that only appeared in search results, never opened"
                  />
                  <TitleStat
                    kind="unvisited"
                    label="Unvisited"
                    now={unvisitedNow}
                    final={unvisitedFinal}
                    hint="Files in the map the agent never touched"
                  />
                  {ghostCount > 0 ? (
                    <TitleStat
                      kind="ghost"
                      label="Ghost"
                      now={ghostCount}
                      final={ghostCount}
                      hint="Files the session touched that are gone from the repository"
                    />
                  ) : null}
                </div>
              </div>

              <div className="tb-cell tb-grow">
                <span className="tb-label">Activity</span>
                <span className="tb-value tb-mono tb-inline">
                  <span data-hint={`Tool calls - ${mixHint(stats.actions)}`}>
                    {countActions(stats.actions)} calls
                  </span>
                  <span data-hint="User messages - each one starts a turn of agent work">
                    {stats.userTurns} turns
                  </span>
                  {stats.subagents > 0 ? (
                    <button
                      className="tb-link"
                      data-hint="Subagent launches (Task/Agent) - open Agent lenses"
                      onClick={() => setAgentsOpen(true)}
                      disabled={!onSelectAgent || locked}
                      aria-label={`Open ${stats.subagents} subagent${stats.subagents === 1 ? "" : "s"} in Agent lenses`}
                    >
                      {stats.subagents} subagent
                      {stats.subagents === 1 ? "" : "s"}
                    </button>
                  ) : null}
                  {stats.compactions > 0 ? (
                    <span data-hint="Context compactions - the conversation was summarized to free memory">
                      {stats.compactions} compaction
                      {stats.compactions === 1 ? "" : "s"}
                    </span>
                  ) : null}
                  <span data-hint="Tool output the agent consumed over the session">
                    {fmtBytes(stats.resultBytes)} out
                  </span>
                  <span data-hint={rereadHint(stats.observability.reads)}>
                    {stats.observability.reads === "unavailable"
                      ? "re-reads n/a"
                      : `re-reads ${approx(stats.observability.reads)}${pct(stats.regressionRate)}`}
                  </span>
                </span>
              </div>

              <div
                className="tb-cell tb-shrink"
                data-hint="Files in the repository map - the denominator of the coverage spectrum"
              >
                <span className="tb-label">Files</span>
                <span className="tb-value tb-mono">
                  {stats.filesInRepo}
                  {sessionMap.repo.truncated ? (
                    <span
                      className="tb-partial"
                      data-hint="The tree holds more files than the map shows - scanning stopped at the budget"
                    >
                      {" "}
                      · partial
                    </span>
                  ) : null}
                </span>
              </div>

              {placement !== "top" ? (
                <div className="tb-cell tb-shrink">
                  <span className="tb-label">Session</span>
                  <span className="tb-value tb-mono tb-session-id">
                    {shortId(trace?.session.id)}
                  </span>
                </div>
              ) : null}

              {showReview ? (
                <div className="tb-cell tb-shrink">
                  <span className="tb-label">Review</span>
                  <span className="tb-value tb-review-bits">
                {errorCount > 0 ? (
                  <span
                    className="warn"
                    data-hint={`${mixHint(stats.errors)} - press X to jump to the next one${errorCaveat(stats.observability.errors)}`}
                  >
                    {approx(stats.observability.errors)}
                    {errorCount} error{errorCount === 1 ? "" : "s"}
                  </span>
                ) : null}
                {stats.churnFiles > 0 ? (
                  <button
                    ref={churnToggleRef}
                    className={
                      churnOpen ? "warn churn-toggle open" : "warn churn-toggle"
                    }
                    aria-expanded={churnOpen}
                    onClick={() => setChurnOpen((open) => !open)}
                    data-hint={`Files edited in three or more separate events - the most-edited file changed ${stats.maxEditsPerFile} times. Click to list them.`}
                  >
                    {stats.churnFiles} file
                    {stats.churnFiles === 1 ? "" : "s"} edited 3+ times
                  </button>
                ) : null}
                {stats.actions.edit > 0 ? (
                  stats.actions.verify === 0 ? (
                    <span
                      className="warn"
                      data-hint="The session edited files but no build or test commands were recognized"
                    >
                      never verified
                    </span>
                  ) : stats.editsAfterLastVerify > 0 ? (
                    <span
                      className="warn"
                      data-hint={`Edit events after the session's last build or test run - ${verifyRuns(stats.actions.verify)} total`}
                    >
                      {stats.editsAfterLastVerify} edit
                      {stats.editsAfterLastVerify === 1 ? "" : "s"} after last
                      verify
                    </span>
                  ) : (
                    <span
                      className="ok"
                      data-hint={`The last edit was followed by a build or test run - ${verifyRuns(stats.actions.verify)} total`}
                    >
                      verified after final edit
                    </span>
                  )
                ) : null}
                  </span>
                </div>
              ) : null}
            </>
          ) : null}
        </div>

        {churnOpen ? (
          <div className="churn-panel" ref={churnPanelRef}>
            {churn.slice(0, CHURN_PANEL_ROWS).map((entry) => (
              <button
                key={entry.path}
                className="churn-row"
                onClick={() => {
                  onSelectFile(entry.path);
                  setChurnOpen(false);
                }}
              >
                <span className="churn-path">{entry.path}</span>
                <span className="churn-count">
                  {entry.edits} edit{entry.edits === 1 ? "" : "s"}
                </span>
              </button>
            ))}
            {churn.length > CHURN_PANEL_ROWS ? (
              <p className="churn-more">
                …and {churn.length - CHURN_PANEL_ROWS} more
              </p>
            ) : null}
          </div>
        ) : null}
      </SessionRail>
    </div>
  );
});

function TitleStat({
  kind,
  label,
  now,
  final,
  hint,
}: {
  kind: "edit" | "read" | "hit" | "unvisited" | "ghost";
  label: string;
  now: number;
  final: number;
  hint: string;
}) {
  return (
    <div className="tb-stat" data-hint={hint}>
      <span className={`legend-dot ${kind}`} />
      <span className="tb-stat-label">{label}</span>
      <strong className="tb-mono">
        {now === final ? final : `${now} → ${final}`}
      </strong>
    </div>
  );
}

function pct(rate: number): string {
  return `${Math.round(rate * 100)}%`;
}

function approx(observability: MetricObservability): string {
  return observability === "estimated" ? "~" : "";
}

function verifyRuns(count: number): string {
  return `${count} verify run${count === 1 ? "" : "s"}`;
}

function rereadHint(observability: MetricObservability): string {
  switch (observability) {
    case "unavailable":
      return "No file reads observed - this session reads files through commands the map could not recognize";
    case "estimated":
      return "Reads that re-read a file unchanged since its last read - inferred from shell commands, so the rate is approximate";
    default:
      return "Reads that re-read a file unchanged since its last read";
  }
}

function errorCaveat(observability: MetricObservability): string {
  return observability === "estimated"
    ? " - inferred from command output; failures inside scripted calls may be missed"
    : "";
}

const ACTION_ORDER = [
  "search",
  "read",
  "edit",
  "exec",
  "verify",
  "other",
] as const;

function countActions(counts: ActionCounts): number {
  return ACTION_ORDER.reduce((sum, key) => sum + counts[key], 0);
}

function mixHint(counts: ActionCounts): string {
  const parts = ACTION_ORDER.filter((key) => counts[key] > 0).map(
    (key) => `${counts[key]} ${key}`
  );
  return parts.length ? parts.join(" · ") : "none";
}

function fmtBytes(bytes: number): string {
  const kb = bytes / 1024;
  if (kb < 1) return `${bytes} B`;
  if (kb < 1000) return `${Math.round(kb)} KB`;
  return `${(kb / 1024).toFixed(1)} MB`;
}

function fmtTokens(n?: number): string {
  if (!n || n <= 0) return "—";
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1000) return `${(n / 1000).toFixed(n >= 10_000 ? 0 : 1)}k`;
  return n.toLocaleString();
}

function fmtCost(n?: number): string {
  if (n == null || n <= 0) return "—";
  if (n < 0.01) return `$${n.toFixed(4)}`;
  return `$${n.toFixed(2)}`;
}

function shortId(id?: string): string {
  if (!id) return "-";
  return id.length > 12 ? `${id.slice(0, 8)}…` : id;
}
