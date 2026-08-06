/** Structured memory line items (preferences / lessons / projects). */

export const MEMORY_VERBS = [
  { value: "prefer", label: "Prefer" },
  { value: "always", label: "Always" },
  { value: "never", label: "Never" },
  { value: "ask", label: "Ask" },
  { value: "avoid", label: "Avoid" },
] as const;

export type MemoryVerb = (typeof MEMORY_VERBS)[number]["value"];

export type MemoryLine = {
  id: string;
  verb: MemoryVerb | "";
  text: string;
};

export const PROJECT_SECTIONS = [
  { id: "current-focus", title: "Current focus" },
  { id: "active-areas", title: "Active areas" },
  { id: "do-not-touch", title: "Do not touch" },
  { id: "notes", title: "Notes" },
] as const;

export type ProjectSectionId = (typeof PROJECT_SECTIONS)[number]["id"];

export type ProjectSection = {
  id: ProjectSectionId | string;
  title: string;
  items: MemoryLine[];
};

const VERB_RE =
  /^(prefer|always|never|ask|avoid|no)\b[\s:-]*/i;
const ID_RE = /<!--\s*id:([a-zA-Z0-9_-]+)\s*-->\s*$/;

function slugId(prefix: string, text: string, index: number): string {
  const base = text
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 32);
  return `${prefix}_${base || "item"}_${index}`;
}

function stripIdComment(raw: string): { id: string | null; text: string } {
  const m = raw.match(ID_RE);
  if (!m) return { id: null, text: raw.trim() };
  return {
    id: m[1],
    text: raw.replace(ID_RE, "").trim(),
  };
}

function withIdComment(formatted: string, id: string): string {
  const bare = formatted.replace(ID_RE, "").trimEnd();
  return `${bare} <!-- id:${id} -->`;
}

function parseVerbAndText(raw: string): { verb: MemoryVerb | ""; text: string } {
  const t = raw.trim().replace(/^[-*•]\s*/, "");
  const m = t.match(VERB_RE);
  if (!m) return { verb: "", text: t };
  let verb = m[1].toLowerCase() as MemoryVerb | "no";
  if (verb === "no") verb = "avoid";
  const rest = t.slice(m[0].length).trim();
  return { verb: verb as MemoryVerb, text: rest || t };
}

function formatLine(verb: MemoryVerb | "", text: string): string {
  const body = text.trim();
  if (!body) return "";
  if (!verb) return body;
  const label = MEMORY_VERBS.find((v) => v.value === verb)?.label || verb;
  return `${label} ${body}`;
}

export function parsePreferenceItems(md: string): MemoryLine[] {
  const items: MemoryLine[] = [];
  let i = 0;
  for (const line of md.split("\n")) {
    const t = line.trim();
    if (!t.startsWith("- ") && !t.startsWith("* ")) continue;
    const raw = t.slice(2).trim();
    if (!raw || /^\(.*\)$/.test(raw)) continue;
    const { id: existingId, text: withoutId } = stripIdComment(raw);
    const { verb, text } = parseVerbAndText(withoutId);
    if (!text) continue;
    items.push({
      id: existingId || slugId("pref", text, i),
      verb,
      text,
    });
    i++;
  }
  return items;
}

export function serializePreferences(items: MemoryLine[]): string {
  const lines = [
    "# Preferences",
    "",
    "How agents should work in this workspace.",
    "",
  ];
  items.forEach((it, i) => {
    const formatted = formatLine(it.verb, it.text);
    if (!formatted) return;
    const id = it.id || slugId("pref", it.text, i);
    lines.push(`- ${withIdComment(formatted, id)}`);
  });
  if (items.length === 0) {
    lines.push(
      `- ${withIdComment("Prefer `so graph query` before broad code search.", "pref_default_0")}`
    );
  }
  return lines.join("\n") + "\n";
}

export function parseProjectSections(md: string): ProjectSection[] {
  const byId = new Map<string, ProjectSection>();
  for (const s of PROJECT_SECTIONS) {
    byId.set(s.id, { id: s.id, title: s.title, items: [] });
  }

  let current: ProjectSection | null = byId.get("notes") || null;
  let i = 0;
  for (const line of md.split("\n")) {
    const h2 = line.match(/^##\s+(.+)\s*$/);
    if (h2) {
      const title = h2[1].trim();
      const known = PROJECT_SECTIONS.find(
        (s) => s.title.toLowerCase() === title.toLowerCase()
      );
      const id = known?.id || title.toLowerCase().replace(/[^a-z0-9]+/g, "-");
      if (!byId.has(id)) {
        byId.set(id, { id, title, items: [] });
      }
      current = byId.get(id)!;
      current.title = title;
      continue;
    }
    const t = line.trim();
    if (!current || (!t.startsWith("- ") && !t.startsWith("* "))) continue;
    const raw = t.slice(2).trim();
    if (!raw || /^\(.*\)$/.test(raw)) continue;
    const { id: existingId, text: withoutId } = stripIdComment(raw);
    const { verb, text } = parseVerbAndText(withoutId);
    current.items.push({
      id: existingId || slugId("proj", text || withoutId, i++),
      verb: verb,
      text: verb ? text : withoutId,
    });
  }

  const out: ProjectSection[] = [];
  for (const s of PROJECT_SECTIONS) {
    out.push(byId.get(s.id)!);
    byId.delete(s.id);
  }
  for (const s of byId.values()) out.push(s);
  return out;
}

export function serializeProjects(sections: ProjectSection[]): string {
  const lines = ["# Projects", ""];
  for (const s of sections) {
    lines.push(`## ${s.title}`, "");
    if (s.items.length === 0) {
      lines.push(`- (${s.title} - add items)`);
    } else {
      s.items.forEach((it, i) => {
        const formatted = it.verb
          ? formatLine(it.verb, it.text)
          : it.text.trim();
        if (!formatted) return;
        const id = it.id || slugId("proj", it.text, i);
        lines.push(`- ${withIdComment(formatted, id)}`);
      });
    }
    lines.push("");
  }
  return lines.join("\n").replace(/\n{3,}/g, "\n\n") + "\n";
}

export function lessonDisplay(raw: string): { verb: MemoryVerb | ""; text: string } {
  return parseVerbAndText(raw);
}

export function composeLessonText(verb: MemoryVerb | "", text: string): string {
  return formatLine(verb, text);
}
