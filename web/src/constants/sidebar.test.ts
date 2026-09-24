import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

describe("sidebar", () => {
  it("orders Graph, then Sessions, Scan, Memory, Harvest, and Settings", () => {
    const src = readFileSync(new URL("./sidebar.tsx", import.meta.url), "utf8");
    const graph = src.indexOf('text: "Graph"');
    const sessions = src.indexOf('text: "Sessions"');
    const scan = src.indexOf('text: "Scan"');
    const memory = src.indexOf('text: "Memory"');
    const harvest = src.indexOf('text: "Harvest"');
    const settings = src.indexOf('text: "Settings"');
    expect(sessions).toBeGreaterThan(graph);
    expect(scan).toBeGreaterThan(sessions);
    expect(memory).toBeGreaterThan(scan);
    expect(harvest).toBeGreaterThan(memory);
    expect(settings).toBeGreaterThan(harvest);
    expect(src).toContain('link: "/harvest"');
    expect(src).toContain('link: "/scan"');
  });
});
