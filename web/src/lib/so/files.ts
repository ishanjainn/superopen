import { fileExists, readText, writeText, absPath } from "./nodeio";
import { existsSync, readdirSync, statSync, unlinkSync } from "fs";
import { basename, join, relative, sep } from "path";
import { repoRoot, soRoot } from "./root";

/** Editable roots: native guidance + Superopen-owned guardrails/evals. */
const EDITABLE_ROOTS = [
  "knowledge", // AGENTS.md (+ nested)
  "rules", // all vendor rules trees (writes prefer discovered target)
  "skills", // all vendor skills trees
  "guardrails",
  "evals",
] as const;

const VENDOR_RULES: { kind: string; rel: string }[] = [
  { kind: "cursor", rel: ".cursor/rules" },
  { kind: "claude", rel: ".claude/rules" },
  { kind: "agents", rel: ".agents/rules" },
  { kind: "gemini", rel: ".gemini/rules" },
  { kind: "codex", rel: ".codex/rules" },
  { kind: "opencode", rel: ".opencode/rules" },
  { kind: "copilot", rel: ".github/instructions" },
  { kind: "pi", rel: ".pi/rules" },
];

const VENDOR_SKILLS: { kind: string; rel: string }[] = [
  { kind: "claude", rel: ".claude/skills" },
  { kind: "cursor", rel: ".cursor/skills" },
  { kind: "agents", rel: ".agents/skills" },
  { kind: "gemini", rel: ".gemini/skills" },
  { kind: "opencode", rel: ".opencode/skills" },
  { kind: "codex", rel: ".codex/skills" },
  { kind: "copilot", rel: ".github/skills" },
  { kind: "pi", rel: ".pi/skills" },
];

function isReservedRule(name: string): boolean {
  return name === "superopen.mdc" || name === "superopen.md";
}

function isRuleFile(name: string): boolean {
  const lower = name.toLowerCase();
  return (
    lower.endsWith(".mdc") ||
    lower.endsWith(".md") ||
    lower.endsWith(".instructions.md")
  );
}

function hasUserRules(dir: string): boolean {
  if (!existsSync(dir)) return false;
  try {
    const walk = (d: string): boolean => {
      for (const name of readdirSync(d)) {
        if (isReservedRule(name)) continue;
        const full = join(d, name);
        let st;
        try {
          st = statSync(full);
        } catch {
          continue;
        }
        if (st.isDirectory()) {
          if (walk(full)) return true;
          continue;
        }
        if (isRuleFile(name)) return true;
      }
      return false;
    };
    return walk(dir);
  } catch {
    return false;
  }
}

function hasUserSkills(dir: string): boolean {
  if (!existsSync(dir)) return false;
  try {
    for (const name of readdirSync(dir)) {
      if (name === "so" || name === "superopen") continue;
      if (fileExists(join(dir, name, "SKILL.md"))) return true;
    }
  } catch {
    return false;
  }
  return false;
}

