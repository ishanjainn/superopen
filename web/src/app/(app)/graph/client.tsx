"use client";

import {
  type Dispatch,
  type FormEvent,
  type ReactNode,
  type SetStateAction,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";
import {
  Crosshair,
  Filter,
  RotateCcw,
  Search,
  Settings2,
  Sparkles,
} from "lucide-react";
import FeaturePageHeader from "@/components/shell/feature-page-header";
import { useProject } from "@/components/shell/project-context";
import {
  SessionRail,
  SessionRailDrawer,
  type SessionRailTool,
} from "@/components/session-rail";
import { StellarGraphScene } from "@/graph/StellarGraphScene";
import { DEFAULT_LABEL_COLOR, labelColor } from "@/graph/colors";
import {
  DEFAULT_GRAPH_DISPLAY,
  type GraphData,
  type GraphDisplaySettings,
  type GraphNode,
} from "@/graph/types";
import "@/map/map.css";
import "./graph.css";

const NODE_BUDGET_STEP = 5000;
const DEFAULT_NODE_BUDGET = 5000;
const COMMUNITY_COLORS = [
  "#4e79a7",
  "#f28e2b",
  "#e15759",
  "#76b7b2",
  "#59a14f",
  "#edc948",
  "#b07aa1",
  "#ff9da7",
  "#9c755f",
  "#bab0ac",
];

type RailTab = "inspect" | "find" | "filters" | "ask" | "display";

function budgetKey(project: string | undefined) {
  return `superopen-graph-budget:${project || "default"}`;
}

function loadBudget(project: string | undefined): number {
  try {
    const value = Number(localStorage.getItem(budgetKey(project)));
    if (Number.isFinite(value) && value > 0) return value;
  } catch {
    // Storage is optional.
  }
  return DEFAULT_NODE_BUDGET;
}

function humanEdge(type: string) {
  return type.replaceAll("_", " ").toLowerCase();
}

type Connection = { node: GraphNode; edge: string };

function groupByEdge(rows: Connection[]): [string, Connection[]][] {
  const groups = new Map<string, Connection[]>();
  for (const row of rows) {
    groups.set(row.edge, [...(groups.get(row.edge) ?? []), row]);
  }
  return [...groups.entries()].sort((a, b) => b[1].length - a[1].length);
}

function ConnectionSection({
  title,
  rows,
  onNavigate,
}: {
  title: string;
  rows: Connection[];
  onNavigate: (node: GraphNode) => void;
}) {
  if (rows.length === 0) return null;
  return (
    <div>
      <p className="graph-group-title">
        {title} <span>({rows.length})</span>
      </p>
      {groupByEdge(rows).map(([edge, group]) => (
        <div key={edge}>
          <p className="graph-group-kind">{humanEdge(edge)}</p>
          <div className="graph-list" style={{ padding: "2px 6px" }}>
            {group.slice(0, 25).map(({ node }, index) => (
              <button
                key={`${node.id}-${index}`}
                type="button"
                className="graph-neighbor"
                onClick={() => onNavigate(node)}
              >
                <span
                  className="graph-dot-sm"
                  style={{ backgroundColor: labelColor(node.label) }}
                />
                <span className="graph-neighbor-name">{node.name}</span>
                <span className="graph-neighbor-kind">{node.label}</span>
              </button>
            ))}
            {group.length > 25 ? (
              <p className="graph-more">+{group.length - 25} more</p>
            ) : null}
          </div>
        </div>
      ))}
    </div>
  );
}

function FilterRow({
  checked,
  color,
  label,
  count,
  onChange,
}: {
  checked: boolean;
  color?: string;
  label: string;
  count: number;
  onChange: () => void;
}) {
  return (
    <label className="tb-filter-row">
      <input type="checkbox" checked={checked} onChange={onChange} />
      <span className="graph-swatch" style={{ background: color }} />
      <span className="tb-filter-name">{label}</span>
      <span className="tb-filter-count">{count.toLocaleString()}</span>
    </label>
  );
}

function GraphNotIndexed({
  repo,
  repoRoot,
  onRetry,
}: {
  repo: string;
  repoRoot?: string;
  onRetry: () => void;
}) {
  const [copied, setCopied] = useState(false);
  const command = repoRoot
    ? `cd ${repoRoot} && so graph build`
    : "so graph build";

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(command);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard access is optional; the command stays selectable.
    }
  };

  return (
    <div className="graph-empty">
      <h2 className="graph-empty-title">No graph for {repo} yet</h2>
      <p className="graph-empty-body">
        Superopen has this repository registered but has not indexed its code.
        Build the graph once — it runs locally and refreshes on its own from
        then on.
      </p>
      <div className="graph-empty-command">
        <code>{command}</code>
        <button
          type="button"
          className="graph-empty-copy"
          onClick={() => void copy()}
        >
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
      <p className="graph-empty-note">
        In a coding agent, <code>/so init</code> does the same and also creates{" "}
        <code>.so/</code> if the repository has never been set up.
      </p>
      <button type="button" className="graph-retry" onClick={onRetry}>
        Retry
      </button>
    </div>
  );
}

