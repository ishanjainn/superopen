import { fileExists, readText, writeText, absPath } from "./nodeio";
import { readdirSync, statSync, unlinkSync } from "fs";
import { basename, join, relative, sep } from "path";
import { soRoot } from "./root";

/** Top-level .so dirs that are git-shared and editable from the UI. */
const EDITABLE_ROOTS = [
  "knowledge",
  "rules",
  "skills",
  "guardrails",
  "evals",
] as const;

function safeJoin(rel: string): string | null {
  const root = absPath(soRoot());
  const cleaned = rel.replace(/^\/+/, "").replace(/\0/g, "");
  if (cleaned.includes("..")) return null;
  const abs = absPath(join(root, cleaned));
  const relCheck = relative(root, abs);
  if (relCheck.startsWith("..") || relCheck.includes(`..${sep}`)) return null;
  return abs;
}

export function listOrRead(relPath: string): {
  type: "dir" | "file";
  entries?: { name: string; path: string; isDir: boolean }[];
  body?: string;
} | null {
  const abs = safeJoin(relPath || ".");
  if (!abs || !fileExists(abs)) return null;
  const st = statSync(abs);
  if (st.isDirectory()) {
    const entries = readdirSync(abs)
      .filter((n) => !n.startsWith(".") && n !== "history.json")
      .map((name) => {
        const child = join(abs, name);
        let isDir = false;
        try {
          isDir = statSync(child).isDirectory();
        } catch {
          isDir = false;
        }
        const path = relative(soRoot(), child).split(sep).join("/");
        return { name, path, isDir };
      });
    return { type: "dir", entries };
  }
  return { type: "file", body: readText(abs) };
}

function assertEditable(relPath: string): { abs: string; rel: string } {
  const cleaned = relPath.replace(/^\/+/, "").replace(/\0/g, "");
  if (!cleaned || cleaned.includes("..")) {
    throw new Error("invalid path");
  }
  const top = cleaned.split("/")[0];
  if (!(EDITABLE_ROOTS as readonly string[]).includes(top)) {
    throw new Error(
      `only ${EDITABLE_ROOTS.join(", ")} are editable (shared harness files)`
    );
  }
  const abs = safeJoin(cleaned);
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
  if (!["md", "yaml", "yml", "json"].includes(ext)) {
    throw new Error("allowed extensions: .md, .yaml, .yml, .json");
  }
  return n;
}

function defaultContent(relPath: string): string {
  const name = basename(relPath);
  const stem = name.replace(/\.(md|ya?ml|json)$/i, "");
  const title = stem
    .replace(/[-_]/g, " ")
    .replace(/\b\w/g, (c) => c.toUpperCase());
  const lower = name.toLowerCase();
  if (lower.endsWith(".md")) {
    return `# ${title}\n\n`;
  }
  if (lower.endsWith(".json")) {
    return "{\n}\n";
  }
  // yaml
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
  const fallbackExt =
    top === "knowledge" || top === "rules" || top === "skills" ? ".md" : ".yaml";
  const name = sanitizeFileName(fileName, fallbackExt);
  const rel = `${top}/${name}`;
  const abs = safeJoin(rel);
  if (abs && fileExists(abs)) {
    throw new Error("file already exists");
  }
  const body =
    typeof bodyOverride === "string" ? bodyOverride : defaultContent(rel);
  writeHarnessFile(rel, body, { create: true });
  return { path: rel, body };
}

export function deleteHarnessFile(relPath: string): void {
  const { abs } = assertEditable(relPath);
  if (!fileExists(abs) || statSync(abs).isDirectory()) {
    throw new Error("file not found");
  }
  // Protect seed filenames from accidental wipe via UI delete? Allow delete of user files.
  unlinkSync(abs);
}
