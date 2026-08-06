import { readdirSync, statSync, writeFileSync } from "fs";
import { join } from "path";
import { homedir } from "os";
import { fileExists, readJSONFile, readText } from "./nodeio";
import { processRepoRoot, soRoot } from "./root";
import { projectsForFilter } from "./workspace";
import {
  parseSessionQuery,
  userMatches,
  vendorMatchesAgent,
  type SessionQueryFacets,
} from "./session-query";

export type SessionMeta = {
  id: string;
  vendor?: string;
  model?: string;
  /** Canonical human identity (usually email from gen_ai.user.name). */
  user?: string;
  title?: string;
  prompt_preview?: string;
  status?: string;
  started_at?: string;
  ended_at?: string;
  duration_ms?: number;
  tokens?: number;
  cost_usd?: number;
  branch?: string;
  project_id?: string;
  [key: string]: unknown;
};

export type ListItem = SessionMeta & {
  checkpoints?: number;
  turns?: number;
  files?: string[];
  match?: string;
  /** Absolute `.so` root this session was loaded from (for transcript/trace search). */
  so_root?: string;
};

export type SessionsListResult = {
  sessions: ListItem[];
  facets: SessionQueryFacets;
};

type TraceSpan = {
  name?: string;
  session_id?: string;
  start_time_unix_nano?: number | string;
  attributes?: Record<string, string>;
  [key: string]: unknown;
};

function readJSON<T>(path: string): T | null {
  return readJSONFile<T>(path);
}

/** Unwrap Cursor/Claude gen_ai.input.messages JSON into plain user text. */
export function humanizePromptPreview(raw?: string): string {
  if (!raw) return "";
  const text = raw.trim();
  if (!text) return "";
  if (!text.startsWith("[") && !text.startsWith("{")) return text;
  try {
    const parsed = JSON.parse(text.startsWith("{") ? `[${text}]` : text);
    if (!Array.isArray(parsed)) return text;
    for (const msg of parsed) {
      const role = String(msg?.role || "");
      if (role && role !== "user" && role !== "user_prompt") continue;
      if (typeof msg?.content === "string" && msg.content.trim()) {
        return msg.content.trim();
      }
      if (Array.isArray(msg?.parts)) {
        const joined = msg.parts
          .map((p: { content?: string; text?: string }) => p?.content || p?.text || "")
          .filter(Boolean)
          .join("\n")
          .trim();
        if (joined) return joined;
      }
      if (typeof msg?.text === "string" && msg.text.trim()) return msg.text.trim();
    }
  } catch {
    /* keep raw */
  }
  return text;
}

function displayTitle(meta: SessionMeta): string {
  const titled = humanizePromptPreview(meta.title);
  if (titled) return titled;
  const preview = humanizePromptPreview(meta.prompt_preview);
  if (preview) return preview;
  return meta.id;
}

const UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function loadAgentLinks(sessionsDir: string): Record<string, string> {
  const path = join(sessionsDir, "agent-links.json");
  const doc = readJSON<{ links?: Record<string, { parent_id?: string }> }>(path);
  const out: Record<string, string> = {};
  for (const [child, entry] of Object.entries(doc?.links || {})) {
    const parent = String(entry?.parent_id || "").trim();
    if (parent && parent !== child) out[child] = parent;
  }
  return out;
}

/** Cursor stores Task agents under agent-transcripts/<parent>/subagents/<child>.jsonl */
function discoverCursorParent(childId: string): string {
  if (!UUID_RE.test(childId)) return "";
  const projects = join(homedir(), ".cursor", "projects");
  if (!fileExists(projects)) return "";
  try {
    for (const proj of readdirSync(projects)) {
      const root = join(projects, proj, "agent-transcripts");
      if (!fileExists(root)) continue;
      for (const parent of readdirSync(root)) {
        if (!UUID_RE.test(parent) || parent === childId) continue;
        const candidate = join(root, parent, "subagents", `${childId}.jsonl`);
        if (fileExists(candidate)) return parent;
      }
    }
  } catch {
    /* ignore */
  }
  return "";
}

