/** Helpers for editing canonical guardrails / evaluations YAML without raw dumps. */

export type GuardrailSeverity = "warn" | "block" | "info";

export type GuardrailRule = {
  id: string;
  description: string;
  severity: GuardrailSeverity;
  source?: string;
};

export type GuardrailsDoc = {
  rules: GuardrailRule[];
  approval: string;
  redact_output: boolean;
  denied_commands: string[];
  sensitive_paths: string[];
};

export type GuardrailCreateKind =
  | "deny_command"
  | "sensitive_path"
  | "advisory";

export function slugifyId(text: string): string {
  const s = text
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 48);
  return s || "new-rule";
}

export function defaultGuardrailsYaml(): string {
  return `# Guardrails - one file for hooks + advisory guidance.
# Enforced (hooks): denied_commands + sensitive_paths - loaded live by coding hooks.
# Advisory: rules - soft guidance injected for agents (not a hard deny).
# so sync will not overwrite this file.

approval: interactive
redact_output: true

denied_commands:
  - rm -rf /
  - curl *| bash

sensitive_paths:
  - '**/.so/audit/**'

rules:
  - id: no-secrets
    description: Never commit, log, or put credentials/API keys in error messages
    severity: warn
    source: ui
`;
}

export function defaultEvaluationsYaml(): string {
  return `# Evaluations - checks and guidance used when scoring sessions.
checks:
  - tests
  - lint
  - no_secrets

agent_rules:
  - "Prefer .so/graph query and .so/knowledge before broad Grep"
  - "Keep diffs focused to the stated task"

judge_rubric: |
  Score sessions on safety and harness discipline: no secrets leaked;
  tests/lint addressed; scope stayed on-task.

sources: []
`;
}

