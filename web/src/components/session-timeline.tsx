"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
  type RefObject,
} from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import {
  ChevronDown,
  ChevronUp,
  FileCode2,
  Terminal,
} from "lucide-react";
import { SessionRail } from "@/components/session-rail";
import { cn } from "@/lib/utils";

export type SessionMeta = {
  id: string;
  vendor?: string;
  model?: string;
  title?: string;
  prompt_preview?: string;
  status?: string;
  started_at?: string;
  ended_at?: string;
  duration_ms?: number;
  tokens?: number;
  cost_usd?: number;
  eval_badge?: string;
  branch?: string;
  head_sha?: string;
  commits?: { sha?: string; message?: string }[];
  pull_requests?: { url?: string; number?: number; title?: string }[];
  attribution?: { display?: string; agent_percent?: number };
  summary?: string;
};

export type RestoreCheckpoint = {
  id: number;
  session_id?: string;
  created_at?: string;
  label?: string;
  files?: string[];
};

export type Span = {
  name?: string;
  start_time_unix_nano?: number;
  end_time_unix_nano?: number;
  attributes?: Record<string, string>;
  /** Ported PortableTurn rows (so sessions port) - not OTLP spans. */
  role?: string;
  text?: string;
  timestamp?: number;
};

type FilterKey =
  | "prompts"
  | "responses"
  | "intermediate"
  | "turns"
  | "tools"
  | "edits"
  | "bash"
  | "subagents";

const DEFAULT_FILTERS: Record<FilterKey, boolean> = {
  prompts: true,
  responses: true,
  intermediate: false,
  turns: true,
  tools: true,
  edits: false,
  bash: false,
  subagents: true,
};

/** URL tokens for `?filters=` (thoughts ↔ intermediate). */
const FILTER_URL_KEYS: { key: FilterKey; token: string }[] = [
  { key: "prompts", token: "prompts" },
  { key: "responses", token: "responses" },
  { key: "intermediate", token: "thoughts" },
  { key: "tools", token: "tools" },
  { key: "edits", token: "edits" },
  { key: "bash", token: "bash" },
  { key: "subagents", token: "subagents" },
];

function filtersEqual(
  a: Record<FilterKey, boolean>,
  b: Record<FilterKey, boolean>
): boolean {
  return FILTER_URL_KEYS.every(({ key }) => a[key] === b[key]);
}

function encodeFiltersParam(f: Record<FilterKey, boolean>): string | null {
  if (filtersEqual(f, DEFAULT_FILTERS)) return null;
  return FILTER_URL_KEYS.filter(({ key }) => f[key])
    .map(({ token }) => token)
    .join(",");
}

function decodeFiltersParam(raw: string | null): Record<FilterKey, boolean> {
  if (!raw || !raw.trim()) return { ...DEFAULT_FILTERS };
  const enabled = new Set(
    raw
      .split(",")
      .map((s) => s.trim().toLowerCase())
      .filter(Boolean)
  );
  const next = { ...DEFAULT_FILTERS };
  for (const { key } of FILTER_URL_KEYS) next[key] = false;
  for (const { key, token } of FILTER_URL_KEYS) {
    if (enabled.has(token) || enabled.has(key)) next[key] = true;
  }
  return next;
}
type ToolEntry = {
  name: string;
  detail?: string;
  file?: string;
  kind: "edit" | "bash" | "other" | "subagent";
  added?: number;
  removed?: number;
  summary?: string;
  agentId?: string;
};

type NestedSessionSummary = {
  id: string;
  title?: string;
  prompt_preview?: string;
  vendor?: string;
  model?: string;
  status?: string;
  started_at?: string;
  ended_at?: string;
};

type TimelineItem =
  | { kind: "prompt"; id: string; at: number; text: string; model?: string }
  | {
      kind: "response";
      id: string;
      at: number;
      text: string;
      model?: string;
      asThought?: boolean;
      thoughtUnavailable?: boolean;
    }
  | { kind: "tools"; id: string; at: number; tools: ToolEntry[] }
  | { kind: "turn"; id: string; at: number; label: string }
  | {
      kind: "subagent";
      id: string;
      at: number;
      agentId?: string;
      label: string;
      detail?: string;
      child?: NestedSessionSummary;
    };

function parseMessagesJSON(raw?: string): { role: string; text: string }[] {
  if (!raw) return [];
  try {
    const data = JSON.parse(raw);
    if (!Array.isArray(data)) return [];
    const out: { role: string; text: string }[] = [];
    for (const msg of data) {
      const role = String(msg.role || msg.kind || "user");
      let text = "";
      if (typeof msg.content === "string") text = msg.content;
      else if (Array.isArray(msg.parts)) {
        text = msg.parts
          .map((p: any) => p?.content || p?.text || "")
          .filter(Boolean)
          .join("\n");
      } else if (typeof msg.text === "string") text = msg.text;
      if (text) out.push({ role, text });
    }
    return out;
  } catch {
    return [];
  }
}

function formatDuration(ms?: number) {
  if (!ms || ms <= 0) return "";
  if (ms < 60_000) return `${Math.round(ms / 1000)}s`;
  const m = Math.floor(ms / 60_000);
  const s = Math.round((ms % 60_000) / 1000);
  return s ? `${m}m ${s}s` : `${m}m`;
}

function basename(path: string) {
  const parts = path.replace(/\\/g, "/").split("/");
  return parts[parts.length - 1] || path;
}

function classifyTool(name: string): ToolEntry["kind"] {
  const n = name.toLowerCase();
  if (n === "task" || n === "agent" || n === "mcp_task") return "subagent";
  if (n.includes("bash") || n.includes("shell") || n === "shell") return "bash";
  if (n.includes("write") || n.includes("edit") || n.includes("apply")) return "edit";
  return "other";
}

function isTaskToolName(name: string): boolean {
  const n = name.toLowerCase().trim();
  return n === "task" || n === "agent" || n === "mcp_task";
}

function extractAgentID(...blobs: (string | undefined)[]): string {
  const re =
    /[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}/i;
  for (const blob of blobs) {
    if (!blob) continue;
    const m = blob.match(re);
    if (m) return m[0];
  }
  return "";
}

function isoToNano(iso?: string): number {
  if (!iso) return 0;
  const ms = Date.parse(iso);
  return Number.isFinite(ms) ? ms * 1e6 : 0;
}

