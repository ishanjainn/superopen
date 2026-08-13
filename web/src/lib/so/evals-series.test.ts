import { describe, expect, it } from "vitest";
import { buildDailySeries, type EvalRun } from "./evals";

function run(at: string, badge: string = "good"): EvalRun {
  return {
    id: at,
    session_id: at,
    at,
    badge,
    notes: [],
    source: "session.json",
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

  it("counts ok badges as ok, not poor", () => {
    const series = buildDailySeries([
      run("2026-08-01T12:00:00Z", "ok"),
      run("2026-08-01T13:00:00Z", "good"),
      run("2026-08-01T14:00:00Z", "poor"),
    ]);

    expect(series).toHaveLength(1);
    expect(series[0]).toMatchObject({
      runs: 3,
      good: 1,
      ok: 1,
      poor: 1,
      unknown: 0,
      pass_rate: 1 / 3,
    });
  });
});
