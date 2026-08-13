import { describe, expect, it } from "vitest";
import {
  countTurnsFromSpans,
  mergeTraceSpans,
  spansHaveActivity,
} from "./sessions";

describe("mergeTraceSpans", () => {
  it("adds live spans that are newer than a materialized transcript", () => {
    const transcript = [
      {
        trace_id: "trace-1",
        span_id: "span-1",
        name: "coding_agent.llm.turn",
        start_time_unix_nano: 10,
      },
    ];
    const live = [
      ...transcript,
      {
        trace_id: "trace-1",
        span_id: "span-2",
        name: "coding_agent.llm.turn",
        start_time_unix_nano: 20,
      },
    ];

    expect(mergeTraceSpans(transcript, live).map((span) => span.span_id)).toEqual([
      "span-1",
      "span-2",
    ]);
  });

  it("does not collapse portable transcript turns without span ids", () => {
    const turns = [
      { role: "user", text: "Question", timestamp: 10 },
      { role: "assistant", text: "Answer", timestamp: 20 },
    ];

    expect(mergeTraceSpans(turns)).toEqual(turns);
  });
});

describe("file-backed active sessions", () => {
  it("counts a prompt hook event as a visible turn before session end", () => {
    expect(
      countTurnsFromSpans([
        {
          name: "coding_agent.user_prompt.submit",
          attributes: { "coding_agent.session.id": "session-1" },
        },
      ])
    ).toBe(1);
  });

  it("keeps a live vendor session visible when the vendor omits prompt spans", () => {
    const spans: Parameters<typeof spansHaveActivity>[0] = [
      {
        name: "coding_agent.llm.turn",
        attributes: {
          "gen_ai.output.messages": '[{"role":"assistant"}]',
        },
      },
      {
        name: "coding_agent.tool.call",
        attributes: {
          "gen_ai.tool.name": "Write",
          "code.file.path": "internal/session/store.go",
        },
      },
    ];

    expect(countTurnsFromSpans(spans)).toBe(1);
    expect(spansHaveActivity(spans)).toBe(true);
  });

  it("still treats lifecycle-only telemetry as empty", () => {
    expect(
      spansHaveActivity([
        {
          name: "coding_agent.session",
          attributes: { "coding_agent.client": "cursor" },
        },
      ]),
    ).toBe(false);
  });
});
