// Superopen OpenCode plugin.
// Telemetry uses Superopen conventions (so coding hook → coding_agent.* / gen_ai.*).
import type { Plugin } from "@opencode-ai/plugin";
import { spawn, spawnSync } from "node:child_process";

function soBin(): string {
  return process.env.SUPEROPEN_SO_BIN?.trim() || "so";
}

function fire(event: string, payload: Record<string, unknown>, sync = false): string {
  const args = ["coding", "hook", "--vendor=opencode", `--event=${event}`];
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
    spawnSync(soBin(), ["sessions", "finalize", "--detach"], {
      timeout: 5000,
      stdio: "ignore",
    });
  } catch {
    /* ignore */
  }
}

function isTerminalMessage(info: Record<string, unknown> | undefined): boolean {
  if (!info) return false;
  if (info.error) return true;
  const time = info.time as { completed?: unknown } | undefined;
  return time?.completed != null;
}

function textFromParts(parts: unknown): string {
  if (!Array.isArray(parts)) return "";
  const out: string[] = [];
  for (const p of parts) {
    if (!p || typeof p !== "object") continue;
    const part = p as Record<string, unknown>;
    if (part.type === "text" && typeof part.text === "string") out.push(part.text);
    if (part.type === "reasoning" && typeof part.text === "string") out.push(part.text);
  }
  return out.join("\n").trim();
}

function toolsFromParts(parts: unknown): Array<Record<string, unknown>> {
  if (!Array.isArray(parts)) return [];
  const out: Array<Record<string, unknown>> = [];
  for (const p of parts) {
    if (!p || typeof p !== "object") continue;
    const part = p as Record<string, unknown>;
    if (part.type !== "tool") continue;
    const state = (part.state as Record<string, unknown>) || {};
    out.push({
      tool_name: part.tool,
      tool_use_id: part.callID,
      tool_input: state.input,
      tool_result: typeof state.output === "string" ? state.output : JSON.stringify(state.output ?? {}),
      errored: state.status === "error" || Boolean(state.error),
    });
  }
  return out;
}

