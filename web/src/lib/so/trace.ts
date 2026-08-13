import { readdirSync, statSync } from "fs";
import { fileExists, readText } from "./nodeio";
import { join, relative } from "path";
import type { SessionMap } from "./session_map";
import { sessionKey } from "./session_map";
import { repoRoot, soPath } from "./root";
import {
  humanizePromptPreview,
  isNestedSubagentSession,
  type SessionMeta,
} from "./sessions";

export type Touch = "hit" | "read" | "edit";
export type Action = "search" | "read" | "edit" | "exec" | "verify" | "other";

export type TraceTarget = {
  path: string;
  fileId?: number;
  touch: Touch;
};

export type TraceEvent = {
  seq: number;
  ts?: string;
  tool: string;
  action: Action;
  targets: TraceTarget[];
  resultBytes: number;
  isError: boolean;
  summary: string;
};

export type TraceMark = {
  seq: number;
  type: "compaction" | "user-message" | "subagent";
  note?: string;
};

export type Trace = {
  version: number;
  session: {
    id: string;
    harness: string;
    model?: string;
    title?: string;
    cwd?: string;
    startedAt?: string;
    endedAt?: string;
    eventCount: number;
    path?: string;
  };
  events: TraceEvent[];
  marks: TraceMark[];
  stats: Record<string, unknown>;
};

export type MapSessionMeta = {
  key: string;
  id: string;
  harness: string;
  title?: string;
  path: string;
  cwd?: string;
  model?: string;
  startedAt?: string;
  endedAt?: string;
  eventCount: number;
  userTurns?: number;
};

type Span = {
  name?: string;
  start_time_unix_nano?: number | string;
  attributes?: Record<string, string>;
};

function relToRepo(cwd: string, path: string): string {
  if (!path) return path;
  const norm = path.replace(/\\/g, "/");
  if (!cwd) return norm;
  try {
    const rel = relative(cwd, path).replace(/\\/g, "/");
    if (!rel.startsWith("..")) return rel;
  } catch {
    /* keep absolute */
  }
  return norm;
}

function emptyStats(
  filesInRepo: number,
  events: TraceEvent[],
  marks: TraceMark[],
) {
  let edited = 0;
  let read = 0;
  let seen = 0;
  const paths = new Map<string, Touch>();
  const editCounts = new Map<string, number>();
  for (const ev of events) {
    for (const t of ev.targets) {
      const prev = paths.get(t.path);
      const rank = { hit: 1, read: 2, edit: 3 } as const;
      if (!prev || rank[t.touch] > rank[prev]) paths.set(t.path, t.touch);
      if (t.touch === "edit") {
        editCounts.set(t.path, (editCounts.get(t.path) ?? 0) + 1);
      }
    }
  }
  for (const t of paths.values()) {
    if (t === "edit") edited++;
    else if (t === "read") read++;
    else seen++;
  }
  const actions = { search: 0, read: 0, edit: 0, exec: 0, verify: 0, other: 0 };
  let errors = 0;
  for (const ev of events) {
    actions[ev.action] = (actions[ev.action] || 0) + 1;
    if (ev.isError) errors++;
  }
  let maxEditsPerFile = 0;
  let churnFiles = 0;
  for (const n of editCounts.values()) {
    if (n > maxEditsPerFile) maxEditsPerFile = n;
    if (n >= 3) churnFiles++;
  }
  const userTurns = marks.filter((m) => m.type === "user-message").length;
  const compactions = marks.filter((m) => m.type === "compaction").length;
  const subagents = marks.filter((m) => m.type === "subagent").length;
  return {
    filesInRepo,
    fovea: edited + read,
    parafovea: seen,
    edited,
    eventsBeforeFirstEdit: 0,
    regressionRate: 0,
    errorRate: events.length ? errors / events.length : 0,
    actions,
    errors: { search: 0, read: 0, edit: 0, exec: 0, verify: 0, other: 0 },
    maxEditsPerFile,
    churnFiles,
    userTurns,
    compactions,
    subagents,
    resultBytes: 0,
    editsAfterLastVerify: 0,
    observability: { reads: "estimated", errors: "estimated" as const },
  };
}

