import { mkdtempSync, writeFileSync, mkdirSync } from "fs";
import { tmpdir } from "os";
import { join } from "path";
import { describe, expect, it } from "vitest";
import {
  applyCachedPositions,
  graphHtmlStamp,
  readCachedPositions,
  writeCachedPositions,
} from "./graph-positions";
import { runWithWorkspace } from "./root";

function makeWorkspace() {
  const repoRoot = mkdtempSync(join(tmpdir(), "so-graph-positions-"));
  const soRoot = join(repoRoot, ".so");
  mkdirSync(join(soRoot, "graph"), { recursive: true });
  return { repoRoot, soRoot };
}

const SAMPLE_HTML = `<html><body>
const RAW_NODES = [{"id":"a","label":"A"},{"id":"b","label":"B"}];
const RAW_EDGES = [];
const network = new vis.Network(container, { nodes: nodesDS, edges: edgesDS }, {
  physics: {
    enabled: true,
    solver: 'forceAtlas2Based',
    stabilization: { iterations: 200, fit: true },
  },
});
</body></html>`;

describe("applyCachedPositions", () => {
  it("injects x/y into every RAW_NODES entry and disables physics", () => {
    const out = applyCachedPositions(SAMPLE_HTML, {
      a: { x: 1, y: 2 },
      b: { x: 3, y: 4 },
    });
    expect(out).toContain('"id":"a"');
    expect(out).toContain('"x":1');
    expect(out).toContain('"y":4');
    expect(out).toContain("stabilization: { enabled: false }");
    expect(out).toContain("enabled: false,\n    solver:");
    expect(out).not.toContain("iterations: 200");
  });

  it("rewrites stabilization even when it was already tuned to an adaptive form", () => {
    // applyGraphPerformanceTuning (part of theming) runs before this and may
    // have already turned the literal iterations:200 into a ternary - make
    // sure we still find and neutralize it rather than leaving it dangling.
    const tuned = SAMPLE_HTML.replace(
      "stabilization: { iterations: 200, fit: true },",
      "stabilization: { iterations: RAW_NODES.length > 2500 ? 50 : 200, fit: true },"
    );
    const out = applyCachedPositions(tuned, { a: { x: 1, y: 2 }, b: { x: 3, y: 4 } });
    expect(out).toContain("stabilization: { enabled: false },");
    expect(out).not.toContain("RAW_NODES.length > 2500");
  });

  it("leaves the graph untouched when the cache doesn't cover every node", () => {
    const out = applyCachedPositions(SAMPLE_HTML, { a: { x: 1, y: 2 } });
    expect(out).toBe(SAMPLE_HTML);
  });

  it("is a no-op on HTML without a RAW_NODES literal", () => {
    const html = "<html>no graph here</html>";
    expect(applyCachedPositions(html, { a: { x: 1, y: 2 } })).toBe(html);
  });
});

describe("position cache round-trip", () => {
  it("returns null when graph.html doesn't exist", () => {
    const { repoRoot, soRoot } = makeWorkspace();
    runWithWorkspace({ repoRoot, soRoot }, () => {
      expect(graphHtmlStamp()).toBeNull();
      expect(readCachedPositions()).toBeNull();
    });
  });

  it("writes and reads back positions keyed to the current graph.html", () => {
    const { repoRoot, soRoot } = makeWorkspace();
    writeFileSync(join(soRoot, "graph", "graph.html"), SAMPLE_HTML);
    runWithWorkspace({ repoRoot, soRoot }, () => {
      const ok = writeCachedPositions({ a: { x: 5, y: 6 } });
      expect(ok).toBe(true);
      expect(readCachedPositions()).toEqual({ a: { x: 5, y: 6 } });
    });
  });

  it("invalidates the cache when graph.html changes after `so graph rebuild`", () => {
    const { repoRoot, soRoot } = makeWorkspace();
    const htmlPath = join(soRoot, "graph", "graph.html");
    writeFileSync(htmlPath, SAMPLE_HTML);
    runWithWorkspace({ repoRoot, soRoot }, () => {
      writeCachedPositions({ a: { x: 5, y: 6 } });
      expect(readCachedPositions()).not.toBeNull();
    });
    // Simulate a rebuild producing a different graph.html.
    writeFileSync(htmlPath, SAMPLE_HTML + "\n<!-- rebuilt -->");
    runWithWorkspace({ repoRoot, soRoot }, () => {
      expect(readCachedPositions()).toBeNull();
    });
  });
});
