import { readdirSync } from "fs";
import { fileExists, readText } from "./nodeio";
import { join } from "path";
import { soPath } from "./root";
import { humanizePromptPreview } from "./sessions";
import { buildTrace, resolveMapSession, type MapSessionMeta } from "./trace";
import { getSessionMap, sessionKey } from "./session_map";
import type { AgentGraph, AgentNode, Trace } from "@/map/types";

type SessionMetaFile = {
  id?: string;
  parent_id?: string;
  is_subagent?: boolean;
  title?: string;
  prompt_preview?: string;
  model?: string;
  started_at?: string;
  ended_at?: string;
  vendor?: string;
};

function readMeta(dir: string): SessionMetaFile | null {
  const p = join(dir, "session.json");
  if (!fileExists(p)) return null;
  try {
    return JSON.parse(readText(p)) as SessionMetaFile;
  } catch {
    return null;
  }
}

function eventCountFor(session: MapSessionMeta): number {
  try {
    const sessionMap = getSessionMap();
    return buildTrace(session, sessionMap).events.length;
  } catch {
    return 0;
  }
}

function agentLabel(meta: SessionMetaFile, fallbackId: string): string {
  const title =
    humanizePromptPreview(String(meta.title || meta.prompt_preview || "")) ||
    String(meta.title || meta.prompt_preview || "");
  if (title && title !== fallbackId) return title.slice(0, 72);
  return `Subagent ${fallbackId.slice(0, 8)}`;
}

function linkMethodFor(_harness: string): "parent_id" {
  return "parent_id";
}

/** Build AgentGraph from `.so/sessions` parent_id nesting (all depths). */
export function buildAgentGraph(rootKey: string): AgentGraph | null {
  const root = resolveMapSession(rootKey);
  if (!root) return null;

  const sessionsDir = soPath("sessions");
  const agents: AgentNode[] = [
    {
      id: root.id,
      depth: 0,
      kind: "main",
      label: "Main",
      status: "main",
      traceAvailability: "available",
      traceSessionKey: root.key,
      traceEventCount: eventCountFor(root),
      linkQuality: "exact",
      linkMethod: "root",
    },
  ];

  if (!fileExists(sessionsDir)) {
    return { version: 1, rootSessionKey: root.key, agents };
  }

  type Row = { id: string; meta: SessionMetaFile; dir: string };
  const byParent = new Map<string, Row[]>();
  for (const name of readdirSync(/* turbopackIgnore: true */sessionsDir)) {
    if (name === "index.json" || name.startsWith(".") || name === root.id) continue;
    const childDir = join(sessionsDir, name);
    const meta = readMeta(childDir);
    if (!meta) continue;
    const parent = String(meta.parent_id || "");
    if (!parent) continue;
    const childId = String(meta.id || name);
    const list = byParent.get(parent) || [];
    list.push({ id: childId, meta, dir: childDir });
    byParent.set(parent, list);
  }

  const harness = root.harness || String(root.id && "cursor") || "cursor";
  const queue: { parentId: string; depth: number }[] = [{ parentId: root.id, depth: 1 }];
  const seen = new Set<string>([root.id]);

  while (queue.length > 0) {
    const { parentId, depth } = queue.shift()!;
    for (const row of byParent.get(parentId) || []) {
      if (seen.has(row.id)) continue;
      seen.add(row.id);
      const childKey = sessionKey(harness, row.dir);
      const childSession: MapSessionMeta = {
        key: childKey,
        id: row.id,
        harness,
        title: agentLabel(row.meta, row.id),
        path: row.dir,
        cwd: root.cwd,
        model: row.meta.model ? String(row.meta.model) : undefined,
        startedAt: row.meta.started_at ? String(row.meta.started_at) : undefined,
        endedAt: row.meta.ended_at ? String(row.meta.ended_at) : undefined,
        eventCount: 0,
      };
      const count = eventCountFor(childSession);
      agents.push({
        id: row.id,
        parentId,
        depth,
        kind: "subagent",
        label: agentLabel(row.meta, row.id),
        instructionPreview:
          humanizePromptPreview(String(row.meta.prompt_preview || "")) || undefined,
        status: "launched",
        traceAvailability: count > 0 ? "available" : "missing",
        traceSessionKey: childKey,
        traceEventCount: count,
        linkQuality: "exact",
        linkMethod: linkMethodFor(harness),
      });
      queue.push({ parentId: row.id, depth: depth + 1 });
    }
  }

  return { version: 1, rootSessionKey: root.key, agents };
}

export function buildAgentTrace(rootKey: string, agentId: string): Trace | null {
  const graph = buildAgentGraph(rootKey);
  if (!graph) return null;
  const node = graph.agents.find((a) => a.id === agentId);
  if (!node?.traceSessionKey) return null;
  const session = resolveMapSession(node.traceSessionKey);
  if (!session) return null;
  return buildTrace(session, getSessionMap()) as unknown as Trace;
}