function footprintEvents(
  sessionDir: string,
  cwd: string,
  startedAt?: string,
): TraceEvent[] {
  const fpPath = join(sessionDir, "session.json");
  if (!fileExists(fpPath)) return [];
  try {
    const doc = JSON.parse(readText(fpPath)) as {
      footprint?: {
        files?: { path: string; state?: string; count?: number }[];
      };
    };
    const fp = doc.footprint || {};
    return (fp.files || []).map((f, i) => {
      let touch: Touch = "hit";
      let tool = "Glob";
      let action: Action = "search";
      const st = (f.state || "").toLowerCase();
      if (st === "read") {
        touch = "read";
        tool = "Read";
        action = "read";
      } else if (st === "edited" || st === "edit") {
        touch = "edit";
        tool = "Write";
        action = "edit";
      }
      return {
        seq: i,
        ts: startedAt,
        tool,
        action,
        targets: [{ path: relToRepo(cwd, f.path), touch }],
        resultBytes: 0,
        isError: false,
        summary: f.path,
      };
    });
  } catch {
    return [];
  }
}

type ParsedSpans = { events: TraceEvent[]; marks: TraceMark[] };

function decodedStringLiteral(code: string, prefix: RegExp): string {
  const match = code.match(prefix);
  if (!match?.[1]) return "";
  try {
    return String(JSON.parse(match[1]));
  } catch {
    return "";
  }
}

function patchTargets(patch: string, cwd: string): TraceTarget[] {
  const seen = new Set<string>();
  const targets: TraceTarget[] = [];
  for (const line of patch.split("\n")) {
    const match = line.match(/^\*\*\* (?:Add|Update|Delete) File:\s+(.+)$/);
    if (!match?.[1]) continue;
    const path = relToRepo(cwd, match[1].trim());
    if (!path || seen.has(path)) continue;
    seen.add(path);
    targets.push({ path, touch: "edit" });
  }
  return targets;
}

function nestedCodexCommand(code: string): string {
  return decodedStringLiteral(code, /\bcmd\s*:\s*("(?:\\.|[^"\\])*")/);
}

function nestedCodexPatch(code: string): string {
  return decodedStringLiteral(
    code,
    /\b(?:const|let|var)\s+patch\s*=\s*("(?:\\.|[^"\\])*")/,
  );
}

/** Parse Codex's authoritative rollout JSONL into Map playback events. */
export function parseCodexRolloutLines(
  lines: string[],
  cwd: string,
): ParsedSpans {
  const events: TraceEvent[] = [];
  const marks: TraceMark[] = [];
  for (const line of lines) {
    if (!line.trim()) continue;
    let record: {
      timestamp?: string;
      type?: string;
      payload?: Record<string, unknown>;
    };
    try {
      record = JSON.parse(line);
    } catch {
      continue;
    }
    const payload = record.payload || {};
    const payloadType = String(payload.type || "");
    if (
      (record.type === "response_item" &&
        payloadType === "message" &&
        payload.role === "user") ||
      (record.type === "event_msg" && payloadType === "user_message")
    ) {
      marks.push({ seq: Math.max(0, events.length - 1), type: "user-message" });
      continue;
    }
    if (record.type !== "response_item") continue;
    if (
      !/^(custom_tool_call|function_call|local_shell_call)$/.test(payloadType)
    )
      continue;

    const rawTool = String(
      payload.name || (payloadType === "local_shell_call" ? "Bash" : ""),
    );
    const input = String(
      payload.input || payload.arguments || payload.command || "",
    );
    let tool = rawTool || "Tool";
    let action: Action = "other";
    let command = "";
    let targets: TraceTarget[] = [];

    if (/^apply_patch$/i.test(rawTool)) {
      tool = "apply_patch";
      action = "edit";
      targets = patchTargets(input, cwd);
    } else if (
      /^(exec|code|bash)$/i.test(rawTool) &&
      /tools\.apply_patch/.test(input)
    ) {
      tool = "apply_patch";
      action = "edit";
      targets = patchTargets(nestedCodexPatch(input), cwd);
    } else if (
      /^(exec|code)$/i.test(rawTool) &&
      /tools\.(?:exec_command|write_stdin)/.test(input)
    ) {
      tool = "Bash";
      action = "exec";
      command = nestedCodexCommand(input);
    } else if (
      payloadType === "local_shell_call" ||
      /bash|shell/i.test(rawTool)
    ) {
      tool = "Bash";
      action = "exec";
      command = input;
    } else if (/update_plan/i.test(input)) {
      tool = "Update plan";
    } else if (/view_image/i.test(input)) {
      tool = "View image";
    }

    events.push({
      seq: events.length,
      ts: record.timestamp,
      tool,
      action,
      targets,
      resultBytes: 0,
      isError: String(payload.status || "").toLowerCase() === "failed",
      summary:
        targets.length > 0
          ? targets.map((target) => target.path).join(", ")
          : command.replace(/\s+/g, " ").slice(0, 140) || tool,
    });
  }
  return { events, marks };
}