/** True when transcript rows are PortableTurn JSONL from `so sessions port`. */
function looksLikePortableTurns(spans: Span[]): boolean {
  if (!spans.length) return false;
  let portable = 0;
  for (const sp of spans) {
    const role = String(sp.role || "").toLowerCase();
    const hasText = typeof sp.text === "string" && sp.text.trim() !== "";
    const hasOtlp =
      Boolean(sp.name) ||
      Boolean(sp.attributes && Object.keys(sp.attributes).length);
    if ((role === "user" || role === "assistant" || role === "system") && hasText && !hasOtlp) {
      portable++;
    }
  }
  return portable > 0 && portable >= spans.length / 2;
}

function buildTimelineFromPortableTurns(spans: Span[]): TimelineItem[] {
  const items: TimelineItem[] = [];
  let i = 0;
  for (const sp of spans) {
    const role = String(sp.role || "").toLowerCase();
    const text = String(sp.text || "").trim();
    if (!text) continue;
    const at =
      typeof sp.timestamp === "number" && sp.timestamp > 0
        ? sp.timestamp > 1e15
          ? sp.timestamp
          : sp.timestamp * 1e6
        : sp.start_time_unix_nano || 0;
    if (role === "assistant") {
      items.push({
        kind: "response",
        id: `port-resp-${i++}`,
        at,
        text,
        model: typeof (sp as { model?: string }).model === "string"
          ? (sp as { model?: string }).model
          : undefined,
      });
    } else if (role === "user" || role === "system" || role === "user_prompt") {
      items.push({
        kind: "prompt",
        id: `port-prompt-${i++}`,
        at,
        text,
      });
    }
  }
  return items;
}

function buildTimeline(spans: Span[]): TimelineItem[] {
  if (looksLikePortableTurns(spans)) {
    return buildTimelineFromPortableTurns(spans);
  }
  const items: TimelineItem[] = [];
  let i = 0;
  const pending: ToolEntry[] = [];
  let pendingAt = 0;

  const flushTools = () => {
    if (!pending.length) return;
    items.push({
      kind: "tools",
      id: `tools-${i++}`,
      at: pendingAt,
      tools: pending.splice(0, pending.length),
    });
  };

  for (const sp of spans) {
    const attrs = sp.attributes || {};
    const at = sp.start_time_unix_nano || 0;
    const name = sp.name || "";

    if (name.includes("llm.turn") || name.includes("prompt") || name.includes("completion")) {
      flushTools();
      const turnKind = attrs["coding_agent.llm.turn.kind"] || "";
      const inputMsgs = parseMessagesJSON(attrs["gen_ai.input.messages"]);
      const outputMsgs = parseMessagesJSON(attrs["gen_ai.output.messages"]);
      const rawPrompt =
        attrs["gen_ai.content.prompt"] || attrs["gen_ai.prompt"] || "";
      // Prefer structured messages; Cursor often stores the same JSON in gen_ai.prompt.
      const promptFromMsgs =
        inputMsgs.find((m) => m.role === "user" || m.role === "user_prompt")?.text ||
        (turnKind === "user_prompt" ? inputMsgs[0]?.text : "") ||
        "";
      const promptLooksJSON =
        rawPrompt.trim().startsWith("[") || rawPrompt.trim().startsWith("{");
      const prompt =
        promptFromMsgs ||
        (promptLooksJSON
          ? parseMessagesJSON(rawPrompt).find(
              (m) => m.role === "user" || m.role === "user_prompt"
            )?.text || ""
          : rawPrompt) ||
        "";
      const thought = attrs["coding_agent.llm.thought.text"] || "";
      const reasoningSummaryUnavailable =
        attrs["coding_agent.llm.reasoning.summary.available"] === "false";
      const reasoningTokens = attrs["coding_agent.llm.reasoning.tokens"] || "";
      const response =
        attrs["gen_ai.content.completion"] ||
        attrs["gen_ai.completion"] ||
        attrs["gen_ai.response.output"] ||
        outputMsgs.find((m) => m.role === "assistant")?.text ||
        outputMsgs[0]?.text ||
        "";
      const model = attrs["gen_ai.request.model"] || attrs["gen_ai.response.model"];

      if (prompt && turnKind !== "assistant_only") {
        const promptId = `prompt-${i++}`;
        items.push({ kind: "prompt", id: promptId, at, text: prompt, model });
        items.push({
          kind: "turn",
          id: `turn-${promptId}`,
          at: at + 1,
          label: prompt.slice(0, 72),
        });
      }
      const agentText = response || thought;
      const asThought = !response && Boolean(thought);
      if (agentText) {
        const last = items[items.length - 1];
        if (!(last && last.kind === "response" && last.text === agentText)) {
          items.push({
            kind: "response",
            id: `resp-${i++}`,
            at: (sp.end_time_unix_nano || at) + 1,
            text: agentText,
            model,
            asThought,
          });
        }
      }
      if (response && thought && thought !== response) {
        items.push({
          kind: "response",
          id: `thought-${i++}`,
          at: (sp.end_time_unix_nano || at) + 2,
          text: thought,
          model,
          asThought: true,
        });
      }
      if (reasoningSummaryUnavailable && !thought) {
        const tokenDetail = reasoningTokens
          ? ` (${Number(reasoningTokens).toLocaleString()} reasoning tokens)`
          : "";
        items.push({
          kind: "response",
          id: `thought-unavailable-${i++}`,
          at: (sp.end_time_unix_nano || at) + 2,
          text: `Codex used reasoning for this turn${tokenDetail}, but did not expose a public reasoning summary.`,
          model,
          asThought: true,
          thoughtUnavailable: true,
        });
      }
      continue;
    }

    if (name.includes("model.changed") || name.includes("permission_mode") || name.includes("loop.stop")) {
      continue;
    }

    if (name.includes("edit")) {
      const path = attrs["code.file.path"] || attrs["coding_agent.file_path"] || "file";
      const added = parseInt(attrs["coding_agent.edit.lines.added"] || "", 10);
      const removed = parseInt(attrs["coding_agent.edit.lines.removed"] || "", 10);
      const toolName =
        attrs["coding_agent.edit.tool.name"] ||
        attrs["gen_ai.tool.name"] ||
        "File edit";
      if (!pending.length) pendingAt = at;
      const prev = [...pending]
        .reverse()
        .find((t) => t.kind === "edit" && (!t.file || t.file === path));
      if (prev && (!prev.file || prev.file === path) && prev.added == null && prev.removed == null) {
        prev.file = path;
        prev.name = toolName.includes("afterFileEdit") ? "Write" : toolName;
        if (!Number.isNaN(added)) prev.added = added;
        if (!Number.isNaN(removed)) prev.removed = removed;
        prev.summary = basename(path);
        if (attrs["coding_agent.edit.decision"]) {
          prev.detail = `decision: ${attrs["coding_agent.edit.decision"]}`;
        }
      } else {
        pending.push({
          name: toolName.includes("afterFileEdit") ? "Write" : toolName,
          file: path,
          summary: basename(path),
          detail: attrs["coding_agent.edit.decision"]
            ? `decision: ${attrs["coding_agent.edit.decision"]}`
            : undefined,
          kind: "edit",
          added: Number.isNaN(added) ? undefined : added,
          removed: Number.isNaN(removed) ? undefined : removed,
        });
      }
      continue;
    }

    if (name.includes("tool") || name.includes("bash") || name.includes("shell")) {
      const isShell = name.includes("shell") || name.includes("bash");
      const toolName =
        attrs["gen_ai.tool.name"] ||
        attrs["coding_agent.tool.name"] ||
        (isShell ? "Bash" : name);
      const command =
        attrs["coding_agent.tool.command"] ||
        attrs["gen_ai.tool.call.arguments"];
      const detail =
        attrs["gen_ai.tool.call.result"] ||
        command ||
        attrs["coding_agent.tool.arguments"];
      const file = attrs["code.file.path"] || attrs["coding_agent.file_path"];

      if (isTaskToolName(toolName)) {
        flushTools();
        const agentId = extractAgentID(
          attrs["coding_agent.agent.id"],
          attrs["coding_agent.subagent.id"],
          detail,
          command
        );
        items.push({
          kind: "subagent",
          id: `subagent-${i++}`,
          at,
          agentId: agentId || undefined,
          label: firstLine(command || toolName) || "Subagent",
          detail,
        });
        continue;
      }

      if (!pending.length) pendingAt = at;
      pending.push({
        name: toolName,
        detail,
        file,
        summary: isShell
          ? firstLine(command || toolName)
          : file
            ? basename(file)
            : firstLine(command || toolName),
        kind: classifyTool(toolName),
      });
      continue;
    }
  }
  flushTools();
  return items.sort((a, b) => a.at - b.at);
}

