import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "fs";
import { tmpdir } from "os";
import { join } from "path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

describe("evaluation dashboard aggregation", () => {
  let root: string;

  beforeEach(() => {
    root = mkdtempSync(join(tmpdir(), "so-evals-"));
    const so = join(root, ".so");
    mkdirSync(join(so, "sessions", "session-1"), { recursive: true });
    writeFileSync(
      join(so, "sessions", "session-1", "session.json"),
      JSON.stringify({
        id: "session-1",
        title: "One session",
        status: "ended",
        started_at: "2026-08-07T00:00:00Z",
        ended_at: "2026-08-07T02:30:00Z",
        tokens: 100,
        cost_usd: 0.01,
        evaluation: {
          session_id: "session-1",
          at: "2026-08-07T03:00:00Z",
          score: 0.2,
          badge: "poor",
          dimensions: { scope: 0 },
          notes: ["No tool activity recorded; insufficient signal to score."],
        },
      })
    );
    writeFileSync(
      join(so, "evals.yaml"),
      "checks:\n  - web_test\nagent_rules: []\n"
    );
    vi.stubEnv("SUPEROPEN_ROOT", root);
    vi.resetModules();
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.resetModules();
    rmSync(root, { recursive: true, force: true });
  });

  it("separates rerun executions from sessions and keeps absent evidence unknown", async () => {
    const { listEvalsDashboard } = await import("./evals");
    const dashboard = listEvalsDashboard();

    expect(dashboard.summary.executions).toBe(1);
    expect(dashboard.summary.total).toBe(1);
    expect(dashboard.summary.unknown).toBe(1);
    expect(dashboard.summary.poor).toBe(0);
    expect(dashboard.summary.pass_rate).toBeNull();
    expect(dashboard.runs).toHaveLength(1);
    expect(dashboard.runs.every((run) => run.badge === "unknown")).toBe(true);
    expect(dashboard.runs.map((run) => run.scope)).toEqual(["complete"]);
    expect(dashboard.evaluation_target).toMatchObject({
      status: "ended",
      whole_chat_evaluated: true,
    });

    const webTest = dashboard.evaluators.find((item) => item.label === "web_test");
    expect(webTest).toMatchObject({ executions: 0, pass_rate: null, trend: [] });
  });
});
