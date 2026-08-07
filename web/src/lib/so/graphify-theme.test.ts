import { describe, expect, it } from "vitest";
import { applyGraphPerformanceTuning } from "./graphify-theme";

describe("applyGraphPerformanceTuning", () => {
  it("uses adaptive stabilization and straight edges for large graphs", () => {
    const html = `
      stabilization: { iterations: 200, fit: true },
      edges: { smooth: { type: 'continuous', roundness: 0.2 }, selectionWidth: 3 },
    `;

    const tuned = applyGraphPerformanceTuning(html);

    expect(tuned).toContain("RAW_NODES.length > 2500 ? 50");
    expect(tuned).toContain("RAW_NODES.length > 1200 ? 100 : 200");
    expect(tuned).toContain("RAW_EDGES.length > 5000 ? false");
    expect(tuned).not.toContain("stabilization: { iterations: 200");
  });

  it("leaves unknown Graphify output intact", () => {
    const html = "<html><body>graph</body></html>";
    expect(applyGraphPerformanceTuning(html)).toBe(html);
  });
});