function clearAgentLink(sessionsDir: string, childId: string) {
  const path = join(sessionsDir, "agent-links.json");
  const existing = readJSON<{ links: Record<string, { parent_id: string; source?: string }> }>(
    path
  );
  if (!existing?.links?.[childId]) return;
  delete existing.links[childId];
  try {
    writeFileSync(path, JSON.stringify(existing, null, 2), { mode: 0o600 });
  } catch {
    /* best-effort */
  }
}

function clearFalseNesting(sessionPath: string, sessionsDir: string, meta: SessionMeta) {
  const id = String(meta.id || "");
  clearAgentLink(sessionsDir, id);
  if (!meta.parent_id && !meta.is_subagent) return;
  meta.parent_id = undefined;
  meta.is_subagent = false;
  try {
    writeFileSync(join(sessionPath, "meta.json"), JSON.stringify(meta, null, 2));
  } catch {
    /* best-effort */
  }
}

function parentIsEmptyStub(sessionsDir: string, parentId: string): boolean {
  const metaPath = join(sessionsDir, parentId, "meta.json");
  if (!fileExists(metaPath)) return false; // parent not on disk yet - keep nesting
  const meta = readJSON<SessionMeta>(metaPath);
  if (!meta) return false;
  const sessionPath = join(sessionsDir, parentId);
  const soDir = join(sessionsDir, "..");
  const stats = enrichSessionStats(soDir, sessionPath, meta);
  return isEmptySession({
    ...meta,
    checkpoints: stats.checkpoints,
    turns: stats.turns,
    files: [],
    prompt_preview: humanizePromptPreview(meta.prompt_preview) || meta.prompt_preview,
  });
}

function persistAgentLink(
  sessionsDir: string,
  childId: string,
  parentId: string
) {
  const path = join(sessionsDir, "agent-links.json");
  let doc: { links: Record<string, { parent_id: string; source?: string }> } = {
    links: {},
  };
  const existing = readJSON<typeof doc>(path);
  if (existing?.links) doc = existing;
  if (doc.links[childId]?.parent_id === parentId) return;
  doc.links[childId] = { parent_id: parentId, source: "ui-repair" };
  try {
    writeFileSync(path, JSON.stringify(doc, null, 2), { mode: 0o600 });
  } catch {
    /* best-effort */
  }
}

function repairSubagentMeta(
  sessionPath: string,
  meta: SessionMeta,
  parentId: string
) {
  if (meta.parent_id === parentId && meta.is_subagent) return;
  meta.parent_id = parentId;
  meta.is_subagent = true;
  try {
    writeFileSync(join(sessionPath, "meta.json"), JSON.stringify(meta, null, 2));
  } catch {
    /* best-effort */
  }
}

function repairOrphanSubagentFlag(sessionPath: string, meta: SessionMeta) {
  if (!meta.is_subagent || meta.parent_id) return;
  meta.is_subagent = false;
  try {
    writeFileSync(join(sessionPath, "meta.json"), JSON.stringify(meta, null, 2));
  } catch {
    /* best-effort */
  }
}

/** True when this row is a nested Task/subagent and must not list as a session. */
export function isNestedSubagentSession(
  meta: SessionMeta,
  sessionsDir: string,
  links?: Record<string, string>
): boolean {
  const linkMap = links || loadAgentLinks(sessionsDir);
  return isNestedSubagent(meta, linkMap, sessionsDir);
}