function parseTranscriptEvents(sessionDir: string, cwd: string): ParsedSpans {
  const tPath = join(sessionDir, "events.jsonl");
  if (!fileExists(tPath)) return { events: [], marks: [] };
  return parseSpanLines(readText(tPath).split("\n"), cwd);
}

function markSeq(events: TraceEvent[]): number {
  return Math.max(0, events.length - 1);
}

function classifyMark(
  name: string,
  attrs: Record<string, string>,
  tool: string,
): TraceMark["type"] | null {
  const blob =
    `${name} ${tool} ${attrs["coding_agent.event"] || ""} ${attrs["gen_ai.operation.name"] || ""}`.toLowerCase();
  if (
    attrs["coding_agent.is_subagent"] === "true" ||
    attrs["coding_agent.subagent"] === "true" ||
    /subagent|task_tool|agent_launch|launch.?agent/.test(blob) ||
    /^task$/i.test(tool)
  ) {
    return "subagent";
  }
  if (
    /compact|compaction|summariz(e|ing).?context|context.?summar/.test(blob)
  ) {
    return "compaction";
  }
  if (
    attrs["gen_ai.prompt.role"] === "user" ||
    attrs["coding_agent.message.role"] === "user" ||
    /user.?prompt|user_prompt|user.?message|user.?turn|gen_ai\.user|llm\.user/.test(
      blob,
    ) ||
    (name.includes("llm") && attrs["gen_ai.input.messages"] && !tool)
  ) {
    // Prefer explicit user turns; llm.turn with input messages often is a user prompt boundary
    if (/assistant|tool\.|completion/.test(blob) && !/user/.test(blob))
      return null;
    return "user-message";
  }
  return null;
}

function toolCommand(attrs: Record<string, string>): string {
  const direct = attrs["coding_agent.tool.command"] || "";
  if (direct.trim()) return direct.trim();
  const args = attrs["gen_ai.tool.call.arguments"] || "";
  if (!args) return "";
  try {
    const parsed = JSON.parse(args) as Record<string, unknown>;
    return String(parsed.command || parsed.cmd || parsed.script || "").trim();
  } catch {
    return "";
  }
}

