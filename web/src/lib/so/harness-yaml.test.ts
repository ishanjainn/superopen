import { describe, expect, it } from "vitest";
import {
  appendDeniedTool,
  defaultGuardrailsYaml,
  parseGuardrailsDoc,
  removeDeniedTool,
} from "./harness-yaml";

describe("denied tool guardrails", () => {
  it("round-trips hook tool patterns", () => {
    const added = appendDeniedTool(
      defaultGuardrailsYaml(),
      "mcp__production__delete_*"
    );
    expect(parseGuardrailsDoc(added)?.denied_tools).toEqual([
      "mcp__production__delete_*",
    ]);
    expect(added).not.toContain("denied_tools: []");
    expect(
      parseGuardrailsDoc(removeDeniedTool(added, "mcp__production__delete_*"))
        ?.denied_tools
    ).toEqual([]);
  });
});
