package install

// Additional vendors beyond the embedded marketplace trees get full
// hook manifests written to the user's config dirs (Gemini settings,
// OpenCode/Pi TypeScript plugins, Copilot CLI hooks).

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

func installGenericVendor(vendor string, dryRun bool) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	soBin, err := resolveSoBin()
	if err != nil {
		return nil, err
	}
	q := strconv.Quote
	var path string
	var body string
	switch vendor {
	case "gemini":
		path = filepath.Join(home, ".gemini", "settings.json")
		body = fmt.Sprintf(`{
  "hooks": {
    "SessionStart": [{"hooks": [{"type": "command", "command": %s}]}],
    "SessionEnd": [{"hooks": [
      {"type": "command", "command": %s},
      {"type": "command", "command": %s, "timeout": 30}
    ]}],
    "UserPromptSubmit": [{"hooks": [{"type": "command", "command": %s}]}],
    "PreToolUse": [{"hooks": [{"type": "command", "command": %s}]}]
  }
}
`, q(soBin+" coding hook --vendor=gemini --event=SessionStart"),
			q(soBin+" coding hook --vendor=gemini --event=SessionEnd"),
			q(soBin+" sessions finalize"),
			q(soBin+" coding hook --vendor=gemini --event=UserPromptSubmit"),
			q(soBin+" coding hook --vendor=gemini --event=PreToolUse"))
	case "opencode":
		path = filepath.Join(home, ".opencode", "plugins", "superopen.ts")
		body = fmt.Sprintf(`// Superopen OpenCode plugin (telemetry parity with Claude/Cursor format)
import { spawn, spawnSync } from "child_process";
const so = %s;
function fire(event, payload, sync) {
  const args = ["coding", "hook", "--vendor=opencode", "--event=" + event];
  const input = JSON.stringify(payload || {}) + "\n";
  try {
    if (sync) {
      const r = spawnSync(so, args, { input, encoding: "utf8", timeout: 8000, stdio: ["pipe", "pipe", "ignore"] });
      return typeof r.stdout === "string" ? r.stdout : "";
    }
    const c = spawn(so, args, { stdio: ["pipe", "ignore", "ignore"] });
    c.stdin?.end(input);
  } catch {}
  return "";
}
function parseDeny(stdout) {
  if (!stdout) return null;
  for (const line of String(stdout).split("\n")) {
    const t = line.trim();
    if (!t.startsWith("{")) continue;
    try {
      const j = JSON.parse(t);
      if (j.decision === "deny" || j.permission === "deny") {
        return j.reason || j.userMessage || "blocked by Superopen guardrails";
      }
    } catch {}
  }
  return null;
}
export default {
  async event({ event }) {
    const type = (event && event.type) || "event";
    const props = (event && event.properties) || {};
    const sync = type === "session.created" || type === "session.idle" || type === "session.deleted";
    fire(type, Object.assign({}, props, { type: type, event: type }), sync);
    if (type === "session.idle" || type === "session.deleted") {
      fire("session.end", { type: "session.end", session_id: props.sessionID || props.session_id }, true);
      try { spawnSync(so, ["sessions", "finalize"], { timeout: 30000, stdio: "ignore" }); } catch {}
    }
  },
  async "chat.message"(input, output) {
    const msg = (output && output.message) || {};
    fire("message.updated", {
      session_id: input && input.sessionID, role: msg.role,
      text: typeof msg.content === "string" ? msg.content : undefined,
      tokens: msg.tokens, cost: msg.cost, model: msg.model && msg.model.id, type: "message.updated",
    });
  },
  async "tool.execute.before"(input) {
    const out = fire("tool.execute.before", {
      session_id: input && input.sessionID, tool_name: input && input.tool, tool_use_id: input && input.callID,
      command: input && input.args && (input.args.command || input.args.cmd),
      path: input && input.args && (input.args.path || input.args.file_path),
      type: "tool.execute.before",
    }, true);
    const deny = parseDeny(out);
    if (deny) throw new Error(deny);
  },
  async "tool.execute.after"(input, output) {
    fire("tool.execute.after", {
      session_id: input && input.sessionID, tool_name: input && input.tool, tool_use_id: input && input.callID,
      tool_result: typeof output === "string" ? output : JSON.stringify(output || {}), type: "tool.execute.after",
    });
  },
    async dispose() {
      fire("dispose", { type: "dispose" }, true);
      fire("session.end", { type: "session.end" }, true);
      try { spawnSync(so, ["sessions", "finalize"], { timeout: 30000, stdio: "ignore" }); } catch {}
    },
};
`, q(soBin))
	case "copilot-cli":
		path = filepath.Join(home, ".github", "hooks", "superopen.json")
		body = fmt.Sprintf(`{
  "hooks": {
    "sessionStart": [{"type": "command", "command": %s}],
    "sessionEnd": [
      {"type": "command", "command": %s},
      {"type": "command", "command": %s}
    ],
    "preToolUse": [{"type": "command", "command": %s}]
  }
}
`, q(soBin+" coding hook --vendor=copilot-cli --event=sessionStart"),
			q(soBin+" coding hook --vendor=copilot-cli --event=sessionEnd"),
			q(soBin+" sessions finalize"),
			q(soBin+" coding hook --vendor=copilot-cli --event=preToolUse"))
	case "pi":
		path = filepath.Join(home, ".pi", "extensions", "superopen", "index.ts")
		body = fmt.Sprintf(`// Superopen Pi extension (telemetry + Active Context inject + guardrails)
import { spawn, spawnSync } from "child_process";
const so = %s;
function fire(event, payload, sync) {
  const args = ["coding", "hook", "--vendor=pi", "--event=" + event];
  const input = JSON.stringify(payload || {}) + "\n";
  try {
    if (sync) {
      const r = spawnSync(so, args, { input, encoding: "utf8", timeout: 8000, stdio: ["pipe", "pipe", "ignore"] });
      return typeof r.stdout === "string" ? r.stdout : "";
    }
    const c = spawn(so, args, { stdio: ["pipe", "ignore", "ignore"] });
    c.stdin?.end(input);
  } catch {}
  return "";
}
function parseInject(stdout) {
  if (!stdout) return null;
  for (const line of String(stdout).split("\n")) {
    const t = line.trim();
    if (!t.startsWith("{")) continue;
    try {
      const j = JSON.parse(t);
      const v = j.inject_context || j.additional_context;
      if (typeof v === "string" && v.length) return v;
    } catch {}
  }
  return null;
}
function parseDeny(stdout) {
  if (!stdout) return null;
  for (const line of String(stdout).split("\n")) {
    const t = line.trim();
    if (!t.startsWith("{")) continue;
    try {
      const j = JSON.parse(t);
      if (j.decision === "deny" || j.permission === "deny") {
        return j.reason || j.userMessage || "blocked by Superopen guardrails";
      }
    } catch {}
  }
  return null;
}
export default function (pi) {
  let pending = null;
  let lastSid = null, lastCwd = null, lastFile = null;
  pi.on("session_start", async (_e, ctx) => {
    lastCwd = ctx.cwd;
    lastFile = ctx.sessionManager.getSessionFile();
    lastSid = ctx.sessionManager.getSessionId && ctx.sessionManager.getSessionId();
    const out = fire("session_start", {
      type: "session_start", cwd: ctx.cwd,
      session_file: lastFile, session_id: lastSid,
    }, true);
    pending = parseInject(out);
  });
  pi.on("before_agent_start", async (event, ctx) => {
    lastCwd = ctx.cwd;
    lastFile = ctx.sessionManager.getSessionFile();
    lastSid = (ctx.sessionManager.getSessionId && ctx.sessionManager.getSessionId()) || lastSid;
    const out = fire("before_agent_start", {
      type: "before_agent_start", cwd: ctx.cwd,
      session_file: lastFile, session_id: lastSid,
      prompt: event && event.prompt,
    }, true);
    const inj = parseInject(out) || pending;
    pending = null;
    if (inj) return { message: { customType: "superopen-context", content: inj, display: false } };
  });
  pi.on("message_end", async (event, ctx) => {
    const message = (event && event.message) || {};
    lastSid = (ctx.sessionManager.getSessionId && ctx.sessionManager.getSessionId()) || lastSid;
    fire("message_end", {
      type: "message_end", cwd: ctx.cwd, session_file: ctx.sessionManager.getSessionFile(),
      session_id: lastSid,
      role: message.role, text: typeof message.content === "string" ? message.content : undefined,
      model: message.model, usage: message.usage,
    });
  });
  pi.on("tool_execution_start", async (event, ctx) => {
    const out = fire("tool.execute.before", {
      type: "tool.execute.before", cwd: ctx.cwd, session_file: ctx.sessionManager.getSessionFile(),
      session_id: (ctx.sessionManager.getSessionId && ctx.sessionManager.getSessionId()) || lastSid,
      tool_name: event && event.toolName, toolName: event && event.toolName,
      command: event && event.args && (event.args.command || event.args.cmd),
      path: event && event.args && (event.args.path || event.args.file_path),
    }, true);
    const deny = parseDeny(out);
    if (deny) throw new Error(deny);
  });
  pi.on("tool_execution_end", async (event, ctx) => {
    fire("tool_execution_end", {
      type: "tool_execution_end", cwd: ctx.cwd, session_file: ctx.sessionManager.getSessionFile(),
      toolName: event && event.toolName, toolCallId: event && event.toolCallId,
      isError: event && event.isError, result: event && event.result,
    });
  });
  pi.on("agent_end", async (_e, ctx) => {
    lastSid = (ctx.sessionManager.getSessionId && ctx.sessionManager.getSessionId()) || lastSid;
    fire("agent_end", {
      type: "agent_end", cwd: ctx.cwd, session_file: ctx.sessionManager.getSessionFile(),
      session_id: lastSid,
    }, true);
    try { spawnSync(so, ["sessions", "finalize"], { timeout: 30000, stdio: "ignore" }); } catch {}
  });
  pi.on("session_shutdown", async () => {
    fire("session_shutdown", {
      type: "session_shutdown", cwd: lastCwd, session_file: lastFile, session_id: lastSid,
    }, true);
    try { spawnSync(so, ["sessions", "finalize"], { timeout: 30000, stdio: "ignore" }); } catch {}
  });
}
`, q(soBin))
	default:
		return nil, fmt.Errorf("unsupported generic vendor %q", vendor)
	}
	if dryRun {
		return []string{path}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return nil, err
	}
	return []string{path}, nil
}