function configuredVendorKinds(): string[] {
	const path = join(soRoot(), "config.yaml");
	if (!fileExists(path)) return [];
	const raw = readText(path);
	const vendors = raw.match(/^vendors:\s*\n([\s\S]*?)(?=^[a-zA-Z_][\w-]*:|\s*$)/m)?.[1] || "";
	const enabled = vendors.match(/^\s{2}enabled:\s*(?:\[([^\]]*)\])?\s*\n?([\s\S]*?)(?=^\s{2}[a-zA-Z_][\w-]*:|\s*$)/m);
	const values = [
		...(enabled?.[1] || "").split(","),
		...Array.from((enabled?.[2] || "").matchAll(/^\s*-\s*([^#\n]+)/gm), (m) => m[1]),
	]
		.map((v) => v.trim().replace(/^['"]|['"]$/g, ""))
		.map((v) => v === "claude-code" ? "claude" : v === "copilot-cli" ? "copilot" : v)
		.filter(Boolean);
	return Array.from(new Set(values));
}

/** Preferred write target for new rules (mirrors Go discoverNativeRoots). */
export function preferredRulesDirAbs(): string {
  const root = repoRoot();
	const existing = VENDOR_RULES.filter((v) => {
    const dir = join(root, ...v.rel.split("/"));
		return hasUserRules(dir);
	});
	if (existing.length === 1) return absPath(join(root, ...existing[0].rel.split("/")));
	const enabled = configuredVendorKinds().filter((kind) => VENDOR_RULES.some((v) => v.kind === kind));
	if (enabled.length === 1) {
		const match = VENDOR_RULES.find((v) => v.kind === enabled[0])!;
		return absPath(join(root, ...match.rel.split("/")));
	}
	throw new Error("choose an explicit vendor rules tree; shared .agents is opt-in");
}

export function preferredSkillsDirAbs(): string {
  const root = repoRoot();
	const existing = VENDOR_SKILLS.filter((v) => {
    const dir = join(root, ...v.rel.split("/"));
		return hasUserSkills(dir);
	});
	if (existing.length === 1) return absPath(join(root, ...existing[0].rel.split("/")));
	const enabled = configuredVendorKinds().filter((kind) => VENDOR_SKILLS.some((v) => v.kind === kind));
	if (enabled.length === 1) {
		const match = VENDOR_SKILLS.find((v) => v.kind === enabled[0])!;
		return absPath(join(root, ...match.rel.split("/")));
	}
	throw new Error("choose an explicit vendor skills tree; shared .agents is opt-in");
}

function preferredRulesKind(): string {
  const preferred = preferredRulesDirAbs();
  const root = absPath(repoRoot());
  for (const v of VENDOR_RULES) {
    if (absPath(join(root, ...v.rel.split("/"))) === preferred) return v.kind;
  }
  return "agents";
}

function preferredSkillsKind(): string {
  const preferred = preferredSkillsDirAbs();
  const root = absPath(repoRoot());
  for (const v of VENDOR_SKILLS) {
    if (absPath(join(root, ...v.rel.split("/"))) === preferred) return v.kind;
  }
  return "agents";
}

function ruleExtForKind(kind: string): string {
  if (kind === "cursor") return ".mdc";
  if (kind === "copilot") return ".instructions.md";
  return ".md";
}

function collectRuleEntries(): { name: string; path: string; isDir: boolean }[] {
  const root = repoRoot();
  const entries: { name: string; path: string; isDir: boolean }[] = [];
  const seen = new Set<string>();
  for (const v of VENDOR_RULES) {
    const base = join(root, ...v.rel.split("/"));
    if (!existsSync(base)) continue;
    const walk = (dir: string, relWithin: string) => {
      let names: string[] = [];
      try {
        names = readdirSync(dir);
      } catch {
        return;
      }
      for (const name of names) {
        if (name.startsWith(".") || isReservedRule(name)) continue;
        const full = join(dir, name);
        let st;
        try {
          st = statSync(full);
        } catch {
          continue;
        }
        const childRel = relWithin ? `${relWithin}/${name}` : name;
        if (st.isDirectory()) {
          walk(full, childRel);
          continue;
        }
        if (!isRuleFile(name)) continue;
        const repoRelativePath = `${v.rel}/${childRel}`;
        if (seen.has(repoRelativePath)) continue;
        seen.add(repoRelativePath);
        entries.push({
          name: `${v.kind}/${childRel}`,
          path: repoRelativePath,
          isDir: false,
        });
      }
    };
    walk(base, "");
  }
  entries.sort((a, b) => a.name.localeCompare(b.name));
  return entries;
}

function collectSkillEntries(): { name: string; path: string; isDir: boolean }[] {
  const root = repoRoot();
  const entries: { name: string; path: string; isDir: boolean }[] = [];
  const seen = new Set<string>();
  for (const v of VENDOR_SKILLS) {
    const base = join(root, ...v.rel.split("/"));
    if (!existsSync(base)) continue;
    let names: string[] = [];
    try {
      names = readdirSync(base);
    } catch {
      continue;
    }
    for (const name of names) {
      if (name === "so" || name === "superopen" || name.startsWith(".")) continue;
      const skillMd = join(base, name, "SKILL.md");
      if (!fileExists(skillMd)) continue;
      const repoRelativePath = `${v.rel}/${name}/SKILL.md`;
      if (seen.has(repoRelativePath)) continue;
      seen.add(repoRelativePath);
      entries.push({
        name: `${v.kind}/${name}`,
        path: repoRelativePath,
        isDir: false,
      });
    }
  }
  entries.sort((a, b) => a.name.localeCompare(b.name));
  return entries;
}

function collectKnowledgeEntries(): {
  name: string;
  path: string;
  isDir: boolean;
}[] {
  const root = absPath(repoRoot());
  const entries: { name: string; path: string; isDir: boolean }[] = [];
  const rootAgents = join(root, "AGENTS.md");
  if (fileExists(rootAgents)) {
    entries.push({ name: "AGENTS.md", path: "knowledge/AGENTS.md", isDir: false });
  }
  const skip = new Set([
    ".git",
    "node_modules",
    "vendor",
    "dist",
    ".so",
    ".next",
    "coverage",
  ]);
  const walk = (dir: string) => {
    let names: string[] = [];
    try {
      names = readdirSync(dir);
    } catch {
      return;
    }
    for (const name of names) {
      if (skip.has(name)) continue;
      const full = join(dir, name);
      let st;
      try {
        st = statSync(full);
      } catch {
        continue;
      }
      if (st.isDirectory()) {
        walk(full);
        continue;
      }
      if (name !== "AGENTS.md") continue;
      if (absPath(full) === absPath(rootAgents)) continue;
      const rel = relative(root, full).split(sep).join("/");
      entries.push({
        name: rel,
        path: `knowledge/${rel}`,
        isDir: false,
      });
    }
  };
  walk(root);
  return entries;
}

function resolveRulesAbs(rest: string): string | null {
  const root = repoRoot();
  if (!rest) return preferredRulesDirAbs();
  const parts = rest.split("/");
  const kind = parts[0];
  const vendor = VENDOR_RULES.find((v) => v.kind === kind);
  if (vendor && parts.length >= 2) {
    return absPath(join(root, ...vendor.rel.split("/"), ...parts.slice(1)));
  }
  // backward-compat: rules/coding.md → preferred write dir
  return absPath(join(preferredRulesDirAbs(), rest));
}

function resolveSkillsAbs(rest: string): string | null {
  const root = repoRoot();
  if (!rest) return preferredSkillsDirAbs();
  const parts = rest.split("/");
  const kind = parts[0];
  const vendor = VENDOR_SKILLS.find((v) => v.kind === kind);
  if (vendor) {
    if (parts.length === 2) {
      // skills/cursor/pr-hygiene → …/SKILL.md
      return absPath(join(root, ...vendor.rel.split("/"), parts[1], "SKILL.md"));
    }
    if (parts.length >= 3) {
      return absPath(join(root, ...vendor.rel.split("/"), ...parts.slice(1)));
    }
  }
  // skills/pr-hygiene or skills/pr-hygiene/SKILL.md → preferred
  if (parts.length === 1) {
    return absPath(join(preferredSkillsDirAbs(), parts[0], "SKILL.md"));
  }
  return absPath(join(preferredSkillsDirAbs(), ...parts));
}

function nativeAbs(rel: string): string | null {
  const cleaned = rel.replace(/^\/+/, "").replace(/\0/g, "");
  if (!cleaned || cleaned.includes("..")) return null;

  // Rules use their physical vendor path in the UI too. Nested rule folders are
  // supported, but the target must still be a rule file under a known root.
  for (const vendor of VENDOR_RULES) {
    const prefix = `${vendor.rel}/`;
    if (!cleaned.startsWith(prefix)) continue;
    const within = cleaned.slice(prefix.length);
    if (!within || !isRuleFile(basename(within))) return null;
    return absPath(join(repoRoot(), ...cleaned.split("/")));
  }

  // The UI exposes native skill files by their real repository-relative path
  // (for example .codex/skills/review/SKILL.md). Only the standard one-folder
  // Agent Skills shape is editable; loose files directly in a skills root are
  // intentionally not treated as skills.
  for (const vendor of VENDOR_SKILLS) {
    const prefix = `${vendor.rel}/`;
    if (!cleaned.startsWith(prefix)) continue;
    const within = cleaned.slice(prefix.length).split("/");
    if (within.length !== 2 || within[1] !== "SKILL.md" || !within[0]) {
      return null;
    }
    return absPath(join(repoRoot(), ...cleaned.split("/")));
  }

  const parts = cleaned.split("/");
  const top = parts[0];
  const rest = parts.slice(1).join("/");

  if (top === "knowledge") {
    if (!rest || rest === "AGENTS.md") {
      return absPath(join(repoRoot(), "AGENTS.md"));
    }
    if (rest.endsWith("/AGENTS.md") || rest === "AGENTS.md") {
      return absPath(join(repoRoot(), rest));
    }
    return absPath(join(repoRoot(), rest, "AGENTS.md"));
  }
  if (top === "rules") {
    return resolveRulesAbs(rest);
  }
  if (top === "skills") {
    return resolveSkillsAbs(rest);
  }
  if (top === "guardrails" || top === "evals") {
		if (top === "guardrails" && (rest === "guardrails.yaml" || rest === "")) {
			return absPath(join(soRoot(), "guardrails.yaml"));
		}
		if (top === "evals" && (rest === "configs.yaml" || rest === "")) {
			return absPath(join(soRoot(), "evals.yaml"));
		}
		return null;
  }
  return null;
}

export function listOrRead(relPath: string): {
  type: "dir" | "file";
  entries?: { name: string; path: string; isDir: boolean }[];
  body?: string;
} | null {
  const cleaned = (relPath || ".").replace(/^\/+/, "");
  if (cleaned === "." || cleaned === "") {
    return {
      type: "dir",
      entries: EDITABLE_ROOTS.map((name) => ({
        name,
        path: name,
        isDir: true,
      })),
    };
  }
  if (cleaned === "knowledge") {
    return { type: "dir", entries: collectKnowledgeEntries() };
  }
  if (cleaned === "rules") {
    return { type: "dir", entries: collectRuleEntries() };
  }
  if (cleaned === "skills") {
    return { type: "dir", entries: collectSkillEntries() };
  }
	if (cleaned === "guardrails") {
		return { type: "dir", entries: [{ name: "guardrails.yaml", path: "guardrails/guardrails.yaml", isDir: false }] };
	}
	if (cleaned === "evals") {
		return { type: "dir", entries: [{ name: "evals.yaml", path: "evals/configs.yaml", isDir: false }] };
	}

  const resolved = nativeAbs(cleaned);
  if (!resolved || !fileExists(resolved)) return null;
  const st = statSync(resolved);
  if (st.isDirectory()) {
    const top = cleaned.split("/")[0];
    if (top === "guardrails" || top === "evals") {
      const entries = readdirSync(resolved)
        .filter(
          (n) =>
            !n.startsWith(".") && n !== "history.json" && n !== "so" && n !== "superopen"
        )
        .map((name) => {
          const child = join(resolved, name);
          let isDir = false;
          try {
            isDir = statSync(child).isDirectory();
          } catch {
            isDir = false;
          }
          return {
            name,
            path: `${top}/${name}`,
            isDir,
          };
        });
      return { type: "dir", entries };
    }
    return null;
  }
  return { type: "file", body: readText(resolved) };
}

function assertEditable(relPath: string): { abs: string; rel: string } {
  const cleaned = relPath.replace(/^\/+/, "").replace(/\0/g, "");
  if (!cleaned || cleaned.includes("..")) {
    throw new Error("invalid path");
  }
  const top = cleaned.split("/")[0];
  const nativeRule = VENDOR_RULES.some((vendor) =>
    cleaned.startsWith(`${vendor.rel}/`)
  );
  const nativeSkill = VENDOR_SKILLS.some((vendor) =>
    cleaned.startsWith(`${vendor.rel}/`)
  );
  if (
    !(EDITABLE_ROOTS as readonly string[]).includes(top) &&
    !nativeRule &&
    !nativeSkill
  ) {
    throw new Error(
      `only ${EDITABLE_ROOTS.join(", ")} are editable (shared harness files)`
    );
  }
  const abs = nativeAbs(cleaned);
  if (!abs) throw new Error("invalid path");
  return { abs, rel: cleaned.split(sep).join("/") };
}

const NAME_RE = /^[a-zA-Z0-9][a-zA-Z0-9._-]{0,120}$/;

function sanitizeFileName(name: string, fallbackExt: string): string {
  let n = name.trim().replace(/\\/g, "/").split("/").pop() || "";
  n = n.replace(/\0/g, "");
  if (!n) throw new Error("filename required");
  if (!NAME_RE.test(n)) {
    throw new Error(
      "filename must be alphanumeric (plus . _ -), max 121 chars"
    );
  }
  if (!n.includes(".")) {
    n = `${n}${fallbackExt.startsWith(".") ? fallbackExt : `.${fallbackExt}`}`;
  }
  const ext = n.split(".").pop()?.toLowerCase() || "";
  if (!["md", "mdc", "yaml", "yml", "json"].includes(ext) && !n.toLowerCase().endsWith(".instructions.md")) {
    throw new Error("allowed extensions: .md, .mdc, .yaml, .yml, .json");
  }
  return n;
}

function defaultContent(relPath: string): string {
  const name = basename(relPath);
  const stem = name.replace(/\.(mdc|md|ya?ml|json|instructions\.md)$/i, "");
  const title = stem
    .replace(/[-_]/g, " ")
    .replace(/\b\w/g, (c) => c.toUpperCase());
  const lower = name.toLowerCase();
  if (lower.endsWith(".md") || lower.endsWith(".mdc") || lower.endsWith(".instructions.md")) {
    return `# ${title}\n\n`;
  }
  if (lower.endsWith(".json")) {
    return "{\n}\n";
  }
  if (relPath.startsWith("evals/")) {
    return `# ${title}\nchecks:\n  - tests\n\nagent_rules:\n  - "Describe what this eval should enforce."\n`;
  }
  if (relPath.startsWith("guardrails/")) {
    return `# ${title}\nrules:\n  - id: ${stem.replace(/[^a-z0-9-]/gi, "-").toLowerCase() || "new-rule"}\n    description: Describe the guardrail.\n    severity: warn\n    source: ui\n`;
  }
  return `# ${title}\n\n`;
}

export function writeHarnessFile(
  relPath: string,
  body: string,
  opts?: { create?: boolean }
): { path: string; created: boolean } {
  const { abs, rel } = assertEditable(relPath);
  const created = !fileExists(abs);
  if (created && !opts?.create) {
    throw new Error("file not found");
  }
	if (created && rel === "guardrails/guardrails.yaml" && !body.trimStart().startsWith("#")) {
		body = `# Superopen guardrails. These are shared project safety rules enforced at coding-agent hook boundaries.\n# Authoritative project policy updated by project maintainers; so sync will not overwrite this file.\n${body}`;
	}
	if (created && rel === "evals/configs.yaml" && !body.trimStart().startsWith("#")) {
		body = `# Superopen evaluation policy. This defines how completed coding sessions are scored and which reviewer backends may be used.\n# Authoritative project policy updated by project maintainers and so init from graph plus agent instructions.\n${body}`;
	}
  writeText(abs, body);
  return { path: rel, created };
}

export function createHarnessFile(
  dir: string,
  fileName: string,
  bodyOverride?: string
): { path: string; body: string } {
  const top = dir.replace(/^\/+/, "").split("/")[0];
  if (!(EDITABLE_ROOTS as readonly string[]).includes(top)) {
    throw new Error(`cannot create under ${dir}`);
  }
  if (top === "knowledge") {
    throw new Error(
      "knowledge is AGENTS.md — edit that file instead of creating siblings"
    );
  }
	if (top === "guardrails" || top === "evals") {
		throw new Error(`${top} has one authoritative file; edit it instead of creating siblings`);
	}
  if (top === "rules") {
		const requested = dir.replace(/^\/+/, "").split("/")[1];
		const kind = VENDOR_RULES.some((v) => v.kind === requested) ? requested : preferredRulesKind();
    const ext = ruleExtForKind(kind);
    const name = sanitizeFileName(fileName, ext);
    const logical = `rules/${kind}/${name}`;
    const abs = nativeAbs(logical);
    if (abs && fileExists(abs)) throw new Error("file already exists");
    const vendor = VENDOR_RULES.find((candidate) => candidate.kind === kind)!;
    const rel = `${vendor.rel}/${name}`;
    const body =
      typeof bodyOverride === "string" ? bodyOverride : defaultContent(rel);
    writeHarnessFile(logical, body, { create: true });
    return { path: rel, body };
  }
  if (top === "skills") {
		const requested = dir.replace(/^\/+/, "").split("/")[1];
		const kind = VENDOR_SKILLS.some((v) => v.kind === requested) ? requested : preferredSkillsKind();
    let stem = fileName.trim().replace(/\\/g, "/").split("/").pop() || "";
    stem = stem.replace(/\.md$/i, "").replace(/\/SKILL$/i, "");
    if (!NAME_RE.test(stem)) {
      throw new Error(
        "skill name must be alphanumeric (plus . _ -), max 121 chars"
      );
    }
    const logical = `skills/${kind}/${stem}/SKILL.md`;
    const abs = nativeAbs(logical);
    if (abs && fileExists(abs)) throw new Error("file already exists");
    const vendor = VENDOR_SKILLS.find((candidate) => candidate.kind === kind)!;
    const rel = `${vendor.rel}/${stem}/SKILL.md`;
    const body =
      typeof bodyOverride === "string" ? bodyOverride : defaultContent(rel);
    writeHarnessFile(logical, body, { create: true });
    return { path: rel, body };
  }
  const fallbackExt = ".yaml";
  const name = sanitizeFileName(fileName, fallbackExt);
  const rel = `${top}/${name}`;
  const abs = nativeAbs(rel);
  if (abs && fileExists(abs)) {
    throw new Error("file already exists");
  }
  const body =
    typeof bodyOverride === "string" ? bodyOverride : defaultContent(rel);
  writeHarnessFile(rel, body, { create: true });
  return { path: rel, body };
}

export function deleteHarnessFile(relPath: string): void {
  const { abs, rel } = assertEditable(relPath);
  if (rel === "knowledge/AGENTS.md" || rel === "knowledge") {
    throw new Error("cannot delete AGENTS.md from UI");
  }
  if (!fileExists(abs) || statSync(abs).isDirectory()) {
    throw new Error("file not found");
  }
  unlinkSync(abs);
}
