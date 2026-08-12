import { NextRequest, NextResponse } from "next/server";
import { existsSync, readFileSync, writeFileSync } from "fs";
import { loadConfig } from "@/lib/so/misc";
import { soPath } from "@/lib/so/root";
import { projectIdFromRequest, runWithProject, runWithProjectAsync } from "@/lib/so/workspace";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(req: NextRequest) {
  const project = projectIdFromRequest(req);
  return runWithProject(project, () => NextResponse.json(loadConfig()));
}

type PatchBody = {
  memory?: {
    enabled?: boolean;
    idle_harvest_hours?: number;
    backend?: string;
  };
  guardrails?: { enabled?: boolean };
  recommendations?: { auto?: boolean; require_approval?: boolean };
  retention?: { days?: number };
  evals?: {
    on_session_end?: boolean;
    auto?: boolean;
    backend?: string;
    model_claude?: string;
    model_codex?: string;
  };
  graph?: { code?: boolean; semantic?: boolean };
  llm?: {
    provider?: string;
    model?: string;
    api_key_env?: string;
    base_url?: string;
  };
};

function formatYamlValue(value: string | number | boolean): string {
  if (typeof value === "boolean" || typeof value === "number") return String(value);
  const s = String(value);
  if (s === "") return '""';
  if (/^[\w./:@+-]+$/.test(s)) return s;
  return JSON.stringify(s);
}