/** True when this row is a nested Task/subagent and must not list as a session. */
function isNestedSubagent(
  meta: SessionMeta,
  links: Record<string, string>,
  sessionsDir: string
): boolean {
  const id = String(meta.id || "");
  const sessionPath = join(sessionsDir, id);

  let parent = String(meta.parent_id || "").trim();
  if (!parent) {
    // Real nesting requires a parent id. Lone is_subagent is poison from
    // parent chats that emit subagent-typed spans on their own session id.
    if (meta.is_subagent) {
      repairOrphanSubagentFlag(sessionPath, meta);
    }
    if (links[id]) {
      parent = links[id];
    } else {
      const discovered = discoverCursorParent(id);
      if (discovered) {
        persistAgentLink(sessionsDir, id, discovered);
        parent = discovered;
      }
    }
  }
  if (!parent) return false;

  // Greedy pending-claim / test stubs: do not hide rows nested under
  // empty parents (Sessions would show nothing).
  if (parentIsEmptyStub(sessionsDir, parent)) {
    clearFalseNesting(sessionPath, sessionsDir, meta);
    return false;
  }

  if (meta.parent_id !== parent || !meta.is_subagent) {
    repairSubagentMeta(sessionPath, meta, parent);
  }
  return true;
}

function countCheckpointDirs(sessionPath: string): number {
  const dir = join(sessionPath, "checkpoints");
  if (!fileExists(dir)) return 0;
  try {
    return readdirSync(dir).filter((name) => {
      try {
        return statSync(join(dir, name)).isDirectory();
      } catch {
        return false;
      }
    }).length;
  } catch {
    return 0;
  }
}

export type RestoreCheckpoint = {
  id: number;
  session_id?: string;
  created_at?: string;
  label?: string;
  files?: string[];
};

function listCheckpoints(sessionPath: string, sessionId: string): RestoreCheckpoint[] {
  const dir = join(sessionPath, "checkpoints");
  if (!fileExists(dir)) return [];
  const out: RestoreCheckpoint[] = [];
  try {
    for (const name of readdirSync(dir)) {
      const cpDir = join(dir, name);
      try {
        if (!statSync(cpDir).isDirectory()) continue;
      } catch {
        continue;
      }
      const meta =
        readJSON<RestoreCheckpoint>(join(cpDir, "meta.json")) ||
        ({ id: Number(name) || 0 } as RestoreCheckpoint);
      if (!meta.id) meta.id = Number(name) || out.length + 1;
      if (!meta.session_id) meta.session_id = sessionId;
      out.push(meta);
    }
  } catch {
    return [];
  }
  out.sort((a, b) => b.id - a.id);
  return out;
}

function countTurnsFromSpans(spans: TraceSpan[]): number {
  let n = 0;
  for (const sp of spans) {
    const attrs = sp.attributes || {};
    if (attrs["gen_ai.prompt"] || attrs["gen_ai.content.prompt"]) {
      n++;
      continue;
    }
    const raw = attrs["gen_ai.input.messages"] || "";
    if (
      raw &&
      (raw.toLowerCase().includes('"role":"user"') ||
        raw.toLowerCase().includes('"role": "user"') ||
        raw.toLowerCase().includes('"role":"user_prompt"'))
    ) {
      n++;
    }
  }
  return n;
}

function loadTranscriptSpans(sessionPath: string): TraceSpan[] {
  const transcriptPath = join(sessionPath, "transcript.jsonl");
  if (!fileExists(transcriptPath)) return [];
  const out: TraceSpan[] = [];
  for (const line of readText(transcriptPath).split("\n")) {
    if (!line.trim()) continue;
    try {
      out.push(JSON.parse(line) as TraceSpan);
    } catch {
      /* skip */
    }
  }
  return out;
}

function enrichSessionStats(
  soDir: string,
  sessionPath: string,
  meta: SessionMeta
): { turns: number; checkpoints: number } {
  let spans = loadTranscriptSpans(sessionPath);
  if (spans.length === 0) {
    spans = loadLiveTraces(soDir, String(meta.id || ""));
  }
  return {
    turns: countTurnsFromSpans(spans),
    checkpoints: countCheckpointDirs(sessionPath),
  };
}

function spanSessionIDs(sp: TraceSpan): string[] {
  const attrs = sp.attributes || {};
  return [
    sp.session_id,
    attrs["coding_agent.session.id"],
    attrs["coding_agent.session_id"],
    attrs["gen_ai.conversation.id"],
  ].filter(Boolean) as string[];
}

