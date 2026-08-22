import { describe, expect, it } from "vitest";
import { soJSONRows } from "./exec";

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
