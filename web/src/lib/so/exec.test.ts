import { describe, expect, it } from "vitest";
import { soJSON, soJSONRows } from "./exec";

describe("soJSONRows", () => {
  it("reads HumanOrJSON list payloads from data", () => {
    expect(
      soJSONRows({ data: [{ id: 1 }, { id: 2 }], items: undefined }),
    ).toEqual([{ id: 1 }, { id: 2 }]);
  });

  it("reads Empty() payloads from items", () => {
    expect(soJSONRows({ data: undefined, items: [] })).toEqual([]);
  });

  it("does not invent rows from an object payload", () => {
    expect(
      soJSONRows({
        data: { text: "No matching nodes found.", seeds: [] },
        items: undefined,
      }),
    ).toEqual([]);
  });
});

describe("soJSON", () => {
  it("returns an error when the installed CLI is missing", async () => {
    const previous = process.env.SUPEROPEN_SO_BIN;
    process.env.SUPEROPEN_SO_BIN = "/nonexistent/superopen/so";
    try {
      const result = await soJSON(["graph", "layout"], { timeoutMs: 1000 });
      expect(result.ok).toBe(false);
      expect(result.error).toContain("Could not start so:");
    } finally {
      if (previous === undefined) delete process.env.SUPEROPEN_SO_BIN;
      else process.env.SUPEROPEN_SO_BIN = previous;
    }
  });
});
