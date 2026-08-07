import { describe, expect, it } from "vitest";
import { parseCodexRolloutLines } from "./trace";

describe("Codex Map rollout parsing", () => {
  it("turns code-mode apply_patch calls into file edit targets", () => {
    const patch = [
      "*** Begin Patch",
      "*** Update File: /repo/web/src/app.tsx",
      "-old",
      "+new",
      "*** Add File: /repo/docs/new.md",
      "+hello",
      "*** End Patch",
    ].join("\n");
    const code = `const patch = ${JSON.stringify(patch)};\ntext(await tools.apply_patch(patch));\n`;
    const line = JSON.stringify({
      timestamp: "2026-08-07T00:00:00Z",
      type: "response_item",
      payload: { type: "custom_tool_call", name: "exec", input: code, status: "completed" },
    });

    const parsed = parseCodexRolloutLines([line], "/repo");
    expect(parsed.events).toHaveLength(1);
    expect(parsed.events[0]).toMatchObject({ tool: "apply_patch", action: "edit" });
    expect(parsed.events[0].targets).toEqual([
      { path: "web/src/app.tsx", touch: "edit" },
      { path: "docs/new.md", touch: "edit" },
    ]);
  });

  it("keeps shell execution separate from edits", () => {
    const code = 'const r=await tools.exec_command({cmd:"go test ./..."}); text(r.output);';
    const line = JSON.stringify({
      type: "response_item",
      payload: { type: "custom_tool_call", name: "exec", input: code },
    });
    const event = parseCodexRolloutLines([line], "/repo").events[0];
    expect(event).toMatchObject({ tool: "Bash", action: "exec", summary: "go test ./..." });
    expect(event.targets).toEqual([]);
  });
});