export const SuperopenPlugin: Plugin = async ({ client, directory }) => {
  let pendingInject: string | null = null;
  let lastSid: string | null = null;

  function rememberSid(...vals: unknown[]): string | null {
    for (const v of vals) {
      if (typeof v === "string" && v && v !== "unknown") {
        lastSid = v;
        return v;
      }
      if (v && typeof v === "object") {
        const o = v as Record<string, unknown>;
        for (const k of ["sessionID", "session_id", "sessionId", "id"]) {
          const s = o[k];
          if (typeof s === "string" && s && s !== "unknown") {
            lastSid = s;
            return s;
          }
        }
      }
    }
    return lastSid;
  }

  return {
    event: async ({ event }) => {
      const type = (event as { type?: string })?.type || "event";
      const props =
        ((event as { properties?: Record<string, unknown> })?.properties as Record<
          string,
          unknown
        >) || {};
      const info = (props.info as Record<string, unknown>) || {};
      const sid = rememberSid(props.sessionID, info.sessionID, info.id, props);

      if (type === "session.created" || type === "session.updated") {
        rememberSid(info.id, props);
        fire(type, {
          type,
          event: type,
          cwd: directory,
          session_id: lastSid,
          sessionID: lastSid,
          title: info.title,
          model: info.model,
        });
        if (type === "session.created") {
          const out = fire(
            "session.created",
            { type: "session.created", cwd: directory, session_id: lastSid, sessionID: lastSid },
            true
          );
          const inj = parseInject(out);
          if (inj) pendingInject = inj;
        }
        return;
      }

      // Assistant turn boundary: terminal message.updated + hydrate parts.
      if (type === "message.updated") {
        if (!isTerminalMessage(info)) return;
        const sessionID = rememberSid(info.sessionID, sid);
        const messageID = typeof info.id === "string" ? info.id : "";
        if (!sessionID || !messageID) return;

        let parts: unknown[] = [];
        let role = typeof info.role === "string" ? info.role : "";
        let model =
          (info.model as { id?: string } | undefined)?.id ||
          (typeof info.modelID === "string" ? info.modelID : undefined);
        let tokens = info.tokens;
        let cost = typeof info.cost === "number" ? info.cost : undefined;

        try {
          const response = await client.session.message({
            path: { id: sessionID, messageID },
          });
          const data = (response as { data?: { info?: Record<string, unknown>; parts?: unknown[] } })
            ?.data;
          if (data?.info) {
            role = typeof data.info.role === "string" ? data.info.role : role;
            model =
              (data.info.model as { id?: string } | undefined)?.id ||
              (typeof data.info.modelID === "string" ? data.info.modelID : model);
            tokens = data.info.tokens ?? tokens;
            cost = typeof data.info.cost === "number" ? data.info.cost : cost;
          }
          if (Array.isArray(data?.parts)) parts = data.parts;
        } catch {
          /* fail-soft: still emit whatever we have */
        }

        if (role && role !== "assistant") return;

        const text = textFromParts(parts);
        const tools = toolsFromParts(parts);
        fire(
          "message.updated",
          {
            type: "message.updated",
            cwd: directory,
            session_id: sessionID,
            sessionID,
            role: "assistant",
            text,
            model,
            tokens,
            cost,
            parts,
            tools,
          },
          true
        );
        for (const t of tools) {
          fire("tool.execute.after", {
            type: "tool.execute.after",
            cwd: directory,
            session_id: sessionID,
            sessionID,
            tool_name: t.tool_name,
            tool_use_id: t.tool_use_id,
            tool_input: t.tool_input,
            tool_result: t.tool_result,
            errored: t.errored,
          });
        }
        return;
      }

      if (type === "session.idle" || type === "session.deleted") {
        fire(
          "session.end",
          { type: "session.end", cwd: directory, session_id: lastSid, sessionID: lastSid },
          true
        );
        runFinalize();
      }
    },

    // User prompt buffer: parts text, not message.content.
    "chat.message": async (input, output) => {
      const sid = rememberSid((input as { sessionID?: string })?.sessionID);
      const parts = (output as { parts?: unknown[] })?.parts;
      const text = textFromParts(parts);
      if (!text || !sid) return;
      fire("message.updated", {
        type: "message.updated",
        cwd: directory,
        session_id: sid,
        sessionID: sid,
        role: "user",
        text,
      });
    },

    "tool.execute.before": async (input, output) => {
      const sid = rememberSid((input as { sessionID?: string })?.sessionID);
      const args = (output as { args?: unknown })?.args ?? (input as { args?: unknown })?.args;
      const out = fire(
        "tool.execute.before",
        {
          type: "tool.execute.before",
          cwd: directory,
          session_id: sid,
          sessionID: sid,
          tool_name: (input as { tool?: string })?.tool,
          tool_use_id: (input as { callID?: string })?.callID,
          tool_input: args,
          command:
            args && typeof args === "object"
              ? (args as { command?: string; cmd?: string }).command ||
                (args as { command?: string; cmd?: string }).cmd
              : undefined,
          path:
            args && typeof args === "object"
              ? (args as { path?: string; file_path?: string }).path ||
                (args as { path?: string; file_path?: string }).file_path
              : undefined,
        },
        true
      );
      const deny = parseDeny(out);
      if (deny) throw new Error(deny);
    },

    "tool.execute.after": async (input, output) => {
      const sid = rememberSid((input as { sessionID?: string })?.sessionID);
      const result =
        typeof (output as { output?: unknown })?.output === "string"
          ? (output as { output: string }).output
          : JSON.stringify(output ?? {});
      fire("tool.execute.after", {
        type: "tool.execute.after",
        cwd: directory,
        session_id: sid,
        sessionID: sid,
        tool_name: (input as { tool?: string })?.tool,
        tool_use_id: (input as { callID?: string })?.callID,
        tool_input: (input as { args?: unknown })?.args,
        tool_result: result,
      });
    },

    "experimental.chat.system.transform": async (_input, output) => {
      if (!pendingInject) return;
      const o = output as { system?: string | string[] };
      if (Array.isArray(o.system)) {
        o.system = [pendingInject, ...o.system];
      } else if (typeof o.system === "string") {
        o.system = pendingInject + "\n\n" + o.system;
      }
      pendingInject = null;
    },

    dispose: async () => {
      fire(
        "session.end",
        { cwd: directory, type: "session.end", session_id: lastSid, sessionID: lastSid },
        true
      );
      runFinalize();
    },
  };
};

export default SuperopenPlugin;
