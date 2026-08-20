import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

describe("sidebar", () => {
  it("orders Graph, then Sessions, Memory, and Settings", () => {
    const src = readFileSync(new URL("./sidebar.tsx", import.meta.url), "utf8");
    const graph = src.indexOf('text: "Graph"');
    const sessions = src.indexOf('text: "Sessions"');
    const memory = src.indexOf('text: "Memory"');
    const settings = src.indexOf('text: "Settings"');
    expect(sessions).toBeGreaterThan(graph);
    expect(memory).toBeGreaterThan(sessions);
    expect(settings).toBeGreaterThan(memory);
    expect(src).toContain('link: "/memory"');
  });
});