/** Read live OTLP spans for a session from .so/traces/*.jsonl (active sessions). */
function loadLiveTraces(soDir: string, sessionId: string): TraceSpan[] {
  const tracesDir = join(soDir, "traces");
  if (!fileExists(tracesDir)) return [];
  const out: TraceSpan[] = [];
  for (const name of readdirSync(tracesDir)) {
    if (!name.endsWith(".jsonl")) continue;
    let body = "";
    try {
      body = readText(join(tracesDir, name));
    } catch {
      continue;
    }
    for (const line of body.split("\n")) {
      if (!line.trim()) continue;
      try {
        const sp = JSON.parse(line) as TraceSpan;
        if (spanSessionIDs(sp).includes(sessionId)) out.push(sp);
      } catch {
        /* skip */
      }
    }
  }
  out.sort(
    (a, b) =>
      Number(a.start_time_unix_nano || 0) - Number(b.start_time_unix_nano || 0)
  );
  return out;
}

/** session id → gen_ai.user.name from daily OTLP dumps (best-effort). */
function loadUserMap(soDir: string): Map<string, string> {
  const out = new Map<string, string>();
  const tracesDir = join(soDir, "traces");
  if (!fileExists(tracesDir)) return out;
  let names: string[] = [];
  try {
    names = readdirSync(tracesDir).filter((n) => n.endsWith(".jsonl"));
  } catch {
    return out;
  }
  // Newest first so active identity wins.
  names.sort().reverse();
  for (const name of names) {
    let body = "";
    try {
      body = readText(join(tracesDir, name));
    } catch {
      continue;
    }
    for (const line of body.split("\n")) {
      if (!line.trim()) continue;
      try {
        const sp = JSON.parse(line) as TraceSpan;
        const user = String(sp.attributes?.["gen_ai.user.name"] || "").trim();
        if (!user) continue;
        for (const id of spanSessionIDs(sp)) {
          if (id && !out.has(id)) out.set(id, user);
        }
      } catch {
        /* skip */
      }
    }
  }
  return out;
}

function pathBase(p: string): string {
  const i = Math.max(p.lastIndexOf("/"), p.lastIndexOf("\\"));
  return i >= 0 ? p.slice(i + 1) : p;
}

function truncateMatch(s: string, n: number): string {
  const t = s.replace(/\s+/g, " ").trim();
  if (t.length <= n) return t;
  return `${t.slice(0, Math.max(0, n - 1))}…`;
}

const FILE_ATTR_KEYS = [
  "coding_agent.file_path",
  "code.file.path",
  "coding_agent.edit.file_path",
];

const TOOL_ATTR_KEYS = [
  "gen_ai.tool.name",
  "coding_agent.tool.name",
  "coding_agent.edit.tool.name",
];

const CHAT_ATTR_KEYS = [
  "gen_ai.prompt",
  "gen_ai.content.prompt",
  "gen_ai.completion",
  "gen_ai.content.completion",
  "coding_agent.llm.thought.text",
  "coding_agent.shell.command",
  "coding_agent.tool.command",
  "gen_ai.tool.call.arguments",
  "gen_ai.input.messages",
  "gen_ai.output.messages",
];

function matchFilePaths(paths: string[], needle: string): string {
  for (const path of paths) {
    const base = pathBase(path);
    if (
      base.toLowerCase().includes(needle) ||
      path.toLowerCase().includes(needle)
    ) {
      return `file:${base}`;
    }
  }
  return "";
}

function matchToolName(name: string, needle: string): boolean {
  const n = name.toLowerCase();
  return n.includes(needle) || needle.includes(n);
}

