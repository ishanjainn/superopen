import { describe, expect, it } from "vitest";
import { buildDailySeries, type EvalRun } from "./evals";

function run(at: string, badge: string = "good"): EvalRun {
  return {
    id: at,
    session_id: at,
    at,
    badge,
    notes: [],
    source: "history",
    scope: "snapshot",
    failure_points: [],
  };
}

describe("evaluation daily series", () => {
  it("fills calendar gaps without inventing a pass rate", () => {
    const series = buildDailySeries([
      run("2026-08-01T12:00:00Z"),
      run("2026-08-03T12:00:00Z", "poor"),
    ]);

    expect(series.map((point) => point.date)).toEqual([
      "2026-08-01",
      "2026-08-02",
      "2026-08-03",
    ]);
    expect(series[1]).toMatchObject({ runs: 0, pass_rate: null });
    expect(series[2]).toMatchObject({ runs: 1, pass_rate: 0 });
  });

  it("caps a long history to thirty calendar days", () => {
    const series = buildDailySeries([
      run("2026-01-01T12:00:00Z"),
      run("2026-08-07T12:00:00Z"),
    ]);

    expect(series).toHaveLength(30);
    expect(series.at(-1)?.date).toBe("2026-08-07");
  });
});
