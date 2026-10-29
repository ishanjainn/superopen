// OpenCode 2 server plugin. V2's event stream uses session.* records.
import { spawn, spawnSync } from "node:child_process";

function soBin() {
  return process.env.SUPEROPEN_SO_BIN?.trim() || "so";
}

function fire(event, payload, sync = false) {
  const args = ["sessions", "hook", "--vendor=opencode", `--event=${event}`];
  const input = JSON.stringify(payload) + "\n";
  try {
    if (sync) {
      const result = spawnSync(soBin(), args, { input, encoding: "utf8", timeout: 12000, stdio: ["pipe", "pipe", "ignore"] });
      return result.stdout || "";
    }
    const child = spawn(soBin(), args, { stdio: ["pipe", "ignore", "ignore"] });
    child.on("error", () => {});
    child.stdin?.end(input);
  } catch {
    // Collection must not interrupt the host.
  }
  return "";
}

function contextFrom(stdout) {
  try {
    const line = stdout.trim().split(/\n/).find((entry) => entry.includes("additionalContext"));
    return line ? JSON.parse(line).additionalContext || "" : "";
  } catch {
    return "";
  }
}

export default {
  id: "superopen",
  async setup(ctx) {
    const directory = ctx.location.directory;
    const context = new Map();
    const models = new Map();
    const usage = new Map();
    const controller = new AbortController();

    await ctx.session.hook("prompt", (event) => {
      const text = event.prompt.text;
      if (!text) return;
      const sid = event.sessionID;
      const stdout = fire("chat.message", { sessionID: sid, cwd: directory, role: "user", text }, true);
      const extra = contextFrom(stdout);
      if (extra) context.set(sid, extra);
      else context.delete(sid);
    });
    await ctx.session.hook("context", (event) => {
      const extra = context.get(event.sessionID);
      if (extra) event.system.push({ type: "text", text: extra });
    });
    await ctx.tool.hook("execute.before", (event) => {
      fire("tool.execute.before", {
        sessionID: event.sessionID, cwd: directory, tool_name: event.tool,
        tool_use_id: event.callID, tool_input: event.input,
      });
    });
    await ctx.tool.hook("execute.after", (event) => {
      fire("tool.execute.after", {
        sessionID: event.sessionID, cwd: directory, tool_name: event.tool,
        tool_use_id: event.callID, tool_input: event.input,
        tool_result: event.status === "error" ? event.error?.message : event.result,
        errored: event.status === "error",
      });
    });

    void (async () => {
      try {
        for await (const event of ctx.event.subscribe({ signal: controller.signal })) {
          const data = event.data || {};
          const sid = data.sessionID || data.info?.id;
          if (!sid) continue;
          if (event.type === "session.created") {
            fire("session.created", { sessionID: sid, cwd: directory }, true);
            try {
              spawn(soBin(), ["graph", "refresh", "--detach"], { stdio: "ignore" }).on("error", () => {});
            } catch { /* ignore */ }
          } else if (event.type === "session.step.started") {
            models.set(data.assistantMessageID, data.model?.id || "");
          } else if (event.type === "session.text.ended" && typeof data.text === "string" && data.text) {
            fire("message.updated", {
              sessionID: sid, cwd: directory, role: "assistant", text: data.text,
              model: models.get(data.assistantMessageID) || "",
              generation_id: data.assistantMessageID,
            });
          } else if (event.type === "session.step.ended") {
            fire("message.part.updated", {
              sessionID: sid, cwd: directory, part_type: "step-finish",
              model: models.get(data.assistantMessageID) || "",
              tokens: data.tokens, cost: data.cost,
              generation_id: data.assistantMessageID,
            }, true);
            models.delete(data.assistantMessageID);
          } else if (event.type === "session.usage.updated") {
            usage.set(sid, { tokens: data.tokens, cost: data.cost });
          } else if (event.type === "session.execution.succeeded" || event.type === "session.execution.failed" || event.type === "session.execution.interrupted" || event.type === "session.deleted") {
            fire("session.end", { sessionID: sid, cwd: directory, ...usage.get(sid) }, true);
            try { spawnSync(soBin(), ["sessions", "finalize", "--detach"], { timeout: 5000, stdio: "ignore" }); } catch { /* ignore */ }
            context.delete(sid);
            usage.delete(sid);
          }
        }
      } catch {
        // Abort and transient event-stream errors are nonfatal.
      }
    })();
    return () => controller.abort();
  },
};
