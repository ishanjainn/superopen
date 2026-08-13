// Superopen Pi extension.
// Telemetry uses Superopen conventions (so coding hook → coding_agent.* / gen_ai.*).
import type { ExtensionAPI } from "@mariozechner/pi-coding-agent";
import { spawn, spawnSync } from "node:child_process";

function soBin(): string {
  return process.env.SUPEROPEN_SO_BIN?.trim() || "so";
}

function fire(event: string, payload: Record<string, unknown>, sync = false): string {
  const args = ["coding", "hook", "--vendor=pi", `--event=${event}`];
  const bin = soBin();
  const input = JSON.stringify(payload) + "\n";
  try {
    if (sync) {
      const r = spawnSync(bin, args, {
        input,
        encoding: "utf8",
        timeout: 12000,
        stdio: ["pipe", "pipe", "ignore"],
      });
      return typeof r.stdout === "string" ? r.stdout : "";
    }
    const child = spawn(bin, args, { stdio: ["pipe", "ignore", "ignore"] });
    child.stdin?.end(input);
  } catch {
    /* never block Pi */
  }
  return "";
}

function parseInject(stdout: string): string | null {
  if (!stdout) return null;
  for (const line of stdout.split("\n")) {
    const t = line.trim();
    if (!t.startsWith("{")) continue;
    try {
      const j = JSON.parse(t) as { inject_context?: string; additional_context?: string };
      const v = j.inject_context || j.additional_context;
      if (typeof v === "string" && v.length > 0) return v;
    } catch {
      /* ignore */
    }
  }
  return null;
}

function parseDeny(stdout: string): string | null {
  if (!stdout) return null;
  for (const line of stdout.split("\n")) {
    const t = line.trim();
    if (!t.startsWith("{")) continue;
    try {
      const j = JSON.parse(t) as {
        decision?: string;
        permission?: string;
        reason?: string;
        userMessage?: string;
      };
      if (j.decision === "deny" || j.permission === "deny") {
        return j.reason || j.userMessage || "blocked by Superopen guardrails";
      }
    } catch {
      /* ignore */
    }
  }
  return null;
}

function runFinalize() {
  try {
    spawnSync(soBin(), ["sessions", "finalize", "--detach"], {
      timeout: 5000,
      stdio: "ignore",
    });
  } catch {
    /* ignore */
  }
}

function textFromContent(content: unknown): string {
  if (typeof content === "string") return content;
  if (!Array.isArray(content)) return "";
  const out: string[] = [];
  for (const block of content) {
    if (!block || typeof block !== "object") continue;
    const b = block as Record<string, unknown>;
    if (b.type === "text" && typeof b.text === "string") out.push(b.text);
    if (b.type === "thinking" && typeof b.thinking === "string") out.push(b.thinking);
    if (typeof b.text === "string" && !b.type) out.push(b.text);
  }
  return out.join("\n").trim();
}

function thinkingFromContent(content: unknown): string {
  if (!Array.isArray(content)) return "";
  const out: string[] = [];
  for (const block of content) {
    if (!block || typeof block !== "object") continue;
    const b = block as Record<string, unknown>;
    if (b.type === "thinking") {
      if (typeof b.thinking === "string") out.push(b.thinking);
      else if (typeof b.text === "string") out.push(b.text);
    }
  }
  return out.join("\n").trim();
}

