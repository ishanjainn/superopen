import { statSync } from "fs";
import { fileExists, readJSONFile, writeText } from "./nodeio";
import { soPath } from "./root";

/**
 * Graphify's exported graph.html bakes in no node coordinates - vis-network
 * cold-starts a ~200 iteration physics stabilization from scratch on every
 * page load. That pass is the multi-second "reload" on every /graph visit.
 *
 * Cache the positions produced by the first stabilization (posted back by the
 * iframe once) and inject them into graph.html on subsequent serves with
 * physics disabled, so only one real stabilization ever runs per
 * `so graph rebuild`, not once per visit.
 */

type PositionsCache = {
  /** Invalidation key: size+mtime of the graph.html this cache was computed from. */
  sourceStamp: string;
  positions: Record<string, { x: number; y: number }>;
};

function cachePath(): string {
  return soPath("graph", "cache", "positions.json");
}

export function graphHtmlStamp(): string | null {
  const path = soPath("graph", "graph.html");
  if (!fileExists(path)) return null;
  const st = statSync(/* turbopackIgnore: true */ path);
  return `${st.size}-${st.mtimeMs}`;
}

export function readCachedPositions(): Record<string, { x: number; y: number }> | null {
  const stamp = graphHtmlStamp();
  if (!stamp) return null;
  const parsed = readJSONFile<PositionsCache>(cachePath());
  if (!parsed || parsed.sourceStamp !== stamp) return null;
  if (!parsed.positions || typeof parsed.positions !== "object") return null;
  return parsed.positions;
}

/** Overwrites the cache unconditionally with positions for the current graph.html. */
export function writeCachedPositions(
  positions: Record<string, { x: number; y: number }>
): boolean {
  const stamp = graphHtmlStamp();
  if (!stamp) return false;
  const payload: PositionsCache = { sourceStamp: stamp, positions };
  writeText(cachePath(), JSON.stringify(payload));
  return true;
}

/**
 * Injects cached positions into RAW_NODES and disables physics, so vis-network
 * paints the frozen layout immediately instead of re-stabilizing. Falls back
 * to leaving physics on (current behavior) when a node has no cached position -
 * a partial/stale cache should not silently misplace nodes.
 */
export function applyCachedPositions(
  html: string,
  positions: Record<string, { x: number; y: number }>
): string {
  const match = html.match(/const RAW_NODES = (\[[\s\S]*?\]);\n/);
  if (!match) return html;
  let nodes: Array<{ id: string; [k: string]: unknown }>;
  try {
    nodes = JSON.parse(match[1]);
  } catch {
    return html;
  }
  if (nodes.some((n) => !positions[String(n.id)])) {
    // Cache doesn't cover every node in this graph (stale or partial) - bail
    // rather than lay out only some nodes.
    return html;
  }
  const withPositions = nodes.map((n) => ({
    ...n,
    x: positions[String(n.id)].x,
    y: positions[String(n.id)].y,
  }));
  let out = html.replace(
    match[0],
    `const RAW_NODES = ${JSON.stringify(withPositions)};\n`
  );
  // Runs after theming, so applyGraphPerformanceTuning may already have
  // rewritten the literal `stabilization: { iterations: 200, fit: true },`
  // into its adaptive form - match either shape.
  out = out.replace(
    /stabilization: \{[^}]*\},/,
    "stabilization: { enabled: false },"
  );
  out = out.replace(
    /physics: \{\s*enabled: true,/,
    "physics: {\n    enabled: false,"
  );
  return out;
}

/** Script injected into graph.html to report the first stabilization's layout
 * back to the parent Superopen page, which persists it via the positions API. */
export const POSITION_REPORT_SCRIPT = `<script id="so-position-report">
(function () {
  if (typeof network === 'undefined') return;
  network.once('stabilizationIterationsDone', function () {
    try {
      var positions = network.getPositions();
      window.parent.postMessage({ type: 'so-graph-positions', positions: positions }, '*');
    } catch (e) {}
  });
})();
</script>`;
