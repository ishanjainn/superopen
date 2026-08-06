import { mkdirSync, writeFileSync } from "fs";
import { fileExists, readText } from "./nodeio";
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

function lessonsPath() {
  return join(soPath("memory"), "lessons.jsonl");
}

function writeLessons(lessons: Lesson[]) {
  ensureMemoryDirs();
  const body = lessons.map((l) => JSON.stringify(l)).join("\n");
  writeFileSync(lessonsPath(), body ? body + "\n" : "", "utf8");
}

function ensureMemoryDirs() {
  const dir = soPath("memory");
  if (!fileExists(dir)) mkdirSync(dir, { recursive: true });
  const hist = join(dir, "history");
  if (!fileExists(hist)) mkdirSync(hist, { recursive: true });
}

export function listLessons(): Lesson[] {
  ensureMemoryDirs();
  const p = lessonsPath();
  if (!fileExists(p)) return [];
  const out: Lesson[] = [];
  for (const line of readText(p).split("\n")) {
    if (!line.trim()) continue;
    try {
      out.push(JSON.parse(line) as Lesson);
    } catch {
      /* skip */
    }
  }
  return out;
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
  const p = join(soPath("memory"), "active-context.md");
  if (!fileExists(p)) return "";
  return readText(p);
}

export function readMarkdown(name: string): string {
  const p = join(soPath("memory"), name);
  if (!fileExists(p)) return "";
  return readText(p);
}

export function writeMarkdown(name: string, body: string) {
  ensureMemoryDirs();
  writeFileSync(join(soPath("memory"), name), body, "utf8");
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
