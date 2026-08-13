import { createHash } from "crypto";
import { readFileSync } from "fs";
import { fileExists, readJSONFile, readText } from "./nodeio";
import { isAbsolute, join } from "path";
import { repoRoot, soPath } from "./root";
import {
  composeLessonText,
  parsePreferenceItems,
  parseProjectSections,
  type MemoryLine,
  type MemoryVerb,
  type ProjectSection,
  type ProjectSectionId,
} from "./memory-items";

export type Lesson = {
  id: string;
  text: string;
  scope?: string;
  confidence?: number;
  source_session?: string;
  created_at?: string;
};

export type MemoryPattern = {
  fingerprint: string;
  vendor: string;
  kind: string;
  change_kind?: string;
  target_type?: string;
  target_path?: string;
  summary: string;
  evidence?: string[];
  occurrences: number;
  session_ids?: string[];
  verified_sessions?: string[];
  confidence?: number;
  explicit_workflow?: boolean;
  status: string;
  first_observed_at?: string;
  last_observed_at?: string;
	scope?: "vendor" | "shared";
	applicability?: string;
	paths?: string[];
	symbols?: string[];
	error_signatures?: string[];
	retrieval_count?: number;
	helpful_count?: number;
	incorrect_count?: number;
	contradiction_count?: number;
	last_retrieved_at?: string;
	last_verified_at?: string;
	status_reason?: string;
	source_sha256?: string;
	guidance_sha256?: string;
	freshness?: "current" | "stale" | "unanchored";
};

type MemoryState = {
	_about: { purpose: string; authority: string; updated_by: string };
	lessons?: Lesson[];
	preferences?: string;
	projects?: string;
	semantic?: unknown[];
	episodic?: unknown[];
	history?: string[];
	patterns?: MemoryPattern[];
};

function statePath() { return join(soPath("memory"), "state.json"); }

function loadState(): MemoryState {
	return readJSONFile<MemoryState>(statePath()) || {
		_about: {
			purpose: "Consolidated lessons, preferences, project notes, harvest cursor, and memory refresh state.",
			authority: "local durable memory state",
			updated_by: "session review and memory consolidation",
		},
		lessons: [], preferences: "", projects: "",
	};
}

export function listLessons(): Lesson[] {
	return loadState().lessons || [];
}

export function listPatterns(): MemoryPattern[] {
	return (loadState().patterns || []).map((pattern) => {
		const expected = pattern.status === "applied" && pattern.guidance_sha256 ? pattern.guidance_sha256 : pattern.source_sha256;
		if (!expected) return { ...pattern, freshness: "unanchored" };
		const target = pattern.target_path || pattern.paths?.[0];
		if (!target) return { ...pattern, freshness: "stale" };
		try {
			const path = isAbsolute(target) ? target : join(repoRoot(), target);
			const actual = createHash("sha256").update(readFileSync(path)).digest("hex");
			return { ...pattern, freshness: actual === expected ? "current" : "stale" };
		} catch { return { ...pattern, freshness: "stale" }; }
	});
}

export function listPreferenceItems(): MemoryLine[] {
  return parsePreferenceItems(readMarkdown("preferences.md"));
}

export function upsertPreferenceItem(input: {
  id?: string;
  verb: MemoryVerb | "";
  text: string;
}): MemoryLine[] {
  const items = listPreferenceItems();
  const text = input.text.trim();
  if (!text) throw new Error("text required");
  if (!input.verb) throw new Error("pick a verb");
  if (input.id) {
    const idx = items.findIndex((i) => i.id === input.id);
    if (idx < 0) throw new Error("item not found");
    items[idx] = { ...items[idx], verb: input.verb, text };
  } else {
    items.push({
      id: `pref_${Date.now().toString(36)}`,
      verb: input.verb,
      text,
    });
  }
  return items;
}

export function deletePreferenceItem(id: string): MemoryLine[] {
  const items = listPreferenceItems().filter((i) => i.id !== id);
  return items;
}

export function listProjectSections(): ProjectSection[] {
  return parseProjectSections(readMarkdown("projects.md"));
}

export function upsertProjectItem(input: {
  sectionId: string;
  id?: string;
  verb?: MemoryVerb | "";
  text: string;
}): ProjectSection[] {
  const sections = listProjectSections();
  const section = sections.find((s) => s.id === input.sectionId);
  if (!section) throw new Error("section not found");
  const text = input.text.trim();
  if (!text) throw new Error("text required");
  const verb = input.verb || "";
  if (input.id) {
    const idx = section.items.findIndex((i) => i.id === input.id);
    if (idx < 0) throw new Error("item not found");
    section.items[idx] = { ...section.items[idx], verb, text };
  } else {
    section.items.push({
      id: `proj_${Date.now().toString(36)}`,
      verb,
      text,
    });
  }
  return sections;
}

export function deleteProjectItem(sectionId: string, id: string): ProjectSection[] {
  const sections = listProjectSections();
  const section = sections.find((s) => s.id === sectionId);
  if (!section) throw new Error("section not found");
  section.items = section.items.filter((i) => i.id !== id);
  return sections;
}

export function readActivePack(): string {
	const p = join(soPath("memory"), "context.md");
  if (!fileExists(p)) return "";
  return readText(p);
}

export function readMarkdown(name: string): string {
	const state = loadState();
	if (name === "preferences.md") return state.preferences || "";
	if (name === "projects.md") return state.projects || "";
	return "";
}

export function isStubMarkdown(content: string): boolean {
  const c = content.trim();
  if (!c) return true;
  let nonHeading = 0;
  for (const line of c.split("\n")) {
    const t = line.trim();
    if (!t) continue;
    if (t.startsWith("#")) continue;
    nonHeading++;
  }
  return nonHeading === 0 || c.length < 40;
}

export { composeLessonText };
export type { MemoryLine, MemoryVerb, ProjectSection, ProjectSectionId };