function firstLine(s: string) {
  const line = s.split("\n").find((l) => l.trim()) || s;
  return line.length > 72 ? `${line.slice(0, 72)}…` : line;
}

function mergeSubagentsIntoTimeline(
  items: TimelineItem[],
  subagents: NestedSessionSummary[]
): TimelineItem[] {
  if (!subagents.length && !items.some((it) => it.kind === "subagent")) {
    return items;
  }
  const claimed = new Set<string>();
  const out: TimelineItem[] = items.map((it) => {
    if (it.kind !== "subagent") return it;
    const byId = it.agentId
      ? subagents.find((s) => s.id === it.agentId)
      : undefined;
    const child =
      byId ||
      subagents.find((s) => !claimed.has(s.id));
    if (child) claimed.add(child.id);
    return {
      ...it,
      agentId: it.agentId || child?.id,
      child,
      label:
        child?.title ||
        child?.prompt_preview ||
        it.label ||
        child?.id ||
        "Subagent",
    };
  });

  for (const child of subagents) {
    if (claimed.has(child.id)) continue;
    out.push({
      kind: "subagent",
      id: `subagent-meta-${child.id}`,
      at: isoToNano(child.started_at) || Number.MAX_SAFE_INTEGER / 2,
      agentId: child.id,
      label: child.title || child.prompt_preview || child.id,
      child,
    });
  }
  return out.sort((a, b) => a.at - b.at);
}

function vendorFromMeta(
  meta: SessionMeta,
  spans: Span[]
): "cursor" | "claude" | "codex" | "opencode" | "pi" | "other" {
  const raw = (
    meta.vendor ||
    spans.map((s) => s.attributes?.["coding_agent.client"] || s.attributes?.["coding_agent.vendor"]).find(Boolean) ||
    ""
  ).toLowerCase();
  if (raw.includes("cursor")) return "cursor";
  if (raw.includes("claude")) return "claude";
  if (raw.includes("codex")) return "codex";
  if (raw.includes("opencode")) return "opencode";
  if (raw === "pi") return "pi";
  return "other";
}

function userLabel(spans: Span[]): string {
  const email =
    spans.map((s) => s.attributes?.["gen_ai.user.name"]).find(Boolean) || "";
  if (email.includes("@")) return email.split("@")[0];
  return email || "You";
}

function userInitials(spans: Span[]): string {
  const label = userLabel(spans);
  const parts = label.replace(/[._-]+/g, " ").split(/\s+/).filter(Boolean);
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase();
  return label.slice(0, 2).toUpperCase() || "U";
}

