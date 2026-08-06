export type GraphHtmlStatus = {
  ok: boolean;
  reason?: string;
  tip?: string;
};

/** Graphify must own communities (non-empty LEGEND). */
export function inspectGraphHtml(raw: string): GraphHtmlStatus {
  if (!raw.trim()) {
    return {
      ok: false,
      reason: "graph.html is empty",
      tip: "so graph rebuild",
    };
  }
  if (!/\b(const|var)\s+LEGEND\b/.test(raw)) {
    return {
      ok: false,
      reason: "graph.html has no Graphify community LEGEND",
      tip: "so graph rebuild",
    };
  }
  if (
    /const\s+LEGEND\s*=\s*\[\s*\]/.test(raw) ||
    /var\s+LEGEND\s*=\s*\[\s*\]/.test(raw)
  ) {
    return {
      ok: false,
      reason: "Graphify community LEGEND is empty",
      tip: "so graph rebuild   or   graphify cluster-only . && graphify export html --graph .so/graph/graph.json",
    };
  }
  return { ok: true };
}
