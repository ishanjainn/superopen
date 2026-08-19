import { describe, expect, it } from "vitest";
import {
  bloomIntensityScale,
  edgeIntensityScale,
  nodeBoostScale,
  nodeGlowBoost,
} from "./density";

describe("stellar graph density compensation", () => {
  it("keeps ordinary graphs at full strength", () => {
    expect(edgeIntensityScale(2500)).toBe(1);
    expect(bloomIntensityScale(25_000)).toBe(1);
    expect(nodeBoostScale(25_000)).toBe(1);
  });

  it("dims dense additive edge fields without hiding them", () => {
    expect(edgeIntensityScale(10_000)).toBeCloseTo(0.5);
    expect(edgeIntensityScale(10_000_000)).toBeGreaterThanOrEqual(0.05);
  });

  it("boosts blue hubs more than already-luminous white stars", () => {
    expect(nodeGlowBoost(0.5, 0.63, 1)).toBeGreaterThan(
      nodeGlowBoost(1, 1, 1),
    );
  });
});