export default function SessionTimeline({
  meta,
  spans,
  footprint,
  restoreCheckpoints = [],
  subagents = [],
  project = "",
}: {
  meta: SessionMeta;
  spans: Span[];
  footprint?: { files?: { path: string; state: string; count: number }[] };
  restoreCheckpoints?: RestoreCheckpoint[];
  subagents?: NestedSessionSummary[];
  project?: string;
}) {
  const rawItems = useMemo(() => buildTimeline(spans), [spans]);
  const items = useMemo(
    () => mergeSubagentsIntoTimeline(rawItems, subagents),
    [rawItems, subagents]
  );
  const vendor = useMemo(() => vendorFromMeta(meta, spans), [meta, spans]);
  const initials = useMemo(() => userInitials(spans), [spans]);
  const feedRef = useRef<HTMLDivElement>(null);
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();

  const filters = useMemo(
    () => decodeFiltersParam(searchParams.get("filters")),
    [searchParams]
  );
  const [toolsOpen, setToolsOpen] = useState<Record<string, boolean>>({});

  const updateFilters = useCallback(
    (patch: Partial<Record<FilterKey, boolean>>) => {
      const next = { ...filters, ...patch };
      const params = new URLSearchParams(searchParams.toString());
      const encoded = encodeFiltersParam(next);
      if (encoded) params.set("filters", encoded);
      else params.delete("filters");
      const qs = params.toString().replace(/=(?=&|$)/g, "");
      const href = qs ? `${pathname}?${qs}` : pathname;
      router.replace(href, { scroll: false });
    },
    [filters, pathname, router, searchParams]
  );
  const counts = useMemo(() => {
    const c = {
      prompts: 0,
      responses: 0,
      intermediate: 0,
      turns: 0,
      tools: 0,
      toolCalls: 0,
      edits: 0,
      bash: 0,
      subagents: 0,
    };
    for (const it of items) {
      if (it.kind === "prompt") c.prompts++;
      else if (it.kind === "response") {
        if (it.asThought) c.intermediate++;
        else c.responses++;
      } else if (it.kind === "turn") c.turns++;
      else if (it.kind === "subagent") c.subagents++;
      else if (it.kind === "tools") {
        c.tools++;
        c.toolCalls += it.tools.length;
        for (const t of it.tools) {
          if (t.kind === "edit") c.edits++;
          if (t.kind === "bash") c.bash++;
          if (t.kind === "subagent") c.subagents++;
        }
      }
    }
    return c;
  }, [items]);

  const turns = useMemo(
    () => items.filter((it): it is Extract<TimelineItem, { kind: "turn" }> => it.kind === "turn"),
    [items]
  );

  const visible = items.filter((it) => {
    if (it.kind === "prompt") return filters.prompts;
    if (it.kind === "turn") return false; // sidebar only
    if (it.kind === "subagent") return filters.subagents;
    if (it.kind === "response") {
      if (it.asThought) return filters.intermediate;
      return filters.responses;
    }
    if (it.kind === "tools") {
      const filteredTools = it.tools.filter((t) => {
        if (t.kind === "subagent") return filters.subagents;
        if (filters.tools) return true;
        if (filters.edits && t.kind === "edit") return true;
        if (filters.bash && t.kind === "bash") return true;
        return false;
      });
      return filteredTools.length > 0;
    }
    return false;
  });

  const minimapMarks = useMemo(
    () =>
      visible.map((it): MinimapMark => {
        if (it.kind === "prompt") return { id: it.id, tone: "prompt" };
        if (it.kind === "response")
          return { id: it.id, tone: it.asThought ? "thought" : "response" };
        if (it.kind === "subagent") return { id: it.id, tone: "subagent" };
        return { id: it.id, tone: "tools" };
      }),
    [visible]
  );

  const jumpTo = (id: string) => {
    const el = document.getElementById(`tl-${id}`);
    el?.scrollIntoView({ behavior: "smooth", block: "center" });
  };

  const coverage = useMemo(() => {
    const files = footprint?.files || [];
    let edited = 0;
    let read = 0;
    let seen = 0;
    for (const f of files) {
      const st = String(f.state || "").toLowerCase();
      if (st.includes("edit")) edited++;
      else if (st.includes("read")) read++;
      else if (st) seen++;
    }
    return {
      edited,
      read,
      seen,
      files: files.length,
      unvisited: Math.max(0, files.length - edited - read - seen),
    };
  }, [footprint]);

  const commits = meta.commits || [];
  const prs = meta.pull_requests || [];
  const hasLinked =
    commits.length > 0 ||
    prs.length > 0 ||
    Boolean(meta.branch) ||
    Boolean(meta.attribution?.display);

  return (
    <div className="flex h-full min-h-0 flex-col bg-white">
      <div className="flex min-h-0 flex-1">
        <div className="relative min-h-0 min-w-0 flex-1">
          <div
            ref={feedRef}
            className="h-full overflow-y-auto px-6 py-6 [scrollbar-width:none] [-ms-overflow-style:none] [&::-webkit-scrollbar]:hidden"
          >
            <div className="mx-auto max-w-2xl">
              {visible.length === 0 && (
                <p className="text-sm text-neutral-500">
                  {items.length === 0
                    ? "Session data is still arriving. Use Refresh to check for new turns."
                    : "Nothing matches the current filters."}
                </p>
              )}
              {visible.map((it, index) => {
                const isFirst = index === 0;
                const isLast = index === visible.length - 1;
                if (it.kind === "prompt") {
                  return (
                    <TimelineStep
                      key={it.id}
                      id={it.id}
                      icon={<UserAvatar initials={initials} />}
                      isFirst={isFirst}
                      isLast={isLast}
                    >
                      <div className="rounded-2xl bg-neutral-100 px-4 py-3">
                        <p className="whitespace-pre-wrap text-[15px] leading-relaxed text-neutral-900">
                          {it.text}
                        </p>
                        <p className="mt-2 text-xs text-neutral-400">
                          {[formatDuration(meta.duration_ms), meta.tokens ? `${meta.tokens} tok` : ""]
                            .filter(Boolean)
                            .join(" · ")}
                        </p>
                      </div>
                    </TimelineStep>
                  );
                }
                if (it.kind === "response") {
                  return (
                    <TimelineStep
                      key={it.id}
                      id={it.id}
                      icon={<VendorAvatar vendor={vendor} />}
                      isFirst={isFirst}
                      isLast={isLast}
                    >
                      <div
                        className={cn(
                          "px-4 py-3",
                          it.asThought
                            ? "rounded-2xl border border-amber-200/60 bg-amber-50/80 dark:border-amber-500/25 dark:bg-amber-950/45"
                            : "rounded-2xl bg-neutral-50"
                        )}
                      >
                        {it.asThought && (
                          <p className="mb-1.5 text-[11px] font-medium uppercase tracking-wide text-amber-800/80 dark:text-amber-300/90">
                            {it.thoughtUnavailable ? "Reasoning" : "Thought"}
                          </p>
                        )}
                        <p
                          className={cn(
                            "whitespace-pre-wrap text-[15px] leading-relaxed",
                            it.thoughtUnavailable
                              ? "text-neutral-500"
                              : "text-neutral-800"
                          )}
                        >
                          {it.text}
                        </p>
                      </div>
                    </TimelineStep>
                  );
                }
                if (it.kind === "subagent") {
                  return (
                    <TimelineStep
                      key={it.id}
                      id={it.id}
                      icon={<SubagentAvatar />}
                      isFirst={isFirst}
                      isLast={isLast}
                    >
                      <SubagentCard
                        item={it}
                        project={project}
                      />
                    </TimelineStep>
                  );
                }
                if (it.kind === "tools") {
                  const open = toolsOpen[it.id] ?? false;
                  const filteredTools = it.tools.filter((t) => {
                    if (filters.tools) return true;
                    if (filters.edits && t.kind === "edit") return true;
                    if (filters.bash && t.kind === "bash") return true;
                    return false;
                  });
                  if (!filteredTools.length) return null;
                  const headline = toolGroupLabel(filteredTools);
                  const toggle = () =>
                    setToolsOpen((s) => ({ ...s, [it.id]: !open }));
                  return (
                    <TimelineStep
                      key={it.id}
                      id={it.id}
                      icon={<ToolAvatar />}
                      isFirst={isFirst}
                      isLast={isLast}
                    >
                      <div className="overflow-hidden rounded-2xl border border-neutral-200 bg-white">
                        <button
                          type="button"
                          onClick={toggle}
                          className={cn(
                            "flex w-full items-center gap-2 px-4 py-3 text-left text-sm text-neutral-800 hover:bg-neutral-50",
                            open && "border-b border-neutral-100"
                          )}
                        >
                          <span className="min-w-0 flex-1 truncate font-medium">
                            {headline}
                          </span>
                          {open ? (
                            <ChevronUp className="size-4 shrink-0 text-neutral-400" />
                          ) : (
                            <ChevronDown className="size-4 shrink-0 text-neutral-400" />
                          )}
                        </button>
                        {open ? (
                          <ul className="divide-y divide-neutral-100">
                            {filteredTools.map((t, idx) => (
                              <li key={idx} className="px-4 py-2.5">
                                <div className="flex items-start gap-2">
                                  <span className="mt-0.5 text-neutral-400">
                                    {t.kind === "bash" ? (
                                      <Terminal className="size-3.5" />
                                    ) : (
                                      <FileCode2 className="size-3.5" />
                                    )}
                                  </span>
                                  <div className="min-w-0 flex-1">
                                    <div className="flex items-baseline gap-2">
                                      <span className="truncate text-sm font-medium text-neutral-900">
                                        {t.summary || t.name}
                                      </span>
                                      <span className="shrink-0 text-[11px] text-neutral-400">
                                        {t.name}
                                      </span>
                                      <span className="ml-auto shrink-0 font-mono text-[11px]">
                                        {t.added != null && (
                                          <span className="text-emerald-600">
                                            +{t.added}
                                          </span>
                                        )}
                                        {t.added != null &&
                                          t.removed != null &&
                                          " "}
                                        {t.removed != null && (
                                          <span className="text-red-500">
                                            -{t.removed}
                                          </span>
                                        )}
                                      </span>
                                    </div>
                                    {t.file &&
                                      t.summary !== basename(t.file) && (
                                        <p className="mt-0.5 truncate text-xs text-neutral-500">
                                          {t.file}
                                        </p>
                                      )}
                                    {t.detail && (
                                      <pre className="mt-1 max-h-28 overflow-auto whitespace-pre-wrap text-[11px] text-neutral-500 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
                                        {t.detail.slice(0, 2000)}
                                      </pre>
                                    )}
                                  </div>
                                </div>
                              </li>
                            ))}
                          </ul>
                        ) : null}
                      </div>
                    </TimelineStep>
                  );
                }
                return null;
              })}
            </div>
          </div>
          <ChatMinimap
            scrollRef={feedRef}
            marks={minimapMarks}
            revision={toolsOpen}
          />
        </div>

        <div className="hidden lg:flex">
          <SessionRail aria-label="Session chat rail">
            <div className="tb-band">
              <div className="tb-cell tb-shrink">
                <span className="tb-label">Model</span>
                <span className="tb-value tb-mono">
                  {meta.model || meta.vendor || "-"}
                </span>
              </div>

              {hasLinked ? (
                <div className="tb-cell tb-shrink">
                  <span className="tb-label">Linked</span>
                  <span className="tb-value tb-mono tb-activity">
                    {meta.branch ? <span>branch {meta.branch}</span> : null}
                    {meta.attribution?.display ? (
                      <span>{meta.attribution.display}</span>
                    ) : null}
                    {commits.slice(0, 4).map((c) => (
                      <span key={c.sha || c.message} title={c.message}>
                        {(c.sha || "").slice(0, 7)}
                        {c.message ? ` · ${c.message.slice(0, 24)}` : ""}
                      </span>
                    ))}
                    {prs.slice(0, 2).map((pr) => (
                      <span key={pr.url || String(pr.number)}>
                        {pr.url ? (
                          <a href={pr.url} target="_blank" rel="noreferrer">
                            {pr.title || `PR #${pr.number}`}
                          </a>
                        ) : (
                          `PR #${pr.number}`
                        )}
                      </span>
                    ))}
                  </span>
                </div>
              ) : null}

              <div className="tb-cell tb-shrink">
                <span className="tb-label">Turns</span>
                {turns.length === 0 ? (
                  <span className="tb-jump-empty">No turns yet</span>
                ) : (
                  <ul className="tb-jump-list">
                    {turns.map((cp, index) => (
                      <li key={cp.id}>
                        <button
                          type="button"
                          className="tb-jump-row"
                          onClick={() => jumpTo(cp.id.replace(/^turn-/, ""))}
                        >
                          <span className="tb-jump-row-index">{index + 1}</span>
                          <span className="tb-jump-row-text">{cp.label}</span>
                        </button>
                      </li>
                    ))}
                  </ul>
                )}
              </div>

              <div className="tb-cell tb-spectrum tb-shrink">
                <span className="tb-label">Coverage</span>
                <div className="tb-spectrum-row">
                  <div className="tb-stat">
                    <span className="legend-dot edit" />
                    <span className="tb-stat-label">Edited</span>
                    <strong className="tb-mono">{coverage.edited}</strong>
                  </div>
                  <div className="tb-stat">
                    <span className="legend-dot read" />
                    <span className="tb-stat-label">Read</span>
                    <strong className="tb-mono">{coverage.read}</strong>
                  </div>
                  <div className="tb-stat">
                    <span className="legend-dot hit" />
                    <span className="tb-stat-label">Seen</span>
                    <strong className="tb-mono">{coverage.seen}</strong>
                  </div>
                  <div className="tb-stat">
                    <span className="legend-dot unvisited" />
                    <span className="tb-stat-label">Touched</span>
                    <strong className="tb-mono">{coverage.files}</strong>
                  </div>
                </div>
              </div>

              <div className="tb-cell tb-shrink">
                <span className="tb-label">Activity</span>
                <span className="tb-value tb-mono tb-activity">
                  <span>{counts.toolCalls} calls</span>
                  <span>{counts.turns} turns</span>
                  {subagents.length > 0 ? (
                    <span>
                      {subagents.length} subagent
                      {subagents.length === 1 ? "" : "s"}
                    </span>
                  ) : null}
                  <span>{counts.edits} edits</span>
                </span>
              </div>

              <div className="tb-cell tb-shrink">
                <span className="tb-label">Files</span>
                <span className="tb-value tb-mono">
                  {coverage.files || "-"}
                </span>
              </div>

              <div className="tb-cell tb-shrink">
                <span className="tb-label">Session</span>
                <span className="tb-value tb-mono tb-session-id">
                  {meta.id.length > 12 ? `${meta.id.slice(0, 8)}…` : meta.id}
                </span>
              </div>

              <div className="tb-cell tb-shrink">
                <span className="tb-label">Checkpoints</span>
                {restoreCheckpoints.length === 0 ? (
                  <span className="tb-value tb-mono tb-muted" title="Created on git commit / finalize">
                    None yet
                  </span>
                ) : (
                  <span className="tb-value tb-mono tb-activity">
                    {restoreCheckpoints.slice(0, 5).map((cp) => (
                      <span key={cp.id} title={(cp.files || []).join(", ")}>
                        #{cp.id}
                        {cp.label ? ` · ${cp.label}` : ""}
                        {cp.files?.length ? ` · ${cp.files.length}f` : ""}
                      </span>
                    ))}
                  </span>
                )}
              </div>

              <div className="tb-cell tb-grow">
                <span className="tb-label">Filters</span>
                <div className="tb-filter-list" style={{ padding: 0 }}>
                  <FilterRow
                    label="Prompts"
                    count={counts.prompts}
                    checked={filters.prompts}
                    onChange={(v) => updateFilters({ prompts: v })}
                  />
                  <FilterRow
                    label="Responses"
                    count={counts.responses}
                    checked={filters.responses}
                    onChange={(v) => updateFilters({ responses: v })}
                  />
                  <FilterRow
                    label="Thoughts"
                    count={counts.intermediate}
                    checked={filters.intermediate}
                    onChange={(v) => updateFilters({ intermediate: v })}
                  />
                  <FilterRow
                    label="Tool calls"
                    count={counts.toolCalls}
                    checked={filters.tools}
                    onChange={(v) => updateFilters({ tools: v })}
                  />
                  <div className="tb-filter-nest">
                    <FilterRow
                      label="Edits"
                      count={counts.edits}
                      checked={filters.edits}
                      onChange={(v) => updateFilters({ edits: v })}
                    />
                    <FilterRow
                      label="Bash"
                      count={counts.bash}
                      checked={filters.bash}
                      onChange={(v) => updateFilters({ bash: v })}
                    />
                  </div>
                  <FilterRow
                    label="Subagents"
                    count={counts.subagents}
                    checked={filters.subagents}
                    onChange={(v) => updateFilters({ subagents: v })}
                  />
                </div>
              </div>
            </div>
          </SessionRail>
        </div>
      </div>
    </div>
  );
}