/** Best-effort path extraction from shell commands for map highlighting. */
function pathFromCommand(command: string, cwd: string): string {
  if (!command) return "";
  const repo = cwd.replace(/\\/g, "/").replace(/\/$/, "");
  const abs = command.match(
    new RegExp(
      `(${repo.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}/[^\\s'\"\`]+)`,
      "i",
    ),
  );
  if (abs?.[1]) return abs[1];
  const rel = command.match(
    /(?:^|[\s"'`])((?:\.\/)?(?:src|pkg|internal|web|cmd|integrations|ops|docs)\/[^\s'"`]+)/,
  );
  if (rel?.[1]) return rel[1].replace(/^\.\//, "");
  return "";
}

function parseSpanLines(lines: string[], cwd: string): ParsedSpans {
  const events: TraceEvent[] = [];
  const marks: TraceMark[] = [];
  for (const line of lines) {
    if (!line.trim()) continue;
    let sp: Span;
    try {
      sp = JSON.parse(line);
    } catch {
      continue;
    }
    const name = sp.name || "";
    const attrs = sp.attributes || {};
    const ts =
      sp.start_time_unix_nano != null
        ? new Date(Number(sp.start_time_unix_nano) / 1e6).toISOString()
        : undefined;

    let filePath =
      attrs["code.file.path"] || attrs["coding_agent.file_path"] || "";
    let tool =
      attrs["gen_ai.tool.name"] || attrs["coding_agent.tool.name"] || "";
    const args = attrs["gen_ai.tool.call.arguments"] || "";
    if (!filePath && args) {
      if (
        args.startsWith("/") ||
        args.startsWith(".") ||
        /^[A-Za-z]:[\\/]/.test(args) ||
        args.startsWith("\\\\")
      )
        filePath = args;
      else {
        try {
          const parsed = JSON.parse(args) as Record<string, unknown>;
          filePath = String(
            parsed.file_path ||
              parsed.path ||
              parsed.target_file ||
              parsed.filePath ||
              "",
          );
        } catch {
          /* ignore */
        }
      }
    }

    const command = toolCommand(attrs);
    if (!filePath && command) {
      filePath = pathFromCommand(command, cwd);
    }

    const markType = classifyMark(name, attrs, tool);
    if (markType) {
      let note: string | undefined;
      if (markType === "user-message") {
        note =
          humanizePromptPreview(
            attrs["gen_ai.prompt"] ||
              attrs["coding_agent.prompt"] ||
              attrs["gen_ai.input.messages"] ||
              attrs["coding_agent.user_prompt"] ||
              "",
          ) || undefined;
      } else if (markType === "subagent") {
        note =
          attrs["coding_agent.subagent.name"] ||
          attrs["coding_agent.agent.name"] ||
          tool ||
          "subagent";
      }
      marks.push({ seq: markSeq(events), type: markType, note });
    }

    // tool.requested is paired with tool.call - only count the call for playback.
    if (/\.tool\.requested$/i.test(name) || /tool\.requested/i.test(name)) {
      continue;
    }

    let touch: Touch | null = null;
    let action: Action = "other";
    const toolBlob = `${tool} ${name}`.toLowerCase();
    if (
      name.includes("edit") ||
      /write|searchreplace|applypatch|edit/i.test(tool)
    ) {
      touch = "edit";
      action = "edit";
      tool = tool || "Write";
    } else if (
      /^read$/i.test(tool) ||
      /\bread\b/i.test(tool) ||
      name.includes("read")
    ) {
      touch = "read";
      action = "read";
      tool = tool || "Read";
    } else if (/grep|glob|search|semantic/i.test(tool)) {
      touch = "hit";
      action = "search";
      tool = tool || "Grep";
    } else if (
      name.includes("shell") ||
      /bash|shell|terminal/i.test(tool) ||
      Boolean(command)
    ) {
      action = "exec";
      tool = tool || "Bash";
    } else if (/tool\.(call|completed|result)/i.test(name) && tool) {
      action = "other";
    } else {
      continue;
    }

    const summary =
      (filePath && relToRepo(cwd, filePath)) ||
      (command ? command.replace(/\s+/g, " ").slice(0, 140) : "") ||
      tool ||
      name;

    // File-touching tools need a path; exec/other tool calls play without one
    // (Claude/Codex often use Bash without code.file.path).
    if (touch && !filePath) continue;
    if (!touch && action !== "exec" && action !== "other") continue;
    if (!touch && !tool && !command) continue;

    const targets: TraceTarget[] = [];
    if (filePath) {
      targets.push({
        path: relToRepo(cwd, filePath),
        touch: touch || "hit",
      });
    }

    events.push({
      seq: events.length,
      ts,
      tool,
      action: touch ? action : action === "other" ? "exec" : action,
      targets,
      resultBytes: Number(attrs["coding_agent.tool.result_bytes"] || 0) || 0,
      isError:
        attrs["error"] === "true" ||
        attrs["coding_agent.is_error"] === "true" ||
        attrs["coding_agent.tool.errored"] === "true" ||
        /error|fail/i.test(attrs["coding_agent.result"] || ""),
      summary,
    });
  }
  return { events, marks };
}

export function listMapSessions(): MapSessionMeta[] {
  const dir = soPath("sessions");
  if (!fileExists(dir)) return [];
  const root = repoRoot();
  const out: MapSessionMeta[] = [];
  for (const name of readdirSync(dir)) {
    if (name === "index.json" || name.startsWith(".") || name.endsWith(".json"))
      continue;
    const sessionDir = join(dir, name);
    try {
      if (!statSync(sessionDir).isDirectory()) continue;
    } catch {
      continue;
    }
    const metaPath = join(sessionDir, "session.json");
    if (!fileExists(metaPath)) continue;
    try {
      const meta = JSON.parse(readText(metaPath)) as SessionMeta;
      if (!meta.id) meta.id = name;
      if (isNestedSubagentSession(meta, dir)) continue;
      const id = String(meta.id || name);
      const harness = String(meta.vendor || "cursor");
      out.push({
        key: sessionKey(harness, sessionDir),
        id,
        harness,
        title:
          humanizePromptPreview(
            String(meta.title || meta.prompt_preview || ""),
          ) || String(meta.title || meta.prompt_preview || ""),
        path: sessionDir,
        cwd: root,
        model: meta.model ? String(meta.model) : undefined,
        startedAt: meta.started_at ? String(meta.started_at) : undefined,
        endedAt: meta.ended_at ? String(meta.ended_at) : undefined,
        eventCount: 0,
        userTurns: 0,
      });
    } catch {
      /* skip */
    }
  }
  out.sort((a, b) =>
    String(b.startedAt || "").localeCompare(String(a.startedAt || "")),
  );
  return out;
}

export function resolveMapSession(selector: string): MapSessionMeta | null {
  const all = listMapSessions();
  const exact = all.find((s) => s.key === selector);
  if (exact) return exact;
  const byId = all.filter((s) => s.id === selector);
  if (byId.length === 1) return byId[0];
  if (byId.length > 1) return byId[0];

  // Nested subagents are omitted from the top-level list - still resolve by id/key.
  return resolveNestedMapSession(selector);
}

function resolveNestedMapSession(selector: string): MapSessionMeta | null {
  const dir = soPath("sessions");
  if (!fileExists(dir)) return null;
  const root = repoRoot();
  for (const name of readdirSync(dir)) {
    if (name === "index.json" || name.startsWith(".") || name.endsWith(".json"))
      continue;
    const sessionDir = join(dir, name);
    try {
      if (!statSync(sessionDir).isDirectory()) continue;
    } catch {
      continue;
    }
    const metaPath = join(sessionDir, "session.json");
    if (!fileExists(metaPath)) continue;
    try {
      const meta = JSON.parse(readText(metaPath)) as SessionMeta;
      if (!meta.id) meta.id = name;
      if (!meta.parent_id) continue;
      const id = String(meta.id || name);
      const harness = String(meta.vendor || "cursor");
      const key = sessionKey(harness, sessionDir);
      if (selector !== key && selector !== id) continue;
      return {
        key,
        id,
        harness,
        title:
          humanizePromptPreview(
            String(meta.title || meta.prompt_preview || ""),
          ) || String(meta.title || meta.prompt_preview || id),
        path: sessionDir,
        cwd: root,
        model: meta.model ? String(meta.model) : undefined,
        startedAt: meta.started_at ? String(meta.started_at) : undefined,
        endedAt: meta.ended_at ? String(meta.ended_at) : undefined,
        eventCount: 0,
        userTurns: 0,
      };
    } catch {
      /* skip */
    }
  }
  return null;
}

function assignFileIds(trace: Trace, sessionMap: SessionMap) {
  const byPath = new Map(sessionMap.files.map((f) => [f.path, f.id]));
  for (const ev of trace.events) {
    for (const t of ev.targets) {
      const id = byPath.get(t.path);
      if (id !== undefined) t.fileId = id;
    }
  }
}

/** Child sessions launched from this session → subagent marks on the timeline. */
function subagentMarksFromChildren(
  parentId: string,
  eventCount: number,
): TraceMark[] {
  const dir = soPath("sessions");
  if (!fileExists(dir)) return [];
  const marks: TraceMark[] = [];
  for (const name of readdirSync(dir)) {
    if (name === "index.json" || name.startsWith(".") || name === parentId)
      continue;
    try {
      const meta = JSON.parse(
        readText(join(dir, name, "session.json")),
      ) as Record<string, unknown>;
      if (String(meta.parent_id || "") !== parentId) continue;
      const title =
        humanizePromptPreview(
          String(meta.title || meta.prompt_preview || ""),
        ) || String(meta.title || name);
      marks.push({
        seq: Math.max(0, eventCount - 1),
        type: "subagent",
        note: title.slice(0, 80),
      });
    } catch {
      /* skip */
    }
  }
  return marks;
}

/** Build map Trace from a Superopen session directory. */
export function buildTrace(session: MapSessionMeta, sessionMap: SessionMap): Trace {
  const cwd = session.cwd || repoRoot();
  let parsed = parseTranscriptEvents(session.path, cwd);
  if (parsed.events.length === 0) {
    parsed = {
      events: footprintEvents(session.path, cwd, session.startedAt),
      marks: [],
    };
  }
  parsed.events.forEach((e, i) => {
    e.seq = i;
  });

  const childMarks = subagentMarksFromChildren(
    session.id,
    parsed.events.length,
  );
  const marks = [...parsed.marks, ...childMarks];
  // Clamp mark seqs into event range
  for (const m of marks) {
    if (parsed.events.length === 0) m.seq = 0;
    else m.seq = Math.min(Math.max(0, m.seq), parsed.events.length - 1);
  }

  const trace: Trace = {
    version: 1,
    session: {
      id: session.id,
      harness: session.harness,
      model: session.model,
      title: session.title,
      cwd,
      startedAt: session.startedAt,
      endedAt: session.endedAt,
      eventCount: parsed.events.length,
      path: session.path,
    },
    events: parsed.events,
    marks,
    stats: emptyStats(sessionMap.files.length, parsed.events, marks),
  };
  assignFileIds(trace, sessionMap);
  return trace;
}
