import { describe, expect, it } from "vitest";
import {
  emptySessionQuery,
  incompleteSessionToken,
  parseSessionQuery,
  serializeSessionQuery,
  userMatches,
  vendorMatchesAgent,
} from "./session-query";

describe("parseSessionQuery", () => {
  it("returns empty query for blank input", () => {
    expect(parseSessionQuery("")).toEqual(emptySessionQuery());
    expect(parseSessionQuery("   ")).toEqual(emptySessionQuery());
  });

  it("parses from/agent/model/file/tool tokens and free text", () => {
    const q = parseSessionQuery(
      "from:@alice agent:cursor model:sonnet file:trace.ts tool:Bash fix map"
    );
    expect(q.user).toBe("alice");
    expect(q.agent).toBe("cursor");
    expect(q.model).toBe("sonnet");
    expect(q.file).toBe("trace.ts");
    expect(q.tool).toBe("Bash");
    expect(q.text).toBe("fix map");
  });

  it("accepts user: and vendor: aliases", () => {
    const q = parseSessionQuery("user:bob vendor:claude");
    expect(q.user).toBe("bob");
    expect(q.agent).toBe("claude");
  });
});

describe("serializeSessionQuery", () => {
  it("round-trips structured filters", () => {
    const raw = serializeSessionQuery({
      text: "hello",
      user: "alice",
      agent: "codex",
      model: "gpt",
      file: "a.ts",
      tool: "Read",
    });
    expect(raw).toContain("from:@alice");
    expect(raw).toContain("agent:codex");
    expect(raw).toContain("model:gpt");
    expect(raw).toContain("file:a.ts");
    expect(raw).toContain("tool:Read");
    expect(raw).toContain("hello");
    const again = parseSessionQuery(raw);
    expect(again.user).toBe("alice");
    expect(again.agent).toBe("codex");
    expect(again.text).toBe("hello");
  });
});

describe("incompleteSessionToken", () => {
  it("detects trailing incomplete tokens", () => {
    expect(incompleteSessionToken("from:@ish")).toMatchObject({
      key: "from",
      prefix: "ish",
    });
    expect(incompleteSessionToken("x tool:Ba")).toMatchObject({
      key: "tool",
      prefix: "ba",
    });
    expect(incompleteSessionToken("file:")).toMatchObject({
      key: "file",
      prefix: "",
    });
  });

  it("returns null when no incomplete token", () => {
    expect(incompleteSessionToken("hello world")).toBeNull();
    expect(incompleteSessionToken("agent:cursor ")).toBeNull();
  });
});

describe("vendorMatchesAgent / userMatches", () => {
  it("matches vendor aliases", () => {
    expect(vendorMatchesAgent("claude-code", "claude")).toBe(true);
    expect(vendorMatchesAgent("cursor", "codex")).toBe(false);
    expect(vendorMatchesAgent(undefined, "")).toBe(true);
  });

  it("matches users by local-part", () => {
    expect(userMatches("alice@example.com", "alice")).toBe(true);
    expect(userMatches("bob", "alice")).toBe(false);
    expect(userMatches(undefined, "")).toBe(true);
  });
});
