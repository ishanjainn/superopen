// Superopen OpenCode plugin - forwards lifecycle events to `so coding hook`.
// Event coverage for OpenCode plugin hooks.
import type { Plugin } from "@opencode-ai/plugin";
import { spawn, spawnSync } from "node:child_process";

function soBin(): string {
  return process.env.SUPEROPEN_SO_BIN?.trim() || "so";
}

function fire(event: string, payload: Record<string, unknown>, sync = false) {
  const args = ["coding", "hook", "--vendor=opencode", `--event=${event}`];
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
    /* never block OpenCode */
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

export const SuperopenPlugin: Plugin = async ({ directory }) => {
  let pendingInject: string | null = null;
  return {
    event: async ({ event }) => {
      const type = (event as { type?: string })?.type || "event";
      const props = (event as { properties?: Record<string, unknown> })?.properties || {};
      const payload = { ...props, type, cwd: directory, event: type };
      const sync = type === "session.created" || type === "session.deleted" || type === "session.idle";
      const out = fire(type, payload, sync);
      if (type === "session.created") {
        const inj = parseInject(out);
        if (inj) pendingInject = inj;
      }
      if (type === "session.idle" || type === "session.deleted") {
        fire("session.end", payload, true);
        runFinalize();
      }
    },
    "chat.message": async (input, output) => {
      const msg = (output as { message?: Record<string, unknown> })?.message || {};
      fire("message.updated", {
        cwd: directory,
        session_id: (input as { sessionID?: string })?.sessionID,
        role: msg.role,
        text: typeof msg.content === "string" ? msg.content : undefined,
        tokens: msg.tokens,
        cost: msg.cost,
        model: (msg.model as { id?: string })?.id,
        type: "message.updated",
      });
    },
    "tool.execute.before": async (input) => {
      const args = (input as { args?: Record<string, unknown> })?.args || {};
      const out = fire(
        "tool.execute.before",
        {
          cwd: directory,
          session_id: (input as { sessionID?: string })?.sessionID,
          tool_name: (input as { tool?: string })?.tool,
          tool_use_id: (input as { callID?: string })?.callID,
          command: args.command || args.cmd,
          path: args.path || args.file_path,
          type: "tool.execute.before",
        },
        true
      );
      const deny = parseDeny(out);
      if (deny) throw new Error(deny);
    },
    "tool.execute.after": async (input, output) => {
      fire("tool.execute.after", {
        cwd: directory,
        session_id: (input as { sessionID?: string })?.sessionID,
        tool_name: (input as { tool?: string })?.tool,
        tool_use_id: (input as { callID?: string })?.callID,
        tool_result: typeof output === "string" ? output : JSON.stringify(output ?? {}),
        type: "tool.execute.after",
      });
    },
    "experimental.chat.system.transform": async (_input, output) => {
      if (!pendingInject) return;
      const o = output as { system?: string };
      if (typeof o.system === "string") {
        o.system = pendingInject + "\n\n" + o.system;
      }
      pendingInject = null;
    },
    dispose: async () => {
      fire("dispose", { cwd: directory, type: "dispose", event: "dispose" }, true);
      fire("session.end", { cwd: directory, type: "session.end", event: "session.end" }, true);
      runFinalize();
    },
  };
};

export default SuperopenPlugin;
