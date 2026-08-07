import { describe, expect, it } from "vitest";
import { mergeTraceSpans } from "./sessions";

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