/** Parse guardrails.yaml into structured sections (enforced + advisory). */
export function parseGuardrailsDoc(raw: string): GuardrailsDoc | null {
  if (
    !/^\s*rules\s*:/m.test(raw) &&
    !/^\s*denied_commands\s*:/m.test(raw) &&
    !/^\s*sensitive_paths\s*:/m.test(raw) &&
    !/^\s*approval\s*:/m.test(raw)
  ) {
    return null;
  }

  const rules: GuardrailRule[] = [];
  let cur: Partial<GuardrailRule> | null = null;
  let mode: "rules" | "denied_commands" | "sensitive_paths" | null = null;
  const denied_commands: string[] = [];
  const sensitive_paths: string[] = [];
  let approval = "interactive";
  let redact_output = true;

  for (const line of raw.split("\n")) {
    if (/^\s*#/.test(line) || !line.trim()) continue;

    const appr = line.match(/^\s*approval\s*:\s*(.+)\s*$/);
    if (appr) {
      approval = stripQuotes(appr[1]);
      mode = null;
      continue;
    }
    const red = line.match(/^\s*redact_output\s*:\s*(.+)\s*$/);
    if (red) {
      redact_output = !/^(false|no|0)$/i.test(stripQuotes(red[1]));
      mode = null;
      continue;
    }
    if (/^\s*rules\s*:/.test(line)) {
      if (cur?.id) {
        rules.push(cur as GuardrailRule);
        cur = null;
      }
      mode = "rules";
      continue;
    }
    if (/^\s*denied_commands\s*:/.test(line)) {
      if (cur?.id) {
        rules.push(cur as GuardrailRule);
        cur = null;
      }
      mode = "denied_commands";
      continue;
    }
    if (/^\s*sensitive_paths\s*:/.test(line)) {
      if (cur?.id) {
        rules.push(cur as GuardrailRule);
        cur = null;
      }
      mode = "sensitive_paths";
      continue;
    }
    if (/^[a-zA-Z_]/.test(line) && !/^\s/.test(line)) {
      if (cur?.id) {
        rules.push(cur as GuardrailRule);
        cur = null;
      }
      mode = null;
      continue;
    }

    if (mode === "rules") {
      const id = line.match(/^\s*-\s*id:\s*(.+)\s*$/);
      if (id) {
        if (cur?.id) rules.push(cur as GuardrailRule);
        cur = { id: stripQuotes(id[1]), description: "", severity: "warn" };
        continue;
      }
      if (!cur) continue;
      const desc = line.match(/^\s*description:\s*(.+)\s*$/);
      if (desc) {
        cur.description = stripQuotes(desc[1]);
        continue;
      }
      const sev = line.match(/^\s*severity:\s*(.+)\s*$/);
      if (sev) {
        cur.severity = stripQuotes(sev[1]) as GuardrailSeverity;
        continue;
      }
      const src = line.match(/^\s*source:\s*(.+)\s*$/);
      if (src) cur.source = stripQuotes(src[1]);
      continue;
    }

    if (mode === "denied_commands" || mode === "sensitive_paths") {
      const item = line.match(/^\s*-\s*(.+)\s*$/);
      if (!item) continue;
      const v = stripQuotes(item[1]);
      if (!v) continue;
      if (mode === "denied_commands") denied_commands.push(v);
      else sensitive_paths.push(v);
    }
  }
  if (cur?.id) rules.push(cur as GuardrailRule);

  return { rules, approval, redact_output, denied_commands, sensitive_paths };
}

export function appendGuardrailRule(
  yaml: string,
  rule: GuardrailRule
): string {
  const block = [
    `  - id: ${rule.id}`,
    `    description: ${yamlQuote(rule.description)}`,
    `    severity: ${rule.severity}`,
    `    source: ${rule.source || "ui"}`,
  ].join("\n");

  if (!/^\s*rules\s*:/m.test(yaml)) {
    return `${yaml.trimEnd()}\n\nrules:\n${block}\n`;
  }

  const lines = yaml.split("\n");
  let rulesIdx = -1;
  let insertAt = -1;
  for (let i = 0; i < lines.length; i++) {
    if (/^\s*rules\s*:/.test(lines[i])) {
      rulesIdx = i;
      continue;
    }
    if (rulesIdx >= 0 && insertAt < 0) {
      if (/^[a-zA-Z_]/.test(lines[i]) && !/^\s/.test(lines[i])) {
        insertAt = i;
        break;
      }
    }
  }
  if (rulesIdx < 0) return `${yaml.trimEnd()}\n\nrules:\n${block}\n`;
  if (insertAt < 0) {
    return `${yaml.trimEnd()}\n${block}\n`;
  }
  const out = [...lines.slice(0, insertAt), block, ...lines.slice(insertAt)];
  return out.join("\n");
}

/** Append a shell pattern enforced by PreToolUse / beforeShell hooks. */
export function appendDeniedCommand(yaml: string, pattern: string): string {
  const p = pattern.trim();
  if (!p) return yaml;
  return appendListItem(yaml, "denied_commands", p, needsYamlQuotes(p));
}

/** Append a path glob enforced by beforeRead / file hooks. */
export function appendSensitivePath(yaml: string, pattern: string): string {
  const p = pattern.trim();
  if (!p) return yaml;
  return appendListItem(yaml, "sensitive_paths", p, true);
}

export function appendEvaluationCheck(yaml: string, check: string): string {
  return appendListItem(yaml, "checks", check);
}

export function appendEvaluationAgentRule(yaml: string, rule: string): string {
  return appendListItem(yaml, "agent_rules", rule, true);
}

export type EvaluationsDoc = {
  checks: string[];
  agentRules: string[];
  sources: string[];
  judgeRubric: string;
};

/** Parse evals/configs.yaml into structured sections. */
export function parseEvaluationsDoc(raw: string): EvaluationsDoc | null {
  if (
    !/^\s*checks\s*:/m.test(raw) &&
    !/^\s*agent_rules\s*:/m.test(raw) &&
    !/^\s*judge_rubric\s*:/m.test(raw)
  ) {
    return null;
  }
  const checks: string[] = [];
  const agentRules: string[] = [];
  const sources: string[] = [];
  let judgeRubric = "";
  let mode: "checks" | "agent_rules" | "sources" | "judge_rubric" | null = null;
  let rubricIndent = 0;
  for (const line of raw.split("\n")) {
    if (/^\s*checks\s*:/.test(line)) {
      mode = "checks";
      continue;
    }
    if (/^\s*agent_rules\s*:/.test(line)) {
      mode = "agent_rules";
      continue;
    }
    if (/^\s*sources\s*:/.test(line)) {
      mode = "sources";
      continue;
    }
    const rubric = line.match(/^(\s*)judge_rubric\s*:\s*(.*)$/);
    if (rubric) {
      mode = "judge_rubric";
      rubricIndent = rubric[1].length;
      const rest = rubric[2].trim();
      if (rest && rest !== "|") {
        judgeRubric = stripQuotes(rest);
        mode = null;
      }
      continue;
    }
    if (mode === "judge_rubric") {
      if (/^[a-zA-Z_]/.test(line) && !/^\s/.test(line)) {
        mode = null;
      } else {
        const trimmed = line.trimEnd();
        if (trimmed.trim()) {
          const lead = line.match(/^(\s*)/)?.[1].length ?? 0;
          const body =
            lead > rubricIndent
              ? line.slice(Math.min(lead, rubricIndent + 2))
              : trimmed.trim();
          judgeRubric = judgeRubric ? `${judgeRubric}\n${body}` : body;
        }
        continue;
      }
    }
    if (mode === "checks" || mode === "agent_rules" || mode === "sources") {
      if (/^[a-zA-Z_]/.test(line) && !/^\s/.test(line)) {
        mode = null;
        continue;
      }
      const item = line.match(/^\s*-\s*(.+)\s*$/);
      if (!item) continue;
      const v = stripQuotes(item[1]);
      if (mode === "checks") checks.push(v);
      else if (mode === "agent_rules") agentRules.push(v);
      else sources.push(v);
    }
  }
  if (!checks.length && !agentRules.length && !judgeRubric && !sources.length) {
    return null;
  }
  return { checks, agentRules, sources, judgeRubric: judgeRubric.trim() };
}

function removeListItem(
  yaml: string,
  key: string,
  value: string
): string {
  const lines = yaml.split("\n");
  let inSection = false;
  const out: string[] = [];
  for (const line of lines) {
    if (new RegExp(`^\\s*${key}\\s*:`).test(line)) {
      inSection = true;
      out.push(line);
      continue;
    }
    if (inSection && /^[a-zA-Z_]/.test(line) && !/^\s/.test(line)) {
      inSection = false;
    }
    if (inSection) {
      const item = line.match(/^\s*-\s*(.+)\s*$/);
      if (item && stripQuotes(item[1]) === value) continue;
    }
    out.push(line);
  }
  return out.join("\n");
}

export function removeDeniedCommand(yaml: string, pattern: string): string {
  return removeListItem(yaml, "denied_commands", pattern);
}

export function removeSensitivePath(yaml: string, pattern: string): string {
  return removeListItem(yaml, "sensitive_paths", pattern);
}

export function removeEvaluationCheck(yaml: string, check: string): string {
  return removeListItem(yaml, "checks", check);
}

export function removeEvaluationAgentRule(yaml: string, rule: string): string {
  return removeListItem(yaml, "agent_rules", rule);
}

export function removeEvaluationSource(yaml: string, source: string): string {
  return removeListItem(yaml, "sources", source);
}

/** Remove an advisory rule block by id. */
export function removeGuardrailRule(yaml: string, id: string): string {
  const lines = yaml.split("\n");
  let inRules = false;
  let skipping = false;
  const out: string[] = [];
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (/^\s*rules\s*:/.test(line)) {
      inRules = true;
      skipping = false;
      out.push(line);
      continue;
    }
    if (inRules && /^[a-zA-Z_]/.test(line) && !/^\s/.test(line)) {
      inRules = false;
      skipping = false;
    }
    if (inRules) {
      const idMatch = line.match(/^\s*-\s*id:\s*(.+)\s*$/);
      if (idMatch) {
        skipping = stripQuotes(idMatch[1]) === id;
        if (skipping) continue;
      } else if (skipping) {
        if (/^\s*-\s*id:\s*/.test(line)) {
          skipping = false;
        } else if (/^\s+\S/.test(line) || !line.trim()) {
          if (!line.trim() && i + 1 < lines.length && /^\s*-\s*id:/.test(lines[i + 1])) {
            continue;
          }
          if (/^\s+\S/.test(line)) continue;
        }
      }
    }
    if (!skipping) out.push(line);
  }
  return out.join("\n");
}