export default function GraphPage() {
  const { projectId, projects, currentSlug } = useProject();
  const [data, setData] = useState<GraphData | null>(null);
  const [loading, setLoading] = useState(true);
  const [progress, setProgress] = useState<{ received: number; total: number | null }>({
    received: 0,
    total: null,
  });
  const [error, setError] = useState("");
  const [errorCode, setErrorCode] = useState("");
  const [budget, setBudget] = useState(DEFAULT_NODE_BUDGET);
  const [budgetDraft, setBudgetDraft] = useState(String(DEFAULT_NODE_BUDGET));
  const [showLabels, setShowLabels] = useState(true);
  const [display, setDisplay] =
    useState<GraphDisplaySettings>(DEFAULT_GRAPH_DISPLAY);
  const [tab, setTab] = useState<RailTab | null>(null);
  const [selected, setSelected] = useState<GraphNode | null>(null);
  const [search, setSearch] = useState("");
  const [enabledLabels, setEnabledLabels] = useState<Set<string>>(new Set());
  const [enabledEdges, setEnabledEdges] = useState<Set<string>>(new Set());
  const [enabledCommunities, setEnabledCommunities] = useState<Set<string>>(
    new Set(),
  );
  const [question, setQuestion] = useState("");
  const [answer, setAnswer] = useState("");
  const [querying, setQuerying] = useState(false);
  const [queryError, setQueryError] = useState("");

  useEffect(() => {
    const timer = window.setTimeout(() => {
      const next = loadBudget(projectId);
      setBudget(next);
      setBudgetDraft(String(next));
    }, 0);
    return () => window.clearTimeout(timer);
  }, [projectId]);

  const loadGraph = useCallback(async () => {
    setLoading(true);
    setError("");
    setErrorCode("");
    setProgress({ received: 0, total: null });
    try {
      const url = new URL("/api/graph/layout", window.location.origin);
      url.searchParams.set("max_nodes", String(budget));
      if (projectId) url.searchParams.set("project", projectId);
      const response = await fetch(url.toString());
      if (!response.ok) {
        const failure = (await response.json().catch(() => null)) as
          | { error?: string; code?: string }
          | null;
        setErrorCode(failure?.code || "");
        throw new Error(failure?.error || "Graph layout failed");
      }
      let body: GraphData;
      if (response.body) {
        const reader = response.body.getReader();
        const total = Number(response.headers.get("content-length")) || null;
        const chunks: Uint8Array[] = [];
        let received = 0;
        for (;;) {
          const { done, value } = await reader.read();
          if (done) break;
          chunks.push(value);
          received += value.length;
          setProgress({ received, total });
        }
        const bytes = new Uint8Array(received);
        let offset = 0;
        for (const chunk of chunks) {
          bytes.set(chunk, offset);
          offset += chunk.length;
        }
        body = JSON.parse(new TextDecoder().decode(bytes)) as GraphData;
      } else {
        body = (await response.json()) as GraphData;
      }
      body.nodes ??= [];
      body.edges ??= [];
      setData(body);
      setEnabledLabels(new Set(body.nodes.map((node) => node.label)));
      setEnabledEdges(new Set(body.edges.map((edge) => edge.type)));
      setEnabledCommunities(
        new Set(body.nodes.map((node) => node.community || "Unclustered")),
      );
      setSelected(null);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Graph layout failed");
    } finally {
      setLoading(false);
    }
  }, [budget, projectId]);

  useEffect(() => {
    const timer = window.setTimeout(() => void loadGraph(), 0);
    return () => window.clearTimeout(timer);
  }, [loadGraph]);

  const activeProject = useMemo(
    () => projects.find((project) => project.id === projectId),
    [projects, projectId],
  );
  const repoLabel =
    activeProject?.slug || activeProject?.name || currentSlug || "this repository";
  const notIndexed = errorCode === "graph_not_indexed";

  const labelCounts = useMemo(() => {
    const counts = new Map<string, number>();
    for (const node of data?.nodes ?? []) {
      counts.set(node.label, (counts.get(node.label) ?? 0) + 1);
    }
    return [...counts].sort((a, b) => b[1] - a[1]);
  }, [data]);
  const edgeCounts = useMemo(() => {
    const counts = new Map<string, number>();
    for (const edge of data?.edges ?? []) {
      counts.set(edge.type, (counts.get(edge.type) ?? 0) + 1);
    }
    return [...counts].sort((a, b) => b[1] - a[1]);
  }, [data]);
  const communityCounts = useMemo(() => {
    const counts = new Map<string, number>();
    for (const node of data?.nodes ?? []) {
      const community = node.community || "Unclustered";
      counts.set(community, (counts.get(community) ?? 0) + 1);
    }
    return [...counts].sort((a, b) => b[1] - a[1]);
  }, [data]);
  const communityColors = useMemo(
    () =>
      new Map(
        communityCounts.map(([community], index) => [
          community,
          COMMUNITY_COLORS[index % COMMUNITY_COLORS.length],
        ]),
      ),
    [communityCounts],
  );
  const communityColor = useCallback(
    (community?: string) =>
      communityColors.get(community || "Unclustered") || DEFAULT_LABEL_COLOR,
    [communityColors],
  );

  const filteredData = useMemo<GraphData | null>(() => {
    if (!data) return null;
    const nodes = data.nodes.filter(
      (node) =>
        enabledLabels.has(node.label) &&
        enabledCommunities.has(node.community || "Unclustered"),
    );
    const ids = new Set(nodes.map((node) => node.id));
    const edges = data.edges.filter(
      (edge) =>
        enabledEdges.has(edge.type) &&
        ids.has(edge.source) &&
        ids.has(edge.target),
    );
    return { ...data, nodes, edges };
  }, [data, enabledLabels, enabledEdges, enabledCommunities]);

  const highlightedIds = useMemo(() => {
    if (!selected || !filteredData) return null;
    const ids = new Set<number>([selected.id]);
    for (const edge of filteredData.edges) {
      if (edge.source === selected.id) ids.add(edge.target);
      if (edge.target === selected.id) ids.add(edge.source);
    }
    return ids;
  }, [selected, filteredData]);

  const connections = useMemo(() => {
    const empty = { outbound: [] as Connection[], inbound: [] as Connection[] };
    if (!selected || !filteredData) return empty;
    const byID = new Map(filteredData.nodes.map((node) => [node.id, node]));
    for (const edge of filteredData.edges) {
      if (edge.source === selected.id) {
        const node = byID.get(edge.target);
        if (node) empty.outbound.push({ node, edge: edge.type });
      }
      if (edge.target === selected.id) {
        const node = byID.get(edge.source);
        if (node) empty.inbound.push({ node, edge: edge.type });
      }
    }
    return empty;
  }, [selected, filteredData]);

  const searchMatches = useMemo(() => {
    const value = search.trim().toLowerCase();
    if (!value || !filteredData) return [];
    return filteredData.nodes
      .filter(
        (node) =>
          node.name.toLowerCase().includes(value) ||
          node.qualified_name.toLowerCase().includes(value) ||
          node.file_path?.toLowerCase().includes(value),
      )
      .sort((a, b) => b.degree - a.degree)
      .slice(0, 30);
  }, [search, filteredData]);

  const commitBudget = () => {
    const parsed = Number(budgetDraft);
    const next = Number.isFinite(parsed)
      ? Math.max(
          NODE_BUDGET_STEP,
          Math.round(parsed / NODE_BUDGET_STEP) * NODE_BUDGET_STEP,
        )
      : DEFAULT_NODE_BUDGET;
    setBudgetDraft(String(next));
    try {
      localStorage.setItem(budgetKey(projectId), String(next));
    } catch {
      // Storage is optional.
    }
    setBudget(next);
  };

  const toggle = (
    setter: Dispatch<SetStateAction<Set<string>>>,
    value: string,
  ) => {
    setter((current) => {
      const next = new Set(current);
      if (next.has(value)) next.delete(value);
      else next.add(value);
      return next;
    });
  };

  const inspect = useCallback((node: GraphNode) => {
    setSelected(node);
    setTab("inspect");
  }, []);

  async function queryGraph(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const value = question.trim();
    if (!value) return;
    setQuerying(true);
    setQueryError("");
    try {
      const url = new URL("/api/graph/query", window.location.origin);
      url.searchParams.set("q", value);
      if (projectId) url.searchParams.set("project", projectId);
      const response = await fetch(url.toString());
      const body = (await response.json()) as { answer?: string; error?: string };
      if (!response.ok) throw new Error(body.error || "Graph query failed");
      setAnswer(body.answer || "");
    } catch (cause) {
      setQueryError(cause instanceof Error ? cause.message : "Graph query failed");
    } finally {
      setQuerying(false);
    }
  }

  const shown = filteredData?.nodes.length ?? 0;
  const shownEdges = filteredData?.edges.length ?? 0;
  const loaded = data?.nodes.length ?? 0;
  const total = data?.total_nodes ?? 0;
  const closeTab = useCallback(() => setTab(null), []);

  const tools: SessionRailTool[] = [
    {
      id: "inspect",
      label: "Inspect",
      hint: "Inspect the selected node",
      icon: Crosshair,
      active: tab === "inspect",
      onClick: () => setTab((current) => (current === "inspect" ? null : "inspect")),
    },
    {
      id: "find",
      label: "Find",
      hint: "Search nodes by name, symbol, or path",
      icon: Search,
      active: tab === "find",
      onClick: () => setTab((current) => (current === "find" ? null : "find")),
    },
    {
      id: "filters",
      label: "Filters",
      hint: "Node types, communities, relationships, node budget",
      icon: Filter,
      active: tab === "filters",
      onClick: () => setTab((current) => (current === "filters" ? null : "filters")),
    },
    {
      id: "ask",
      label: "Ask",
      hint: "Ask the graph a question in plain language",
      icon: Sparkles,
      active: tab === "ask",
      badge: answer ? "done" : null,
      onClick: () => setTab((current) => (current === "ask" ? null : "ask")),
    },
    {
      id: "display",
      label: "Display",
      hint: "Labels, edge brightness, glow, bloom",
      icon: Settings2,
      active: tab === "display",
      onClick: () => setTab((current) => (current === "display" ? null : "display")),
    },
  ];

  let panel: ReactNode = null;
  if (tab === "inspect") {
    panel = (
      <SessionRailDrawer title="Inspect" onClose={closeTab}>
        {selected ? (
          <>
            <div className="graph-inspect-head">
              <div className="graph-inspect-title">
                <span
                  className="graph-dot"
                  style={{ backgroundColor: labelColor(selected.label) }}
                />
                <strong>{selected.name}</strong>
              </div>
              <div className="graph-chips">
                <span
                  className="graph-chip"
                  style={{
                    backgroundColor: `${labelColor(selected.label)}18`,
                    color: labelColor(selected.label),
                  }}
                >
                  {selected.label}
                </span>
                <span
                  className="graph-chip"
                  title="Call-graph community"
                  style={{
                    backgroundColor: `${communityColor(selected.community)}18`,
                    color: communityColor(selected.community),
                  }}
                >
                  {selected.community || "Unclustered"}
                </span>
              </div>
              {selected.file_path || selected.qualified_name ? (
                <p className="graph-inspect-path">
                  {selected.file_path || selected.qualified_name}
                  {selected.start_line ? (
                    <span>
                      {" "}
                      :{selected.start_line}
                      {selected.end_line && selected.end_line !== selected.start_line
                        ? `-${selected.end_line}`
                        : ""}
                    </span>
                  ) : null}
                </p>
              ) : null}
              <div className="graph-stats">
                <span>
                  <i>Out</i>
                  <b>{connections.outbound.length}</b>
                </span>
                <span>
                  <i>In</i>
                  <b>{connections.inbound.length}</b>
                </span>
                <span>
                  <i>Total</i>
                  <b>{connections.outbound.length + connections.inbound.length}</b>
                </span>
              </div>
              <button
                type="button"
                className="graph-clear"
                onClick={() => setSelected(null)}
              >
                Clear selection
              </button>
            </div>
            <ConnectionSection
              title="References"
              rows={connections.outbound}
              onNavigate={setSelected}
            />
            <ConnectionSection
              title="Referenced by"
              rows={connections.inbound}
              onNavigate={setSelected}
            />
            {connections.outbound.length + connections.inbound.length === 0 ? (
              <p className="graph-note">No connections</p>
            ) : null}
          </>
        ) : (
          <p className="graph-note">
            Click any star on the stage to read its relationships, or find one by
            name.
          </p>
        )}
      </SessionRailDrawer>
    );
  } else if (tab === "find") {
    panel = (
      <SessionRailDrawer title="Find" onClose={closeTab}>
        <div className="graph-ask">
          <input
            className="rail-map-input"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Search nodes…"
            autoFocus
          />
        </div>
        <div className="graph-list">
          {searchMatches.map((node) => (
            <button
              key={node.id}
              type="button"
              className="graph-neighbor"
              onClick={() => inspect(node)}
            >
              <span
                className="graph-dot-sm"
                style={{ backgroundColor: labelColor(node.label) }}
              />
              <span className="graph-neighbor-name">{node.name}</span>
              <span className="graph-neighbor-kind">{node.label}</span>
            </button>
          ))}
        </div>
        {search.trim() && searchMatches.length === 0 ? (
          <p className="graph-note">No node in view matches that.</p>
        ) : null}
      </SessionRailDrawer>
    );
  } else if (tab === "filters") {
    panel = (
      <SessionRailDrawer title="Filters" onClose={closeTab}>
        <div className="graph-section-head">
          <span className="tb-label">Node types</span>
          <span>
            <button
              type="button"
              className="graph-mini-btn"
              onClick={() => {
                setEnabledLabels(new Set(labelCounts.map(([label]) => label)));
                setEnabledEdges(new Set(edgeCounts.map(([edge]) => edge)));
                setEnabledCommunities(
                  new Set(communityCounts.map(([community]) => community)),
                );
              }}
            >
              All
            </button>
            {" · "}
            <button
              type="button"
              className="graph-mini-btn"
              onClick={() => {
                setEnabledLabels(new Set());
                setEnabledEdges(new Set());
                setEnabledCommunities(new Set());
              }}
            >
              None
            </button>
          </span>
        </div>
        <div className="tb-filter-list">
          {labelCounts.map(([label, count]) => (
            <FilterRow
              key={label}
              checked={enabledLabels.has(label)}
              color={labelColor(label)}
              label={label}
              count={count}
              onChange={() => toggle(setEnabledLabels, label)}
            />
          ))}
        </div>
        <div className="graph-section-head">
          <span className="tb-label">Communities</span>
        </div>
        <div className="tb-filter-list">
          {communityCounts.map(([community, count], index) => (
            <FilterRow
              key={community}
              checked={enabledCommunities.has(community)}
              color={COMMUNITY_COLORS[index % COMMUNITY_COLORS.length]}
              label={community}
              count={count}
              onChange={() => toggle(setEnabledCommunities, community)}
            />
          ))}
        </div>
        <div className="graph-section-head">
          <span className="tb-label">Relationships</span>
        </div>
        <div className="tb-filter-list">
          {edgeCounts.map(([edge, count]) => (
            <FilterRow
              key={edge}
              checked={enabledEdges.has(edge)}
              label={humanEdge(edge)}
              count={count}
              onChange={() => toggle(setEnabledEdges, edge)}
            />
          ))}
        </div>
        <div className="graph-field">
          <span className="graph-field-head">Node budget</span>
          <input
            className="rail-map-input"
            type="number"
            step={NODE_BUDGET_STEP}
            min={NODE_BUDGET_STEP}
            value={budgetDraft}
            onChange={(event) => setBudgetDraft(event.target.value)}
            onBlur={commitBudget}
            onKeyDown={(event) => {
              if (event.key === "Enter") event.currentTarget.blur();
            }}
          />
        </div>
        {total > loaded ? (
          <p className="graph-note graph-note-warn">
            Showing {loaded.toLocaleString()} of {total.toLocaleString()} nodes -
            raise the budget or narrow the filters.
          </p>
        ) : null}
      </SessionRailDrawer>
    );
  } else if (tab === "ask") {
    panel = (
      <SessionRailDrawer title="Ask the graph" onClose={closeTab}>
        <form className="graph-ask" onSubmit={queryGraph}>
          <div className="graph-ask-row">
            <input
              className="rail-map-input"
              value={question}
              onChange={(event) => setQuestion(event.target.value)}
              placeholder="How does this subsystem work?"
              autoFocus
            />
            <button
              type="submit"
              className="rail-map-go"
              disabled={querying || !question.trim()}
            >
              {querying ? "…" : "Ask"}
            </button>
          </div>
        </form>
        {queryError ? <p className="graph-error">{queryError}</p> : null}
        {answer ? <pre className="graph-answer">{answer}</pre> : null}
        {!answer && !queryError ? (
          <p className="graph-note">
            Answers come from the native graph - the same query the agents run.
          </p>
        ) : null}
      </SessionRailDrawer>
    );
  } else if (tab === "display") {
    panel = (
      <SessionRailDrawer title="Display" onClose={closeTab}>
        <label className="graph-toggle">
          <input
            type="checkbox"
            checked={showLabels}
            onChange={() => setShowLabels((value) => !value)}
          />
          Labels on hubs
        </label>
        {(
          [
            ["edgeBrightness", "Edge brightness", 0.1, 3],
            ["nodeGlow", "Node glow", 0, 2],
            ["bloom", "Bloom", 0, 2],
          ] as const
        ).map(([key, label, min, max]) => (
          <div key={key} className="graph-field">
            <span className="graph-field-head">
              {label}
              <strong>{display[key].toFixed(1)}</strong>
            </span>
            <input
              type="range"
              min={min}
              max={max}
              step={0.1}
              value={display[key]}
              onChange={(event) =>
                setDisplay((current) => ({
                  ...current,
                  [key]: Number(event.target.value),
                }))
              }
            />
          </div>
        ))}
      </SessionRailDrawer>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <FeaturePageHeader
        title="Graph"
        actions={
          <button
            type="button"
            onClick={() => void loadGraph()}
            className="rounded border border-neutral-200 p-1.5 text-neutral-500 hover:bg-neutral-50 hover:text-neutral-900"
            title="Rebuild the layout"
          >
            <RotateCcw size={14} />
          </button>
        }
      />

      <div className="map-root graph-root">
        <main className="app-frame">
          <section className="stage">
            <div className="viewport">
              {filteredData && filteredData.nodes.length > 0 ? (
                <StellarGraphScene
                  className="session-map"
                  data={filteredData}
                  highlightedIds={highlightedIds}
                  focusIds={highlightedIds}
                  showLabels={showLabels}
                  display={display}
                  onNodeClick={inspect}
                  onBackgroundClick={() => setSelected(null)}
                />
              ) : null}

              <div className="graph-grid" aria-hidden />

              <div className="hud">
                <SessionRail
                  bare
                  aria-label="Graph rail"
                  tools={tools}
                  panel={panel}
                  onClosePanel={closeTab}
                >
                  <div className="tb-band">
                    <div className="tb-cell tb-shrink">
                      <span className="tb-label">Repository</span>
                      <span className="tb-value tb-mono">
                        {data?.project || repoLabel}
                      </span>
                    </div>
                    <div className="tb-cell tb-shrink">
                      <span className="tb-label">Nodes</span>
                      <span className="tb-value tb-mono">
                        {shown.toLocaleString()}
                        {shown === loaded ? "" : ` of ${loaded.toLocaleString()}`}
                        {total > loaded ? (
                          <span className="tb-partial"> · budgeted</span>
                        ) : null}
                      </span>
                    </div>
                    <div className="tb-cell tb-shrink">
                      <span className="tb-label">Relationships</span>
                      <span className="tb-value tb-mono">
                        {shownEdges.toLocaleString()}
                      </span>
                    </div>
                    <div className="tb-cell tb-shrink">
                      <span className="tb-label">Kinds</span>
                      <span className="tb-value tb-mono tb-activity">
                        {labelCounts.slice(0, 4).map(([label, count]) => (
                          <span key={label}>
                            {label} {count.toLocaleString()}
                          </span>
                        ))}
                      </span>
                    </div>
                    {selected ? (
                      <div className="tb-cell tb-shrink">
                        <span className="tb-label">Selected</span>
                        <span className="tb-value tb-mono tb-activity">
                          <span>{selected.name}</span>
                          <span style={{ color: communityColor(selected.community) }}>
                            {selected.community || "Unclustered"}
                          </span>
                        </span>
                      </div>
                    ) : null}
                  </div>
                </SessionRail>
              </div>

              {loading ? (
                <div className="map-status">
                  <div className="graph-progress">
                    <div className="graph-progress-track">
                      <div
                        className="graph-progress-fill"
                        style={{
                          width: progress.total
                            ? `${Math.min(100, (progress.received / progress.total) * 100)}%`
                            : "35%",
                        }}
                      />
                    </div>
                    <span>
                      Computing anchored 3D layout
                      {progress.received > 0
                        ? ` · ${(progress.received / 1024).toFixed(0)} KB`
                        : "…"}
                    </span>
                  </div>
                </div>
              ) : null}

              {error && !loading ? (
                <div
                  className={
                    notIndexed ? "map-status" : "map-status map-status-error"
                  }
                >
                  {notIndexed ? (
                    <GraphNotIndexed
                      repo={repoLabel}
                      repoRoot={activeProject?.repo_root}
                      onRetry={() => void loadGraph()}
                    />
                  ) : (
                    <div>
                      {error}
                      <div>
                        <button
                          type="button"
                          className="graph-retry"
                          onClick={() => void loadGraph()}
                        >
                          Retry
                        </button>
                      </div>
                    </div>
                  )}
                </div>
              ) : null}

              {!loading && !error && filteredData?.nodes.length === 0 ? (
                <div className="map-status">Every node is filtered out.</div>
              ) : null}
            </div>
          </section>
        </main>
      </div>
    </div>
  );
}
