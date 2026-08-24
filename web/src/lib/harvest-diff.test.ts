import { describe, expect, it } from "vitest";
import { diffStats, parseUnifiedDiff } from "./harvest-diff";

const SAMPLE = `--- a/AGENTS.md
+++ b/AGENTS.md
@@ -1,3 +1,4 @@
 # Agents
 
-Follow graph-first search.
+Follow graph-first search always.
+Prefer so harvest review.
`;

describe("harvest-diff", () => {
  it("counts add and remove lines", () => {
    expect(diffStats(SAMPLE)).toEqual({ plus: 2, minus: 1 });
  });

  it("parses add, remove, and adjacent change", () => {
    const hunks = parseUnifiedDiff(SAMPLE);
    expect(hunks).toHaveLength(1);
    const kinds = hunks[0].lines.map((l) => l.kind);
    expect(kinds.filter((k) => k !== "ctx")).toEqual(["chg-del", "chg-add", "add"]);
    const change = hunks[0].lines.find((l) => l.kind === "chg-add");
    expect(change?.spans?.some((s) => s.changed && s.text.includes("always"))).toBe(
      true,
    );
  });
});