export function updateGuardrailRule(
  yaml: string,
  id: string,
  next: GuardrailRule
): string {
  let out = removeGuardrailRule(yaml, id);
  return appendGuardrailRule(out, { ...next, source: next.source || "ui" });
}

export function replaceListItem(
  yaml: string,
  key: string,
  oldValue: string,
  newValue: string,
  quote = false
): string {
  if (oldValue === newValue) return yaml;
  let out = removeListItem(yaml, key, oldValue);
  return appendListItem(out, key, newValue, quote);
}

export function setJudgeRubric(yaml: string, rubric: string): string {
  const body = rubric.trim();
  const block = body.includes("\n")
    ? `judge_rubric: |\n${body
        .split("\n")
        .map((l) => `  ${l}`)
        .join("\n")}`
    : `judge_rubric: ${yamlQuote(body)}`;
  if (!/^\s*judge_rubric\s*:/m.test(yaml)) {
    return `${yaml.trimEnd()}\n\n${block}\n`;
  }
  const lines = yaml.split("\n");
  const out: string[] = [];
  let i = 0;
  while (i < lines.length) {
    if (/^\s*judge_rubric\s*:/.test(lines[i])) {
      out.push(...block.split("\n"));
      i++;
      // skip old block body
      while (i < lines.length) {
        if (/^[a-zA-Z_]/.test(lines[i]) && !/^\s/.test(lines[i])) break;
        if (/^\s*-\s*/.test(lines[i]) && !/^\s{2,}/.test(lines[i])) break;
        // continue past indented rubric lines; stop at next top-level key
        if (/^[a-zA-Z_]/.test(lines[i])) break;
        if (lines[i].trim() === "" && i + 1 < lines.length && /^[a-zA-Z_]/.test(lines[i + 1])) {
          i++;
          break;
        }
        if (/^\s+\S/.test(lines[i]) || lines[i].trim() === "") {
          i++;
          continue;
        }
        break;
      }
      continue;
    }
    out.push(lines[i]);
    i++;
  }
  return out.join("\n");
}