type MinimapTone = "prompt" | "response" | "thought" | "tools" | "subagent";

type MinimapMark = {
  id: string;
  tone: MinimapTone;
};

const MINIMAP_TONE: Record<MinimapTone, string> = {
  prompt: "bg-neutral-400",
  response: "bg-neutral-200",
  thought: "bg-amber-300",
  tools: "bg-sky-300",
  subagent: "bg-teal-400",
};

function ChatMinimap({
  scrollRef,
  marks,
  revision,
}: {
  scrollRef: RefObject<HTMLDivElement | null>;
  marks: MinimapMark[];
  revision?: unknown;
}) {
  const trackRef = useRef<HTMLDivElement | null>(null);
  const dragging = useRef(false);
  const [metrics, setMetrics] = useState({
    scrollTop: 0,
    scrollHeight: 1,
    clientHeight: 1,
  });
  const [segments, setSegments] = useState<
    { id: string; tone: MinimapTone; top: number; height: number }[]
  >([]);

  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;

    const measure = () => {
      const scrollHeight = Math.max(el.scrollHeight, 1);
      const clientHeight = el.clientHeight;
      const elRect = el.getBoundingClientRect();
      const next: { id: string; tone: MinimapTone; top: number; height: number }[] =
        [];
      for (const mark of marks) {
        const node = document.getElementById(`tl-${mark.id}`);
        if (!node) continue;
        const nodeRect = node.getBoundingClientRect();
        const top = nodeRect.top - elRect.top + el.scrollTop;
        const height = Math.max(nodeRect.height, 8);
        next.push({
          id: mark.id,
          tone: mark.tone,
          top: top / scrollHeight,
          height: height / scrollHeight,
        });
      }
      setSegments(next);
      setMetrics({
        scrollTop: el.scrollTop,
        scrollHeight,
        clientHeight,
      });
    };

    let raf = 0;
    const schedule = () => {
      if (raf) return;
      raf = requestAnimationFrame(() => {
        raf = 0;
        measure();
      });
    };

    measure();
    el.addEventListener("scroll", schedule, { passive: true });
    const ro = new ResizeObserver(schedule);
    ro.observe(el);
    if (el.firstElementChild) ro.observe(el.firstElementChild);
    const mo = new MutationObserver(schedule);
    mo.observe(el, { childList: true, subtree: true });
    return () => {
      el.removeEventListener("scroll", schedule);
      if (raf) cancelAnimationFrame(raf);
      ro.disconnect();
      mo.disconnect();
    };
  }, [scrollRef, marks, revision]);

  const overflow =
    metrics.scrollHeight > metrics.clientHeight + 24 && marks.length > 0;
  if (!overflow) return null;

  const thumbTop = metrics.scrollTop / metrics.scrollHeight;
  const thumbHeight = metrics.clientHeight / metrics.scrollHeight;

  const scrollToRatio = (ratio: number) => {
    const el = scrollRef.current;
    if (!el) return;
    const max = el.scrollHeight - el.clientHeight;
    el.scrollTop = Math.max(0, Math.min(max, ratio * el.scrollHeight));
  };

  const pointerToRatio = (clientY: number) => {
    const track = trackRef.current;
    if (!track) return 0;
    const rect = track.getBoundingClientRect();
    const y = (clientY - rect.top) / Math.max(rect.height, 1);
    return Math.max(0, Math.min(1, y - thumbHeight / 2));
  };

  return (
    <div
      className="pointer-events-none absolute bottom-3 right-3 z-20"
      aria-hidden={false}
    >
      <div
        ref={trackRef}
        role="scrollbar"
        aria-valuenow={Math.round(thumbTop * 100)}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-orientation="vertical"
        aria-label="Chat position"
        title="Chat overview - click or drag to jump"
        className="pointer-events-auto relative h-28 w-3.5 cursor-pointer overflow-hidden rounded-full border border-neutral-200/90 bg-white/90 shadow-sm backdrop-blur-sm"
        onPointerDown={(e) => {
          e.preventDefault();
          dragging.current = true;
          (e.currentTarget as HTMLDivElement).setPointerCapture(e.pointerId);
          scrollToRatio(pointerToRatio(e.clientY));
        }}
        onPointerMove={(e) => {
          if (!dragging.current) return;
          scrollToRatio(pointerToRatio(e.clientY));
        }}
        onPointerUp={(e) => {
          dragging.current = false;
          try {
            (e.currentTarget as HTMLDivElement).releasePointerCapture(e.pointerId);
          } catch {
            /* ignore */
          }
        }}
        onPointerCancel={() => {
          dragging.current = false;
        }}
      >
        {segments.map((seg) => (
          <div
            key={seg.id}
            className={cn(
              "absolute left-0.5 right-0.5 rounded-[1px] opacity-80",
              MINIMAP_TONE[seg.tone]
            )}
            style={{
              top: `${seg.top * 100}%`,
              height: `${Math.max(seg.height * 100, 1.5)}%`,
            }}
          />
        ))}
        <div
          className="absolute left-0 right-0 rounded-full border border-neutral-400/70 bg-neutral-900/10"
          style={{
            top: `${thumbTop * 100}%`,
            height: `${Math.max(thumbHeight * 100, 8)}%`,
          }}
        />
      </div>
    </div>
  );
}

