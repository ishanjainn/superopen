import { describe, expect, it } from "vitest";
import { mapRestingColors, productStage } from "./sceneUtils";

describe("productStage", () => {
  it("matches product light tokens (bg-white / ink)", () => {
    const pal = productStage(false);
    expect(pal.surface.getHexString()).toBe("ffffff");
    expect(pal.ink.getHexString()).toBe("171717");
  });

  it("matches product dark tokens (inverted --color-white, not #000)", () => {
    const pal = productStage(true);
    expect(pal.surface.getHexString()).toBe("0a0a0a");
    expect(pal.ink.getHexString()).toBe("fafafa");
  });
});

describe("mapRestingColors", () => {
  it("uses ink, not a blue-grey floor, on both themes", () => {
    const light = mapRestingColors(false);
    const dark = mapRestingColors(true);
    expect(light.unvisited.getHexString()).not.toBe("5a6375");
    expect(dark.unvisited.getHexString()).not.toBe("5b6372");
    expect(light.selected.getHexString()).toBe("171717");
    expect(dark.selected.getHexString()).toBe("fafafa");
  });
});