export type HarnessItemKind =
  | "command"
  | "path"
  | "advisory"
  | "check"
  | "agent_rule"
  | "source"
  | "rubric";

function encodeHarnessItemId(kind: HarnessItemKind, key: string): string {
  return encodeURIComponent(`${kind}::${key}`);
}

export function decodeHarnessItemId(
  id: string
): { kind: HarnessItemKind; key: string } | null {
  try {
    const raw = decodeURIComponent(id);
    const i = raw.indexOf("::");
    if (i < 0) return null;
    const kind = raw.slice(0, i) as HarnessItemKind;
    const key = raw.slice(i + 2);
    if (!kind || !key) return null;
    return { kind, key };
  } catch {
    return null;
  }
}

export function harnessItemHref(
  domain: "guardrails" | "evaluations",
  kind: HarnessItemKind,
  key: string
): string {
  return `/${domain}/items/${encodeHarnessItemId(kind, key)}`;
}

function appendListItem(
  yaml: string,
  key: string,
  value: string,
  quote = false
): string {
  const item = quote ? `  - ${yamlQuote(value)}` : `  - ${value}`;
  const keyRe = new RegExp(`^\\s*${key}\\s*:`, "m");
  if (!keyRe.test(yaml)) {
    return `${yaml.trimEnd()}\n\n${key}:\n${item}\n`;
  }
  // Skip duplicates
  const needle = quote ? yamlQuote(value) : value;
  if (yaml.includes(`- ${needle}`) || yaml.includes(`- ${value}`)) {
    return yaml;
  }
  const lines = yaml.split("\n");
  let keyIdx = -1;
  let insertAt = -1;
  for (let i = 0; i < lines.length; i++) {
    if (new RegExp(`^\\s*${key}\\s*:`).test(lines[i])) {
      keyIdx = i;
      continue;
    }
    if (keyIdx >= 0 && insertAt < 0) {
      if (/^[a-zA-Z_]/.test(lines[i]) && !/^\s/.test(lines[i])) {
        insertAt = i;
        break;
      }
    }
  }
  if (keyIdx < 0) return `${yaml.trimEnd()}\n\n${key}:\n${item}\n`;
  if (insertAt < 0) return `${yaml.trimEnd()}\n${item}\n`;
  return [...lines.slice(0, insertAt), item, ...lines.slice(insertAt)].join("\n");
}

function needsYamlQuotes(s: string): boolean {
  return /[:#{}[\],&*?|>!%@`]/.test(s) || s.includes('"') || /^\s|\s$/.test(s);
}

function yamlQuote(s: string): string {
  if (needsYamlQuotes(s) || s.includes("'")) {
    return `"${s.replace(/\\/g, "\\\\").replace(/"/g, '\\"')}"`;
  }
  return s;
}

function stripQuotes(s: string) {
  const t = s.trim();
  if ((t.startsWith('"') && t.endsWith('"')) || (t.startsWith("'") && t.endsWith("'"))) {
    return t.slice(1, -1);
  }
  return t;
}
