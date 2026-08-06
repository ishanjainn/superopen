/**
 * Session search tokens.
 *
 *   from:@user   / from:user / user:…  → username / email
 *   agent:cursor / vendor:…            → coding agent (vendor)
 *   model:…                            → model id substring
 *   file:…                             → footprint / transcript file path
 *   tool:…                             → tool name (Bash, Read, Grep, …)
 *   remaining text                     → free-text haystack (title, files, chat, tools)
 */

export type SessionQuery = {
  text: string;
  user: string;
  agent: string;
  model: string;
  file: string;
  tool: string;
};

export type SessionQueryFacets = {
  users: string[];
  agents: string[];
  /** Footprint file basenames for `file:` autocomplete. */
  files?: string[];
};

const TOKEN_RE =
  /\b(from|user|agent|vendor|model|file|tool):(@?[^\s]+)/gi;

export function emptySessionQuery(): SessionQuery {
  return { text: "", user: "", agent: "", model: "", file: "", tool: "" };
}

export function parseSessionQuery(raw: string): SessionQuery {
  const out = emptySessionQuery();
  if (!raw.trim()) return out;
  const leftovers: string[] = [];
  let last = 0;
  const src = raw;
  TOKEN_RE.lastIndex = 0;
  let m: RegExpExecArray | null;
  while ((m = TOKEN_RE.exec(src))) {
    leftovers.push(src.slice(last, m.index));
    last = m.index + m[0].length;
    const key = m[1].toLowerCase();
    const val = m[2].replace(/^@/, "").trim();
    if (!val) continue;
    if (key === "from" || key === "user") out.user = val;
    else if (key === "agent" || key === "vendor") out.agent = val;
    else if (key === "model") out.model = val;
    else if (key === "file") out.file = val;
    else if (key === "tool") out.tool = val;
  }
  leftovers.push(src.slice(last));
  out.text = leftovers.join(" ").replace(/\s+/g, " ").trim();
  return out;
}

/** Reassemble a query string from structured filters + free text. */
export function serializeSessionQuery(q: SessionQuery): string {
  const parts: string[] = [];
  if (q.user) parts.push(`from:@${q.user}`);
  if (q.agent) parts.push(`agent:${q.agent}`);
  if (q.model) parts.push(`model:${q.model}`);
  if (q.file) parts.push(`file:${q.file}`);
  if (q.tool) parts.push(`tool:${q.tool}`);
  if (q.text.trim()) parts.push(q.text.trim());
  return parts.join(" ");
}

/** Detect an incomplete trailing token for autocomplete (e.g. `from:@ish`). */
export function incompleteSessionToken(raw: string): {
  key: "from" | "agent" | "model" | "file" | "tool";
  prefix: string;
  start: number;
} | null {
  const m = raw.match(
    /(?:^|\s)(from|user|agent|vendor|model|file|tool):(@?[^\s]*)$/i
  );
  if (!m || m.index === undefined) return null;
  const keyRaw = m[1].toLowerCase();
  const key =
    keyRaw === "from" || keyRaw === "user"
      ? "from"
      : keyRaw === "model"
        ? "model"
        : keyRaw === "file"
          ? "file"
          : keyRaw === "tool"
            ? "tool"
            : "agent";
  const tokenStart = m.index + (m[0].startsWith(" ") || m[0].startsWith("\t") ? 1 : 0);
  return {
    key,
    prefix: m[2].replace(/^@/, "").toLowerCase(),
    start: tokenStart,
  };
}

export function vendorMatchesAgent(vendor: string | undefined, agent: string): boolean {
  if (!agent) return true;
  const v = (vendor || "").toLowerCase();
  const a = agent.toLowerCase();
  if (!v) return false;
  if (v.includes(a) || a.includes(v)) return true;
  const aliases: Record<string, string[]> = {
    cursor: ["cursor"],
    claude: ["claude", "claude-code", "cc", "claudecode"],
    "claude code": ["claude", "claude-code", "cc"],
    codex: ["codex"],
    opencode: ["opencode"],
    pi: ["pi"],
    gemini: ["gemini"],
    copilot: ["copilot"],
  };
  for (const [label, ids] of Object.entries(aliases)) {
    if (label.includes(a) || a.includes(label) || ids.some((id) => id.includes(a) || a.includes(id))) {
      if (ids.some((id) => v.includes(id))) return true;
    }
  }
  return false;
}

export function userMatches(user: string | undefined, needle: string): boolean {
  if (!needle) return true;
  const u = (user || "").toLowerCase();
  const n = needle.toLowerCase();
  if (!u) return false;
  if (u.includes(n) || n.includes(u)) return true;
  const local = u.split("@")[0] || u;
  return local.includes(n) || n.includes(local);
}

export function displayUser(user?: string): string {
  if (!user) return "";
  const at = user.indexOf("@");
  if (at > 0 && at < user.length - 1) return user.slice(0, at);
  return user;
}