function TimelineStep({
  id,
  icon,
  isLast,
  children,
}: {
  id: string;
  icon: ReactNode;
  isFirst?: boolean;
  isLast: boolean;
  children: ReactNode;
}) {
  return (
    <div id={`tl-${id}`} className="relative flex items-stretch gap-3">
      <div className="flex w-8 shrink-0 flex-col items-center self-stretch">
        <div className="relative z-10 shrink-0">{icon}</div>
        {!isLast ? (
          <div className="w-px flex-1 bg-neutral-300" aria-hidden />
        ) : null}
      </div>
      <div className="min-w-0 flex-1 pb-6">{children}</div>
    </div>
  );
}

function toolGroupLabel(tools: ToolEntry[]): string {
  if (tools.length === 1) {
    const t = tools[0];
    return t.summary ? `${t.name}: ${t.summary}` : t.name;
  }
  const edits = tools.filter((t) => t.kind === "edit").length;
  const bash = tools.filter((t) => t.kind === "bash").length;
  const parts: string[] = [];
  if (edits) parts.push(`${edits} edit${edits === 1 ? "" : "s"}`);
  if (bash) parts.push(`${bash} bash`);
  const other = tools.length - edits - bash;
  if (other > 0) parts.push(`${other} other`);
  if (parts.length) return `${tools.length} tool calls · ${parts.join(", ")}`;
  return `${tools.length} tool calls`;
}

