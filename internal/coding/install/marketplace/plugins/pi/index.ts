// Superopen Pi extension - forwards lifecycle events to `so coding hook`.
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
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
        timeout: 8000,
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
    spawnSync(soBin(), ["sessions", "finalize"], {
      timeout: 30000,
      stdio: "ignore",
    });
  } catch {
    /* ignore */
  }
}

export default function (pi: ExtensionAPI) {
  let lastSessionId: string | undefined;
  let lastCwd: string | undefined;
  let lastSessionFile: string | undefined;

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
      true,
    );
    const inj = parseInject(out);
    if (inj) {
      (pi as any).__soInject = inj;
    }
  });

  pi.on("before_agent_start", async (event, ctx) => {
    lastCwd = ctx.cwd;
    lastSessionFile = ctx.sessionManager.getSessionFile();
    lastSessionId = ctx.sessionManager.getSessionId?.() || lastSessionId;
    const out = fire(
      "before_agent_start",
      {
        type: "before_agent_start",
        cwd: ctx.cwd,
        session_file: lastSessionFile,
        session_id: lastSessionId,
        prompt: (event as { prompt?: string }).prompt,
      },
      true,
    );
    const inj = parseInject(out) || (pi as any).__soInject;
    if (inj) {
      (pi as any).__soInject = null;
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
    lastSessionId = ctx.sessionManager.getSessionId?.() || lastSessionId;
    fire("message_end", {
      type: "message_end",
      cwd: ctx.cwd,
      session_file: ctx.sessionManager.getSessionFile(),
      session_id: lastSessionId,
      role: message.role,
      text: typeof message.content === "string" ? message.content : undefined,
      model: message.model,
      usage: message.usage,
    });
  });

  pi.on("tool_execution_start", async (event, ctx) => {
    const args = (event as { args?: Record<string, unknown>; input?: Record<string, unknown> }).args
      || (event as { input?: Record<string, unknown> }).input
      || {};
    const out = fire(
      "tool.execute.before",
      {
        type: "tool.execute.before",
        cwd: ctx.cwd,
        session_file: ctx.sessionManager.getSessionFile(),
        session_id: ctx.sessionManager.getSessionId?.() || lastSessionId,
        tool_name: (event as { toolName?: string }).toolName,
        toolName: (event as { toolName?: string }).toolName,
        command: args.command || args.cmd,
        path: args.path || args.file_path,
      },
      true,
    );
    const deny = parseDeny(out);
    if (deny) throw new Error(deny);
  });

  pi.on("tool_execution_end", async (event, ctx) => {
    fire("tool_execution_end", {
      type: "tool_execution_end",
      cwd: ctx.cwd,
      session_file: ctx.sessionManager.getSessionFile(),
      toolName: (event as { toolName?: string }).toolName,
      toolCallId: (event as { toolCallId?: string }).toolCallId,
      isError: (event as { isError?: boolean }).isError,
      result: (event as { result?: string }).result,
    });
  });

  pi.on("agent_end", async (_event, ctx) => {
    lastSessionId = ctx.sessionManager.getSessionId?.() || lastSessionId;
    fire(
      "agent_end",
      {
        type: "agent_end",
        cwd: ctx.cwd,
        session_file: ctx.sessionManager.getSessionFile(),
        session_id: lastSessionId,
      },
      true,
    );
    runFinalize();
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
      true,
    );
    runFinalize();
  });
}
