import { describe, expect, it } from "vitest";
import { chartDateLabel, chartFullDate, compactChartNumber } from "./charts";

describe("dashboard chart formatting", () => {
  it("formats compact axis values without losing small values", () => {
    expect(compactChartNumber(2_600_000)).toMatch(/2\.6M/i);
    expect(compactChartNumber(2.07)).toBe("2.07");
  });

  it("formats short axis dates and full tooltip dates", () => {
    expect(chartDateLabel("2026-08-07")).not.toBe("2026-08-07");
    expect(chartFullDate("2026-08-07")).toContain("2026");
  });
});