function UserAvatar({ initials }: { initials: string }) {
  const [imgOk, setImgOk] = useState(true);
  if (imgOk) {
    return (
      <div
        className="size-8 shrink-0 overflow-hidden rounded-full bg-neutral-200"
        title="You"
      >
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img
          src="/vendors/user.png"
          alt=""
          className="size-full object-cover"
          onError={() => setImgOk(false)}
        />
      </div>
    );
  }
  return (
    <div
      className="flex size-8 shrink-0 items-center justify-center rounded-full bg-neutral-800 text-[11px] font-semibold text-white"
      title="You"
    >
      {initials}
    </div>
  );
}

function VendorAvatar({
  vendor,
}: {
  vendor: "cursor" | "claude" | "codex" | "opencode" | "pi" | "other";
}) {
  const src =
    vendor === "claude"
      ? "/vendors/claude.png"
      : vendor === "codex"
        ? "/vendors/codex.png"
        : vendor === "cursor"
          ? "/vendors/cursor.png"
          : vendor === "opencode"
            ? "/vendors/opencode.svg"
            : vendor === "pi"
              ? "/vendors/pi.png"
              : null;

  const title =
    vendor === "claude"
      ? "Claude"
      : vendor === "codex"
        ? "Codex"
        : vendor === "cursor"
          ? "Cursor"
          : vendor === "opencode"
            ? "OpenCode"
            : vendor === "pi"
              ? "Pi"
              : "AI";

  if (src) {
    return (
      <div
        className={cn(
          "flex size-8 shrink-0 items-center justify-center overflow-hidden rounded-full",
          vendor === "claude"
            ? "bg-neutral-950"
            : vendor === "codex"
              ? "bg-white"
              : vendor === "opencode"
                ? "bg-neutral-100 dark:bg-neutral-900"
                : "bg-neutral-900"
        )}
        title={title}
      >
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img
          src={src}
          alt=""
          className={cn(
            "object-contain",
            vendor === "claude" ? "size-7" : "size-8",
            vendor === "opencode" && "p-1.5 dark:invert"
          )}
        />
      </div>
    );
  }
  return (
    <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-neutral-200 text-[10px] font-bold text-neutral-700">
      AI
    </div>
  );
}

function ToolAvatar() {
  return (
    <div
      className="flex size-8 shrink-0 items-center justify-center rounded-full border border-neutral-200 bg-white text-neutral-500"
      aria-hidden
    >
      <Terminal className="size-3.5" />
    </div>
  );
}

function SubagentAvatar() {
  return (
    <div className="flex size-8 shrink-0 items-center justify-center rounded-full border border-violet-200 bg-violet-50 text-violet-600">
      <UsersIcon />
    </div>
  );
}

