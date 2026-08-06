import { memo, useEffect, useRef, useState, type ReactNode } from "react";
import { Mountain, TreePine, type LucideIcon } from "lucide-react";
import {
  SessionRail,
  type SessionRailTool,
} from "@/components/session-rail";
import type { ActionCounts, CityMap, MetricObservability, Trace } from "../types";

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
  checkpoints?: { id: number; label?: string; files?: string[] }[];
}

interface HudProps {
  trace?: Trace;
  city?: CityMap;
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
  onOpenAgents?: () => void;
  locked?: boolean;
}

const CHURN_PANEL_ROWS = 8;

export const Hud = memo(function Hud({
  trace,
  city,
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
  onOpenAgents,
  locked = false,
}: HudProps) {
  const stats = trace?.stats;
  const readFinal = stats ? stats.fovea - stats.edited : 0;
  const unvisitedNow = stats
    ? Math.max(0, stats.filesInRepo - editedNow - readNow - seenNow)
    : 0;
  const unvisitedFinal = stats
    ? Math.max(0, stats.filesInRepo - stats.fovea - stats.parafovea)
    : 0;
  const ghostCount = city
    ? city.files.reduce((n, file) => n + (file.ghost ? 1 : 0), 0)
    : 0;
  const errorCount = stats ? countActions(stats.errors) : 0;
  const showReview = stats
    ? errorCount > 0 || stats.churnFiles > 0 || stats.actions.edit > 0
    : false;
  const panelOpen = Boolean(panel);

  const [churnOpen, setChurnOpen] = useState(false);
  const churnPanelRef = useRef<HTMLDivElement | null>(null);
  const churnToggleRef = useRef<HTMLButtonElement | null>(null);

  useEffect(() => setChurnOpen(false), [trace]);
  useEffect(() => {
    if (panelOpen) setChurnOpen(false);
  }, [panelOpen]);

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

  if (!city) {
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

  const commits = sessionExtras?.commits || [];
  const prs = sessionExtras?.pullRequests || [];
  const checkpoints = sessionExtras?.checkpoints || [];
  const showLinked =
    Boolean(sessionExtras?.branch) ||
    Boolean(sessionExtras?.attribution) ||
    commits.length > 0 ||
    prs.length > 0;

  return (
    <div className="hud" aria-hidden={false}>
      <SessionRail
        bare
        aria-label="Session map title block"
        locked={locked}
        tools={railTools}
        panel={panel}
        onClosePanel={onClosePanel}
        viewToggle={
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
        }
      >
        <div className="tb-band">
          <div className="tb-cell tb-shrink">
            <span className="tb-label">Revision</span>
            <span className="tb-value tb-mono">
              {city.repo.commit || "worktree"}
              {city.repo.dirty ? (
                <span className="tb-dirty" title="Uncommitted changes">
                  {" "}
                  ● dirty
                </span>
              ) : null}
            </span>
          </div>

          {showLinked ? (
            <div className="tb-cell tb-shrink">
              <span className="tb-label">Linked</span>
              <span className="tb-value tb-mono tb-activity">
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

          <div className="tb-cell tb-grow">
            <span className="tb-label">Model</span>
            <span className="tb-value tb-mono tb-model-row">
              <span>{trace?.session.model || "-"}</span>
              {trace && agentLabel ? (
                <button
                  className="tb-lens-chip"
                  onClick={onOpenAgents}
                  disabled={!onOpenAgents || locked}
                  aria-label={`Open Agent lenses, current ${agentLabel}`}
                >
                  {agentLabel}
                </button>
              ) : null}
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
                <span className="tb-value tb-mono tb-activity">
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
                      onClick={onOpenAgents}
                      disabled={!onOpenAgents || locked}
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
                  {city.repo.truncated ? (
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

              <div className="tb-cell tb-shrink">
                <span className="tb-label">Session</span>
                <span className="tb-value tb-mono tb-session-id">
                  {shortId(trace?.session.id)}
                </span>
              </div>

              {checkpoints.length > 0 ? (
                <div className="tb-cell tb-shrink">
                  <span className="tb-label">Checkpoints</span>
                  <span className="tb-value tb-mono tb-activity">
                    {checkpoints.slice(0, 4).map((cp) => (
                      <span key={cp.id}>
                        #{cp.id}
                        {cp.label ? ` · ${cp.label}` : ""}
                        {cp.files?.length ? ` · ${cp.files.length} files` : ""}
                      </span>
                    ))}
                  </span>
                </div>
              ) : null}
            </>
          ) : null}
        </div>

        {stats && showReview ? (
          <div className="tb-band tb-review">
            <div className="tb-cell tb-grow">
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
          </div>
        ) : null}

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

function shortId(id?: string): string {
  if (!id) return "-";
  return id.length > 12 ? `${id.slice(0, 8)}…` : id;
}