function isTopLevelKey(line: string): boolean {
  return /^[a-zA-Z_][\w-]*:\s*(?:#.*)?$/.test(line) || /^[a-zA-Z_][\w-]*:\s+\S/.test(line);
}

/** Set a nested key under a top-level YAML section (2-3 path segments). */
function setYamlPath(
  raw: string,
  path: string[],
  value: string | number | boolean
): string {
  if (path.length < 2 || path.length > 3) return raw;
  const [section, midOrKey, maybeKey] = path;
  const leafKey = path.length === 3 ? maybeKey! : midOrKey;
  const midKey = path.length === 3 ? midOrKey : null;
  const formatted = formatYamlValue(value);

  let lines = raw.replace(/\r\n/g, "\n").split("\n");
  let sectionIdx = lines.findIndex((l) => new RegExp(`^${section}:\\s*(?:#.*)?$`).test(l));
  if (sectionIdx < 0) {
    const block =
      midKey != null
        ? [`${section}:`, `  ${midKey}:`, `    ${leafKey}: ${formatted}`]
        : [`${section}:`, `  ${leafKey}: ${formatted}`];
    const trimmed = raw.trimEnd();
    return `${trimmed}${trimmed ? "\n" : ""}${block.join("\n")}\n`;
  }

  // Find section end (next top-level key).
  let sectionEnd = lines.length;
  for (let i = sectionIdx + 1; i < lines.length; i++) {
    if (lines[i].trim() && !/^\s/.test(lines[i]) && isTopLevelKey(lines[i])) {
      sectionEnd = i;
      break;
    }
  }

  const indentOf = (line: string) => (line.match(/^(\s*)/)?.[1].length ?? 0);
  let baseIndent = 2;
  for (let i = sectionIdx + 1; i < sectionEnd; i++) {
    if (!lines[i].trim() || lines[i].trim().startsWith("#")) continue;
    baseIndent = indentOf(lines[i]) || 2;
    break;
  }
  const ind1 = " ".repeat(baseIndent);
  const ind2 = " ".repeat(baseIndent * 2);

  if (midKey == null) {
    for (let i = sectionIdx + 1; i < sectionEnd; i++) {
      if (new RegExp(`^\\s+${leafKey}:\\s*`).test(lines[i]) && indentOf(lines[i]) === baseIndent) {
        lines[i] = `${ind1}${leafKey}: ${formatted}`;
        return lines.join("\n");
      }
    }
    lines.splice(sectionIdx + 1, 0, `${ind1}${leafKey}: ${formatted}`);
    return lines.join("\n");
  }

  // Nested: section → mid → leaf
  let midIdx = -1;
  for (let i = sectionIdx + 1; i < sectionEnd; i++) {
    if (
      new RegExp(`^\\s+${midKey}:\\s*(?:#.*)?$`).test(lines[i]) &&
      indentOf(lines[i]) === baseIndent
    ) {
      midIdx = i;
      break;
    }
  }
  if (midIdx < 0) {
    lines.splice(
      sectionIdx + 1,
      0,
      `${ind1}${midKey}:`,
      `${ind2}${leafKey}: ${formatted}`
    );
    return lines.join("\n");
  }

  let midEnd = sectionEnd;
  for (let i = midIdx + 1; i < sectionEnd; i++) {
    if (lines[i].trim() && indentOf(lines[i]) <= baseIndent) {
      midEnd = i;
      break;
    }
  }
  for (let i = midIdx + 1; i < midEnd; i++) {
    if (new RegExp(`^\\s+${leafKey}:\\s*`).test(lines[i])) {
      lines[i] = `${ind2}${leafKey}: ${formatted}`;
      return lines.join("\n");
    }
  }
  lines.splice(midIdx + 1, 0, `${ind2}${leafKey}: ${formatted}`);
  return lines.join("\n");
}

const DEFAULT_YAML = `# Superopen project configuration. This is the authoritative source for enabled vendors, review behavior, graph refresh, retention, and feature settings.
# Updated by project maintainers and Superopen configuration commands.
layout_version: 2
vendors:
  enabled: []
  shared_agents: false
graph:
  code: true
  semantic: true
  semantic_backend: auto
  refresh_policy: after_changed_session
memory:
  enabled: true
  idle_harvest_hours: 6
  backend: auto
guardrails:
  enabled: true
recommendations:
  auto: true
  require_approval: true
evals:
  on_session_end: true
  auto: true
  backend: auto
retention:
  days: 7
`;

export async function PUT(req: NextRequest) {
  const project = projectIdFromRequest(req);
  const body = (await req.json().catch(() => ({}))) as PatchBody;

  return runWithProjectAsync(project, async () => {
    const p = soPath("config.yaml");
    let raw = existsSync(p) ? readFileSync(p, "utf8") : DEFAULT_YAML;

    if (body.memory?.enabled !== undefined) {
      raw = setYamlPath(raw, ["memory", "enabled"], body.memory.enabled);
    }
    if (body.memory?.idle_harvest_hours !== undefined) {
      raw = setYamlPath(raw, ["memory", "idle_harvest_hours"], body.memory.idle_harvest_hours);
    }
    if (body.memory?.backend !== undefined) {
      raw = setYamlPath(raw, ["memory", "backend"], body.memory.backend);
    }
    if (body.guardrails?.enabled !== undefined) {
      raw = setYamlPath(raw, ["guardrails", "enabled"], body.guardrails.enabled);
    }
    if (body.recommendations?.auto !== undefined) {
      raw = setYamlPath(raw, ["recommendations", "auto"], body.recommendations.auto);
    }
    if (body.recommendations?.require_approval !== undefined) {
      raw = setYamlPath(
        raw,
        ["recommendations", "require_approval"],
        body.recommendations.require_approval
      );
    }
    if (body.evals?.on_session_end !== undefined) {
      raw = setYamlPath(raw, ["evals", "on_session_end"], body.evals.on_session_end);
    }
    if (body.evals?.auto !== undefined) {
      raw = setYamlPath(raw, ["evals", "auto"], body.evals.auto);
    }
    if (body.evals?.backend !== undefined) {
      raw = setYamlPath(raw, ["evals", "backend"], body.evals.backend);
    }
    if (body.evals?.model_claude !== undefined) {
      raw = setYamlPath(raw, ["evals", "models", "claude"], body.evals.model_claude);
    }
    if (body.evals?.model_codex !== undefined) {
      raw = setYamlPath(raw, ["evals", "models", "codex"], body.evals.model_codex);
    }
    if (body.retention?.days !== undefined) {
      raw = setYamlPath(raw, ["retention", "days"], body.retention.days);
    }
    if (body.graph?.code !== undefined) {
      raw = setYamlPath(raw, ["graph", "code"], body.graph.code);
    }
    if (body.graph?.semantic !== undefined) {
      raw = setYamlPath(raw, ["graph", "semantic"], body.graph.semantic);
    }
    if (body.llm?.provider !== undefined) {
      raw = setYamlPath(raw, ["llm", "provider"], body.llm.provider);
    }
    if (body.llm?.model !== undefined) {
      raw = setYamlPath(raw, ["llm", "model"], body.llm.model);
    }
    if (body.llm?.api_key_env !== undefined) {
      raw = setYamlPath(raw, ["llm", "api_key_env"], body.llm.api_key_env);
    }
    if (body.llm?.base_url !== undefined) {
      raw = setYamlPath(raw, ["llm", "base_url"], body.llm.base_url);
    }

    writeFileSync(p, raw.endsWith("\n") ? raw : raw + "\n", "utf8");
    return NextResponse.json({ ok: true, config: loadConfig() });
  });
}
