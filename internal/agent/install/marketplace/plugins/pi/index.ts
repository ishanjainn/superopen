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

function additionalContext(stdout: string): string {
  try {
    const line = stdout.trim().split(/\n/).find((l) => l.includes("additionalContext"));
    if (!line) return "";
    const parsed = JSON.parse(line) as { additionalContext?: string };
    return typeof parsed.additionalContext === "string" ? parsed.additionalContext : "";
  } catch {
    return "";
  }
}

function shellSafeNudge(text: string): string {
  return text.replace(/`/g, "").replace(/\$\(/g, "(").replace(/"/g, "'");
}

function isExploreTool(name: string): boolean {
  const n = name.toLowerCase();
  if (n.startsWith("graph_")) return false;
  return n === "bash" || n === "shell" || n === "grep" || n === "glob" || n === "read" || n === "readfile";
}

function prependBashNudge(command: string, nudge: string): string {
  return "echo " + JSON.stringify(shellSafeNudge(nudge)) + " ; " + command;
}

function runSoGraph(args: string[], cwd?: string): string {
  try {
    const r = spawnSync(soBin(), ["graph", ...args], {
      cwd: cwd || process.cwd(),
      encoding: "utf8",
      timeout: 60000,
      stdio: ["ignore", "pipe", "pipe"],
      env: process.env,
    });
    const out = typeof r.stdout === "string" ? r.stdout : "";
    const err = typeof r.stderr === "string" ? r.stderr : "";
    if (r.status !== 0) {
      return (out + "\n" + err).trim() || `so graph failed (exit ${r.status})`;
    }
    return out.trim() || err.trim();
  } catch (e) {
    return `so graph error: ${e instanceof Error ? e.message : String(e)}`;
  }
}

function registerGraphTools(pi: ExtensionAPI, getCwd: () => string | undefined) {
  const register = (pi as { registerTool?: (tool: Record<string, unknown>) => void }).registerTool;
  if (typeof register !== "function") {
    return;
  }
  const text = (s: string) => [{ type: "text", text: s }];
  register({
    name: "graph_search",
    label: "Superopen graph search",
    description:
      "PRIMARY symbol lookup in the Superopen native code graph. Prefer over Grep/Glob for structural finds.",
    parameters: {
      type: "object",
      properties: { query: { type: "string", description: "Symbol or search query" } },
      required: ["query"],
    },
    async execute(_id: string, params: { query?: string }) {
      return { content: text(runSoGraph(["search", String(params.query || "")], getCwd())) };
    },
  });
  register({
    name: "graph_trace",
    label: "Superopen graph trace",
    description: "PRIMARY callers/callees / path trace. Prefer over manual Read chains.",
    parameters: {
      type: "object",
      properties: {
        start: { type: "string", description: "Qualified name or symbol" },
        direction: { type: "string", description: "outgoing|incoming|both" },
      },
      required: ["start"],
    },
    async execute(_id: string, params: { start?: string; direction?: string }) {
      const args = ["trace", String(params.start || "")];
      if (params.direction) args.push("--direction", String(params.direction));
      return { content: text(runSoGraph(args, getCwd())) };
    },
  });
  register({
    name: "graph_snippet",
    label: "Superopen graph snippet",
    description: "PRIMARY symbol body from the graph. Prefer over Read when you have a name.",
    parameters: {
      type: "object",
      properties: { qualified_name: { type: "string" } },
      required: ["qualified_name"],
    },
    async execute(_id: string, params: { qualified_name?: string }) {
      return { content: text(runSoGraph(["snippet", String(params.qualified_name || "")], getCwd())) };
    },
  });
  register({
    name: "graph_query",
    label: "Superopen graph query",
    description: "PRIMARY natural-language structural question against the native graph.",
    parameters: {
      type: "object",
      properties: { question: { type: "string" } },
      required: ["question"],
    },
    async execute(_id: string, params: { question?: string }) {
      return { content: text(runSoGraph(["query", String(params.question || "")], getCwd())) };
    },
  });
  register({
    name: "graph_architecture",
    label: "Superopen graph architecture",
    description: "PRIMARY architecture overview from the native graph.",
    parameters: { type: "object", properties: {} },
    async execute() {
      return { content: text(runSoGraph(["architecture"], getCwd())) };
    },
  });
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
  let reminded = false;

  registerGraphTools(pi, () => lastCwd);

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
    fire(
      "session_start",
      {
        type: "session_start",
        cwd: ctx.cwd,
        session_file: lastSessionFile,
        session_id: lastSessionId,
      },
      true
    );
    try {
      spawn(soBin(), ["graph", "refresh", "--detach"], { stdio: "ignore" });
    } catch {
      /* ignore */
    }
  });

  pi.on("before_agent_start", async (event, ctx) => {
    lastCwd = ctx.cwd;
    lastSessionFile = ctx.sessionManager.getSessionFile();
    fire(
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
    const toolName = String((event as { toolName?: string }).toolName || "");
    const stdout = fire(
      "tool.execute.before",
      {
        type: "tool.execute.before",
        cwd: ctx.cwd,
        session_file: ctx.sessionManager.getSessionFile(),
        session_id: sid(ctx),
        tool_name: toolName,
        toolName,
        toolCallId: (event as { toolCallId?: string }).toolCallId,
        command: args.command || args.cmd,
        path: args.path || args.file_path,
        args,
        input: args,
      },
      true
    );
    if (reminded || toolName.toLowerCase().startsWith("graph_")) return;
    const nudge = additionalContext(stdout);
    const cmd = typeof args.command === "string" ? args.command : typeof args.cmd === "string" ? args.cmd : "";
    if (nudge && isExploreTool(toolName) && cmd) {
      const rewritten = prependBashNudge(cmd, nudge);
      if (typeof args.command === "string") args.command = rewritten;
      else if (typeof args.cmd === "string") args.cmd = rewritten;
      const evArgs = (event as { args?: Record<string, unknown> }).args;
      if (evArgs && typeof evArgs === "object") {
        if (typeof evArgs.command === "string") evArgs.command = rewritten;
        else if (typeof evArgs.cmd === "string") evArgs.cmd = rewritten;
      }
      reminded = true;
    }
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