/** Scan transcript / live OTLP spans for file, tool, or chat hits. */
function matchSpansContent(
  spans: TraceSpan[],
  needle: string,
  mode: "any" | "file" | "tool"
): string {
  for (const sp of spans) {
    const attrs = sp.attributes || {};

    if (mode === "any" || mode === "tool") {
      for (const key of TOOL_ATTR_KEYS) {
        const v = String(attrs[key] || "").trim();
        if (v && matchToolName(v, needle)) return `tool:${v}`;
      }
      const spanName = String(sp.name || "");
      if (
        /shell/i.test(spanName) &&
        (needle === "bash" || needle === "shell" || needle === "exec")
      ) {
        return "tool:Bash";
      }
    }

    if (mode === "any" || mode === "file") {
      const filePaths: string[] = [];
      for (const key of FILE_ATTR_KEYS) {
        const v = String(attrs[key] || "").trim();
        if (v) filePaths.push(v);
      }
      const fileHit = matchFilePaths(filePaths, needle);
      if (fileHit) return fileHit;
    }

    if (mode === "tool") continue;

    for (const key of CHAT_ATTR_KEYS) {
      const v = String(attrs[key] || "");
      if (!v) continue;
      if (!v.toLowerCase().includes(needle)) continue;
      if (mode === "file") {
        const tokens = v.match(/[^\s"'`]+/g) || [];
        for (const tok of tokens) {
          if (
            tok.toLowerCase().includes(needle) &&
            (tok.includes("/") || tok.includes(".") || tok.includes("\\"))
          ) {
            return `file:${pathBase(tok)}`;
          }
        }
        continue;
      }
      return `chat:${truncateMatch(v, 80)}`;
    }
  }
  return "";
}

function loadSessionSpans(item: ListItem): TraceSpan[] {
  const so = item.so_root;
  if (!so || !item.id) return [];
  const sessionPath = join(so, "sessions", item.id);
  let spans = loadTranscriptSpans(sessionPath);
  if (spans.length === 0) {
    spans = loadLiveTraces(so, item.id);
  }
  return spans;
}

/**
 * Match free text / file: / tool: against title, footprint files, transcript, and live traces.
 * Returns a label like `file:foo.ts`, `tool:Bash`, `chat:…`, or `title`.
 */
function matchSessionContent(
  item: ListItem,
  needle: string,
  mode: "any" | "file" | "tool"
): string {
  if (!needle) return "ok";

  if (mode === "any") {
    if ((item.title || "").toLowerCase().includes(needle)) return "title";
    if ((item.prompt_preview || "").toLowerCase().includes(needle)) return "prompt";
    if ((item.id || "").toLowerCase().includes(needle)) return "id";
    if ((item.vendor || "").toLowerCase().includes(needle)) return "vendor";
    if ((item.model || "").toLowerCase().includes(needle)) return "model";
    if ((item.user || "").toLowerCase().includes(needle)) return "user";
  }

  if (mode === "any" || mode === "file") {
    const foot = matchFilePaths(item.files || [], needle);
    if (foot) return foot;
  }

  if (mode === "any" || mode === "tool") {
    // Footprint never stores tools; fall through to spans.
  }

  return matchSpansContent(loadSessionSpans(item), needle, mode);
}

function listSessionsInSo(soDir: string, projectId: string): ListItem[] {
  const dir = join(soDir, "sessions");
  if (!fileExists(dir)) return [];
  const links = loadAgentLinks(dir);
  const users = loadUserMap(soDir);
  const items: ListItem[] = [];
  for (const name of readdirSync(dir)) {
    if (name === "index.json" || name.startsWith(".") || name.endsWith(".json"))
      continue;
    const sessionPath = join(dir, name);
    try {
      if (!statSync(sessionPath).isDirectory()) continue;
    } catch {
      continue;
    }
    const meta = readJSON<SessionMeta>(join(sessionPath, "meta.json"));
    if (!meta) continue;
    if (!meta.id) meta.id = name;
    // Subagents are not top-level sessions - they nest under the parent chat.
    if (isNestedSubagent(meta, links, dir)) continue;
    const footprint = readJSON<{ files?: { path: string }[] }>(
      join(sessionPath, "footprint.json")
    );
    const files = (footprint?.files || []).map((f) => f.path).filter(Boolean);
    const title = displayTitle(meta);
    const user =
      String(meta.user || "").trim() || users.get(meta.id) || undefined;
    const stats = enrichSessionStats(soDir, sessionPath, meta);
    const item: ListItem = {
      ...meta,
      user,
      title,
      prompt_preview: humanizePromptPreview(meta.prompt_preview) || meta.prompt_preview,
      project_id: projectId,
      so_root: soDir,
      checkpoints: stats.checkpoints,
      turns: stats.turns,
      files,
    };
    if (isEmptySession(item)) continue;
    items.push(item);
  }
  return items;
}

/** Opened chat with no turns/work - hide from UI (matches Go session.IsEmptyListItem). */
function isEmptySession(item: ListItem): boolean {
  if ((item.turns || 0) > 0 || Number(item.tokens || 0) > 0 || (item.checkpoints || 0) > 0) {
    return false;
  }
  if ((item.files || []).length > 0) return false;
  if (String(item.prompt_preview || "").trim()) return false;
  return true;
}

function collectAllSessions(projectFilter = ""): ListItem[] {
  const projects = projectsForFilter(projectFilter);
  const items: ListItem[] = [];
  const seen = new Set<string>();
  for (const p of projects) {
    const so = p.so_root || join(p.repo_root, ".so");
    if (seen.has(so)) continue;
    seen.add(so);
    items.push(...listSessionsInSo(so, p.id));
  }
  if (items.length === 0 && projects.length === 0) {
    const root = processRepoRoot();
    items.push(...listSessionsInSo(join(root, ".so"), "local"));
  }
  items.sort((a, b) =>
    String(b.started_at || b.ended_at || "").localeCompare(
      String(a.started_at || a.ended_at || "")
    )
  );
  return items;
}

function facetsFrom(items: ListItem[]): SessionQueryFacets {
  const users = new Set<string>();
  const agents = new Set<string>();
  const files = new Set<string>();
  for (const s of items) {
    if (s.user) users.add(s.user);
    if (s.vendor) agents.add(s.vendor);
    for (const f of s.files || []) {
      const base = pathBase(f);
      if (base) files.add(base);
    }
  }
  return {
    users: Array.from(users).sort((a, b) => a.localeCompare(b)),
    agents: Array.from(agents).sort((a, b) => a.localeCompare(b)),
    files: Array.from(files).sort((a, b) => a.localeCompare(b)).slice(0, 40),
  };
}

function applySessionQuery(items: ListItem[], q: string): ListItem[] {
  const parsed = parseSessionQuery(q);
  const hasFilter = Boolean(
    parsed.text ||
      parsed.user ||
      parsed.agent ||
      parsed.model ||
      parsed.file ||
      parsed.tool
  );
  if (!hasFilter) return items;
  const needle = parsed.text.toLowerCase();
  const fileNeedle = parsed.file.toLowerCase();
  const toolNeedle = parsed.tool.toLowerCase();
  const out: ListItem[] = [];
  for (const item of items) {
    if (parsed.user && !userMatches(item.user, parsed.user)) continue;
    if (parsed.agent && !vendorMatchesAgent(item.vendor, parsed.agent)) continue;
    if (parsed.model) {
      const m = (item.model || "").toLowerCase();
      if (!m.includes(parsed.model.toLowerCase())) continue;
    }
    let match = "";
    if (fileNeedle) {
      match = matchSessionContent(item, fileNeedle, "file");
      if (!match) continue;
    }
    if (toolNeedle) {
      match = matchSessionContent(item, toolNeedle, "tool");
      if (!match) continue;
    }
    if (needle) {
      match = matchSessionContent(item, needle, "any");
      if (!match) continue;
    } else if (!match) {
      if (parsed.user) match = `from:@${parsed.user}`;
      else if (parsed.agent) match = `agent:${parsed.agent}`;
      else if (parsed.model) match = `model:${parsed.model}`;
    }
    out.push(match ? { ...item, match } : item);
  }
  return out;
}

export function listSessions(q = "", projectFilter = ""): ListItem[] {
  return applySessionQuery(collectAllSessions(projectFilter), q);
}

/** Sessions list + facet values for search autocomplete. */
export function listSessionsPage(q = "", projectFilter = ""): SessionsListResult {
  const all = collectAllSessions(projectFilter);
  return {
    sessions: applySessionQuery(all, q),
    facets: facetsFrom(all),
  };
}

export function getSessionDetail(id: string, projectFilter = "") {
  const projects = projectsForFilter(projectFilter || "all");
  const candidates =
    projects.length > 0
      ? projects
      : [
          {
            id: "local",
            name: "local",
            repo_root: processRepoRoot(),
            so_root: join(processRepoRoot(), ".so"),
          },
        ];

  for (const p of candidates) {
    const so = p.so_root || join(p.repo_root, ".so");
    const sessionPath = join(so, "sessions", id);
    if (!fileExists(sessionPath)) continue;
    const meta =
      readJSON<SessionMeta>(join(sessionPath, "meta.json")) ||
      ({ id } as SessionMeta);
    if (!meta.id) meta.id = id;
    meta.project_id = p.id;
    meta.title = displayTitle(meta);
    meta.prompt_preview =
      humanizePromptPreview(meta.prompt_preview) || meta.prompt_preview;
    if (!meta.user) {
      const users = loadUserMap(so);
      meta.user = users.get(id) || undefined;
    }

    const footprint = readJSON(join(sessionPath, "footprint.json"));
    const transcriptPath = join(sessionPath, "transcript.jsonl");
    const transcript: unknown[] = [];
    if (fileExists(transcriptPath)) {
      const lines = readText(transcriptPath).split("\n");
      for (const line of lines) {
        if (!line.trim()) continue;
        try {
          transcript.push(JSON.parse(line));
        } catch {
          /* skip */
        }
      }
    }
    // Active Cursor sessions often have OTLP spans in .so/traces before finalize
    // writes transcript.jsonl - fall back so Chat UI is not empty mid-session.
    if (transcript.length === 0) {
      transcript.push(...loadLiveTraces(so, id));
    }

    const subagents: SessionMeta[] = [];
    const sessionsDir = join(so, "sessions");
    const links = loadAgentLinks(sessionsDir);
    if (fileExists(sessionsDir)) {
      for (const name of readdirSync(sessionsDir)) {
        if (name === "index.json" || name.startsWith(".") || name === id) continue;
        const childMeta = readJSON<SessionMeta>(
          join(sessionsDir, name, "meta.json")
        );
        if (!childMeta) continue;
        if (!childMeta.id) childMeta.id = name;
        const linkedParent = links[childMeta.id];
        if (
          childMeta.parent_id === id ||
          childMeta.parent_id === meta.id ||
          linkedParent === id
        ) {
          childMeta.project_id = p.id;
          childMeta.title = displayTitle(childMeta);
          childMeta.prompt_preview =
            humanizePromptPreview(childMeta.prompt_preview) ||
            childMeta.prompt_preview;
          if (!childMeta.title || childMeta.title === childMeta.id) {
            childMeta.title =
              humanizePromptPreview(childMeta.prompt_preview) ||
              `Subagent ${childMeta.id.slice(0, 8)}`;
          }
          subagents.push(childMeta);
        }
      }
      subagents.sort((a, b) =>
        String(b.started_at || "").localeCompare(String(a.started_at || ""))
      );
    }

    const stats = enrichSessionStats(so, sessionPath, meta);
    const evalPath = join(sessionPath, "eval.json");
    const evalResult = fileExists(evalPath) ? readJSON(evalPath) : null;
    return {
      meta,
      transcript,
      footprint,
      checkpoints: listCheckpoints(sessionPath, id),
      // Replay lives in Map / `so sessions` CLI - not duplicated in Chat.
      replay: undefined,
      eval: evalResult,
      subagents,
    };
  }
  return null;
}

export function sessionDir(id: string): string {
  return join(soRoot(), "sessions", id);
}