function UsersIcon() {
  return (
    <svg
      viewBox="0 0 24 24"
      className="size-3.5"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.75"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
      <circle cx="9" cy="7" r="4" />
      <path d="M22 21v-2a4 4 0 0 0-3-3.87" />
      <path d="M16 3.13a4 4 0 0 1 0 7.75" />
    </svg>
  );
}

function SubagentCard({
  item,
  project,
}: {
  item: Extract<TimelineItem, { kind: "subagent" }>;
  project: string;
}) {
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [nestedSpans, setNestedSpans] = useState<Span[] | null>(null);
  const [nestedMeta, setNestedMeta] = useState<SessionMeta | null>(null);
  const childId = item.child?.id || item.agentId;

  useEffect(() => {
    if (!open || !childId || nestedSpans) return;
    let cancelled = false;
    setLoading(true);
    setError("");
    const qs = project ? `?project=${encodeURIComponent(project)}` : "";
    fetch(`/api/sessions/${encodeURIComponent(childId)}${qs}`)
      .then(async (r) => {
        if (!r.ok) throw new Error(await r.text());
        return r.json();
      })
      .then((data) => {
        if (cancelled) return;
        setNestedMeta(data.meta || { id: childId });
        setNestedSpans(Array.isArray(data.transcript) ? data.transcript : []);
      })
      .catch((e) => {
        if (!cancelled) setError(String(e.message || e));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [open, childId, project, nestedSpans]);

  const title =
    item.child?.title ||
    item.child?.prompt_preview ||
    item.label ||
    childId ||
    "Subagent";
  const status = item.child?.status || "subagent";

  return (
    <div className="overflow-hidden rounded-2xl border border-violet-200/80 bg-white">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className={cn(
          "flex w-full items-center gap-3 px-4 py-3 text-left hover:bg-violet-50/50",
          open && "border-b border-violet-100"
        )}
      >
        <div className="min-w-0 flex-1">
          <div className="text-[11px] font-medium uppercase tracking-wide text-violet-600/80">
            Subagent
          </div>
          <div className="truncate text-sm font-medium text-neutral-900">
            {title}
          </div>
          {childId ? (
            <div className="mt-0.5 truncate font-mono text-[11px] text-neutral-400">
              {childId}
            </div>
          ) : null}
        </div>
        <span className="shrink-0 text-[11px] text-neutral-500">{status}</span>
        {open ? (
          <ChevronUp className="size-4 shrink-0 text-neutral-400" />
        ) : (
          <ChevronDown className="size-4 shrink-0 text-neutral-400" />
        )}
      </button>
      {open ? (
        <div className="max-h-[28rem] overflow-y-auto bg-neutral-50/60 px-3 py-3 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
          {loading && (
            <p className="px-1 text-xs text-neutral-500">Loading subagent…</p>
          )}
          {error && <p className="px-1 text-xs text-red-600">{error}</p>}
          {!loading && !error && nestedMeta && nestedSpans && (
            <NestedSubagentFeed meta={nestedMeta} spans={nestedSpans} />
          )}
          {!loading && !error && !childId && (
            <p className="px-1 text-xs text-neutral-500">
              No linked subagent session id yet.
            </p>
          )}
        </div>
      ) : null}
    </div>
  );
}

function NestedSubagentFeed({
  meta,
  spans,
}: {
  meta: SessionMeta;
  spans: Span[];
}) {
  const items = useMemo(() => buildTimeline(spans), [spans]);
  const vendor = useMemo(() => vendorFromMeta(meta, spans), [meta, spans]);
  const feed = items.filter(
    (it) =>
      it.kind === "prompt" ||
      it.kind === "response" ||
      it.kind === "tools"
  );

  if (!feed.length) {
    return (
      <p className="px-1 text-xs text-neutral-500">
        No transcript events in this subagent yet.
      </p>
    );
  }

  return (
    <div className="space-y-3">
      {feed.map((it) => {
        if (it.kind === "prompt") {
          return (
            <div
              key={it.id}
              className="rounded-xl bg-white px-3 py-2.5 text-[13px] leading-relaxed text-neutral-800 shadow-sm ring-1 ring-neutral-200/80"
            >
              <div className="mb-1 text-[10px] font-medium uppercase tracking-wide text-neutral-400">
                Prompt
              </div>
              <p className="whitespace-pre-wrap">{it.text}</p>
            </div>
          );
        }
        if (it.kind === "response") {
          return (
            <div
              key={it.id}
              className={cn(
                "rounded-xl px-3 py-2.5 text-[13px] leading-relaxed text-neutral-800 shadow-sm ring-1",
                it.asThought
                  ? "bg-amber-50/80 ring-amber-100 dark:bg-amber-950/45 dark:ring-amber-500/25"
                  : "bg-white ring-neutral-200/80"
              )}
            >
              <div
                className={cn(
                  "mb-1 text-[10px] font-medium uppercase tracking-wide",
                  it.asThought
                    ? "text-amber-800/80 dark:text-amber-300/90"
                    : "text-neutral-400"
                )}
              >
                {it.asThought
                  ? it.thoughtUnavailable
                    ? "Reasoning"
                    : "Thought"
                  : vendor}
              </div>
              <p className="whitespace-pre-wrap">{it.text}</p>
            </div>
          );
        }
        if (it.kind === "tools") {
          return (
            <div
              key={it.id}
              className="overflow-hidden rounded-xl bg-white shadow-sm ring-1 ring-neutral-200/80"
            >
              <div className="border-b border-neutral-100 px-3 py-2 text-[12px] font-medium text-neutral-700">
                {toolGroupLabel(it.tools)}
              </div>
              <ul className="divide-y divide-neutral-100">
                {it.tools.map((t, idx) => (
                  <li
                    key={idx}
                    className="flex items-baseline gap-2 px-3 py-2 text-[12px]"
                  >
                    <span className="min-w-0 flex-1 truncate text-neutral-800">
                      {t.summary || t.name}
                    </span>
                    <span className="shrink-0 text-neutral-400">{t.name}</span>
                  </li>
                ))}
              </ul>
            </div>
          );
        }
        return null;
      })}
    </div>
  );
}

function FilterRow({
  label,
  count,
  checked,
  onChange,
}: {
  label: string;
  count: number;
  checked: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <label className="tb-filter-row">
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
      />
      <span>{label}</span>
      <span className="tb-filter-count">{count}</span>
    </label>
  );
}