export default function (pi: ExtensionAPI) {
  let lastSessionId: string | undefined;
  let lastCwd: string | undefined;
  let lastSessionFile: string | undefined;
  let pendingInject: string | null = null;

  function sid(ctx: {
    sessionManager: {
      getSessionId?: () => string | undefined;
      getSessionFile?: () => string | undefined;
    };
    cwd?: string;
  }): string | undefined {
    try {
      const v = ctx.sessionManager.getSessionId?.() || lastSessionId;
      if (v) lastSessionId = v;
    } catch {
      /* ignore */
    }
    return lastSessionId;
  }

  pi.on("session_start", async (_event, ctx) => {
    lastCwd = ctx.cwd;
    lastSessionFile = ctx.sessionManager.getSessionFile();
    lastSessionId = ctx.sessionManager.getSessionId?.() || undefined;
    const out = fire(
      "session_start",
      {
        type: "session_start",
        cwd: ctx.cwd,
        session_file: lastSessionFile,
        session_id: lastSessionId,
      },
      true
    );
    pendingInject = parseInject(out);
  });

  pi.on("before_agent_start", async (event, ctx) => {
    lastCwd = ctx.cwd;
    lastSessionFile = ctx.sessionManager.getSessionFile();
    const out = fire(
      "before_agent_start",
      {
        type: "before_agent_start",
        cwd: ctx.cwd,
        session_file: lastSessionFile,
        session_id: sid(ctx),
        prompt: (event as { prompt?: string }).prompt,
      },
      true
    );
    const inj = parseInject(out) || pendingInject;
    pendingInject = null;
    if (inj) {
      return {
        message: {
          customType: "superopen-context",
          content: inj,
          display: false,
        },
      };
    }
  });

  pi.on("message_end", async (event, ctx) => {
    const message = (event as { message?: Record<string, unknown> }).message || {};
    const role = typeof message.role === "string" ? message.role : "";
    fire("message_end", {
      type: "message_end",
      cwd: ctx.cwd,
      session_file: ctx.sessionManager.getSessionFile(),
      session_id: sid(ctx),
      role,
      text: textFromContent(message.content),
      thinking: thinkingFromContent(message.content),
      content: message.content,
      model: message.model,
      usage: message.usage,
    });
  });

  pi.on("tool_execution_start", async (event, ctx) => {
    const args =
      (event as { args?: Record<string, unknown>; input?: Record<string, unknown> }).args ||
      (event as { input?: Record<string, unknown> }).input ||
      {};
    const out = fire(
      "tool.execute.before",
      {
        type: "tool.execute.before",
        cwd: ctx.cwd,
        session_file: ctx.sessionManager.getSessionFile(),
        session_id: sid(ctx),
        tool_name: (event as { toolName?: string }).toolName,
        toolName: (event as { toolName?: string }).toolName,
        toolCallId: (event as { toolCallId?: string }).toolCallId,
        command: args.command || args.cmd,
        path: args.path || args.file_path,
        args,
        input: args,
      },
      true
    );
    const deny = parseDeny(out);
    if (deny) throw new Error(deny);
  });

  pi.on("tool_execution_end", async (event, ctx) => {
    const args =
      (event as { args?: Record<string, unknown>; input?: Record<string, unknown> }).args ||
      (event as { input?: Record<string, unknown> }).input ||
      {};
    fire("tool_execution_end", {
      type: "tool_execution_end",
      cwd: ctx.cwd,
      session_file: ctx.sessionManager.getSessionFile(),
      session_id: sid(ctx),
      toolName: (event as { toolName?: string }).toolName,
      tool_name: (event as { toolName?: string }).toolName,
      toolCallId: (event as { toolCallId?: string }).toolCallId,
      isError: (event as { isError?: boolean }).isError,
      result: (event as { result?: string }).result,
      args,
      input: args,
    });
  });

  // Primary turn export: assistant content blocks + toolResults.
  pi.on("turn_end", async (event, ctx) => {
    const message = (event as { message?: Record<string, unknown> }).message || {};
    const toolResults = (event as { toolResults?: unknown[] }).toolResults || [];
    const sessionId = sid(ctx);
    fire(
      "turn_end",
      {
        type: "turn_end",
        cwd: ctx.cwd,
        session_file: ctx.sessionManager.getSessionFile(),
        session_id: sessionId,
        role: "assistant",
        text: textFromContent(message.content),
        thinking: thinkingFromContent(message.content),
        content: message.content,
        model: message.model || (message as { modelId?: string }).modelId,
        usage: message.usage,
        toolResults,
      },
      true
    );
    for (const tr of toolResults) {
      if (!tr || typeof tr !== "object") continue;
      const t = tr as Record<string, unknown>;
      fire("tool_execution_end", {
        type: "tool_execution_end",
        cwd: ctx.cwd,
        session_id: sessionId,
        toolName: t.toolName,
        tool_name: t.toolName,
        toolCallId: t.toolCallId,
        isError: t.isError,
        result: textFromContent(t.content) || (typeof t.result === "string" ? t.result : undefined),
      });
    }
  });

	pi.on("agent_end", async (_event, ctx) => {
    fire(
      "agent_end",
      {
        type: "agent_end",
        cwd: ctx.cwd,
        session_file: ctx.sessionManager.getSessionFile(),
        session_id: sid(ctx),
      },
      true
    );
    // agent_end is a turn boundary (one user→agent loop), not chat close.
  });

  pi.on("session_shutdown", async () => {
    fire(
      "session_shutdown",
      {
        type: "session_shutdown",
        cwd: lastCwd,
        session_file: lastSessionFile,
        session_id: lastSessionId,
      },
      true
    );
    runFinalize();
  });
}
