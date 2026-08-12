import { mkdirSync, writeFileSync } from "fs";
import { fileExists, readJSONFile, readText } from "./nodeio";
import { join } from "path";
import { soPath } from "./root";
import {
  composeLessonText,
  parsePreferenceItems,
  parseProjectSections,
  serializePreferences,
  serializeProjects,
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

function saveState(state: MemoryState) {
	ensureMemoryDirs();
	writeFileSync(statePath(), JSON.stringify(state, null, 2) + "\n", "utf8");
}

function writeLessons(lessons: Lesson[]) {
  ensureMemoryDirs();
	const state = loadState(); state.lessons = lessons; saveState(state);
}

function ensureMemoryDirs() {
  const dir = soPath("memory");
  if (!fileExists(dir)) mkdirSync(dir, { recursive: true });
}

export function listLessons(): Lesson[] {
  ensureMemoryDirs();
	return loadState().lessons || [];
}

export function listPatterns(): MemoryPattern[] {
  return loadState().patterns || [];
}

export function deleteLessonLocal(id: string): void {
  writeLessons(listLessons().filter((l) => l.id !== id));
}

export function listPreferenceItems(): MemoryLine[] {
  return parsePreferenceItems(readMarkdown("preferences.md"));
}

function savePreferenceItems(items: MemoryLine[]): void {
  writeMarkdown("preferences.md", serializePreferences(items));
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
  savePreferenceItems(items);
  return items;
}

export function deletePreferenceItem(id: string): MemoryLine[] {
  const items = listPreferenceItems().filter((i) => i.id !== id);
  savePreferenceItems(items);
  return items;
}

export function listProjectSections(): ProjectSection[] {
  return parseProjectSections(readMarkdown("projects.md"));
}

function saveProjectSections(sections: ProjectSection[]): void {
  writeMarkdown("projects.md", serializeProjects(sections));
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
  saveProjectSections(sections);
  return sections;
}

export function deleteProjectItem(sectionId: string, id: string): ProjectSection[] {
  const sections = listProjectSections();
  const section = sections.find((s) => s.id === sectionId);
  if (!section) throw new Error("section not found");
  section.items = section.items.filter((i) => i.id !== id);
  saveProjectSections(sections);
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

export function writeMarkdown(name: string, body: string) {
	const state = loadState();
	if (name === "preferences.md") state.preferences = body;
	if (name === "projects.md") state.projects = body;
	saveState(state);
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
