"use client";

import { useEffect, useMemo, useRef, useState, useCallback } from "react";
import { Pencil, Plus, Search, Trash2, X } from "lucide-react";
import FeaturePageHeader from "@/components/shell/feature-page-header";
import { MarkdownView } from "@/components/markdown-view";
import { Dropdown } from "@/components/ui/dropdown";
import {
  MEMORY_VERBS,
  PROJECT_SECTIONS,
  lessonDisplay,
  type MemoryLine,
  type MemoryVerb,
  type ProjectSection,
} from "@/lib/so/memory-items";
import { cn } from "@/lib/utils";
import { useSoftPoll } from "@/hooks/use-soft-poll";

type Lesson = { id: string; text: string; created_at?: string };
type Pattern = {
  fingerprint: string; vendor: string; kind: string; summary: string;
  occurrences: number; status: string; confidence?: number; target_path?: string;
  verified_sessions?: string[]; session_ids?: string[];
};
type Section = "active" | "lessons" | "patterns" | "prefs" | "projects";
type Pack = { text?: string };

const SECTIONS: { id: Section; label: string; hint: string }[] = [
  {
    id: "active",
    label: "Active Context",
    hint: "Injected on every SessionStart",
  },
  {
    id: "lessons",
    label: "Lessons",
    hint: "Corrections learned from sessions",
  },
  {
    id: "patterns",
    label: "Review patterns",
    hint: "Evidence accumulated across sessions",
  },
  {
    id: "prefs",
    label: "Preferences",
    hint: "Standing rules for how agents work",
  },
  {
    id: "projects",
    label: "Projects",
    hint: "Focus, active areas, boundaries",
  },
];

const VERB_OPTIONS = MEMORY_VERBS.map((v) => ({
  value: v.value,
  label: v.label,
}));

export default function MemoryPage() {
  const [section, setSection] = useState<Section>("active");
  const [lessons, setLessons] = useState<Lesson[]>([]);
  const [patterns, setPatterns] = useState<Pattern[]>([]);
  const [prefItems, setPrefItems] = useState<MemoryLine[]>([]);
  const [projectSections, setProjectSections] = useState<ProjectSection[]>([]);
  const [pack, setPack] = useState<Pack | null>(null);
  const [q, setQ] = useState("");
  const [hits, setHits] = useState<any[]>([]);
  const [error, setError] = useState("");
  const [composerOpen, setComposerOpen] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [draftVerb, setDraftVerb] = useState<MemoryVerb | "">("prefer");
  const [draftText, setDraftText] = useState("");
  const [draftSectionId, setDraftSectionId] = useState<string>(
    PROJECT_SECTIONS[0].id
  );
  const draftRef = useRef<HTMLInputElement | null>(null);

  async function loadLessons() {
    const r = await fetch("/api/memory?op=lessons");
    const data = await r.json();
    setLessons(Array.isArray(data) ? data : data?.data || []);
  }

  async function loadPrefs() {
    const r = await fetch("/api/memory?op=preferences");
    const j = await r.json();
    setPrefItems(Array.isArray(j.items) ? j.items : []);
  }

  async function loadPatterns() {
    const r = await fetch("/api/memory?op=patterns");
    const data = await r.json();
    setPatterns(Array.isArray(data) ? data : []);
  }

  async function loadProjects() {
    const r = await fetch("/api/memory?op=projects");
    const j = await r.json();
    setProjectSections(Array.isArray(j.sections) ? j.sections : []);
  }

  async function loadActive() {
    setError("");
    const r = await fetch("/api/memory?op=active");
    const j = await r.json();
    const text = typeof j.text === "string" ? j.text : "";
    setPack({
      text: text || "_No active context yet. Click Refresh to build it._",
    });
  }

  useEffect(() => {
    void loadLessons();
  }, []);

  useEffect(() => {
    setError("");
    setComposerOpen(false);
    setEditingId(null);
    setDraftText("");
    setDraftVerb("prefer");
    setHits([]);
    setQ("");
    if (section === "active") void loadActive();
    if (section === "lessons") void loadLessons();
    if (section === "patterns") void loadPatterns();
    if (section === "prefs") void loadPrefs();
    if (section === "projects") void loadProjects();
  }, [section]);

  const softRefresh = useCallback(() => {
    if (section === "active") void loadActive();
    if (section === "lessons") void loadLessons();
    if (section === "patterns") void loadPatterns();
    if (section === "prefs") void loadPrefs();
    if (section === "projects") void loadProjects();
  }, [section]);

  useSoftPoll(softRefresh, 10000);

  useEffect(() => {
    if (composerOpen || editingId) {
      requestAnimationFrame(() => draftRef.current?.focus());
    }
  }, [composerOpen, editingId]);

  const filteredLessons = useMemo(() => {
    const needle = q.trim().toLowerCase();
    if (!needle) return lessons;
    return lessons.filter((l) => l.text.toLowerCase().includes(needle));
  }, [lessons, q]);

  const filteredPrefs = useMemo(() => {
    const needle = q.trim().toLowerCase();
    if (!needle) return prefItems;
    return prefItems.filter((p) =>
      `${p.verb} ${p.text}`.toLowerCase().includes(needle)
    );
  }, [prefItems, q]);

  async function searchCorpus() {
    if (!q.trim()) {
      setHits([]);
      return;
    }
    const r = await fetch(`/api/memory?op=search&q=${encodeURIComponent(q)}`);
    const j = await r.json();
    setHits(Array.isArray(j.data) ? j.data : []);
  }

  function openCreate() {
    setEditingId(null);
    setDraftVerb(section === "projects" ? "" : "prefer");
    setDraftText("");
    setDraftSectionId(PROJECT_SECTIONS[0].id);
    setComposerOpen(true);
  }

  function openEditLesson(l: Lesson) {
    const { verb, text } = lessonDisplay(l.text);
    setEditingId(l.id);
    setDraftVerb(verb || "prefer");
    setDraftText(text);
    setComposerOpen(true);
  }

  function openEditPref(item: MemoryLine) {
    setEditingId(item.id);
    setDraftVerb(item.verb || "prefer");
    setDraftText(item.text);
    setComposerOpen(true);
  }

  function openEditProject(sectionId: string, item: MemoryLine) {
    setEditingId(item.id);
    setDraftSectionId(sectionId);
    setDraftVerb(item.verb || "");
    setDraftText(item.text);
    setComposerOpen(true);
  }

  async function saveComposer() {
    setError("");
    if (!draftText.trim()) {
      setError("Enter the statement");
      return;
    }
    if (section !== "projects" && !draftVerb) {
      setError("Pick a verb (Prefer / Always / Never / …)");
      return;
    }

    if (section === "lessons") {
      const op = editingId ? "update_lesson" : "add_lesson";
      const r = await fetch("/api/memory", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          op,
          id: editingId || undefined,
          verb: draftVerb,
          text: draftText,
        }),
      });
      const j = await r.json();
      if (j.ok === false || !r.ok) {
        setError(j.error || "save failed");
        return;
      }
      if (Array.isArray(j.lessons)) setLessons(j.lessons);
      else await loadLessons();
    }

    if (section === "prefs") {
      const r = await fetch("/api/memory", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          op: "upsert_preference",
          id: editingId || undefined,
          verb: draftVerb,
          text: draftText,
        }),
      });
      const j = await r.json();
      if (j.ok === false || !r.ok) {
        setError(j.error || "save failed");
        return;
      }
      setPrefItems(Array.isArray(j.items) ? j.items : []);
    }

    if (section === "projects") {
      const r = await fetch("/api/memory", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          op: "upsert_project_item",
          sectionId: draftSectionId,
          id: editingId || undefined,
          verb: draftVerb || "",
          text: draftText,
        }),
      });
      const j = await r.json();
      if (j.ok === false || !r.ok) {
        setError(j.error || "save failed");
        return;
      }
      setProjectSections(Array.isArray(j.sections) ? j.sections : []);
    }

    setComposerOpen(false);
    setEditingId(null);
    setDraftText("");
  }

  async function removeLesson(id: string) {
    setError("");
    const r = await fetch("/api/memory", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ op: "delete_lesson", id }),
    });
    const j = await r.json();
    if (j.ok === false) {
      setError(j.error || "delete failed");
      return;
    }
    if (Array.isArray(j.lessons)) setLessons(j.lessons);
    else await loadLessons();
  }

  async function removePref(id: string) {
    const r = await fetch("/api/memory", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ op: "delete_preference", id }),
    });
    const j = await r.json();
    if (j.ok === false) {
      setError(j.error || "delete failed");
      return;
    }
    setPrefItems(Array.isArray(j.items) ? j.items : []);
  }

  async function removeProjectItem(sectionId: string, id: string) {
    const r = await fetch("/api/memory", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ op: "delete_project_item", sectionId, id }),
    });
    const j = await r.json();
    if (j.ok === false) {
      setError(j.error || "delete failed");
      return;
    }
    setProjectSections(Array.isArray(j.sections) ? j.sections : []);
  }

  async function refreshPack() {
    setError("");
    const r = await fetch("/api/memory", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ op: "refresh" }),
    });
    const j = await r.json();
    if (j.ok === false) {
      setError(j.error || "refresh failed");
      return;
    }
    if (j.data?.text) setPack(j.data);
    else await loadActive();
  }

  const active = SECTIONS.find((s) => s.id === section)!;
  const showListChrome =
    section === "lessons" || section === "prefs" || section === "projects";

  return (
    <div className="flex h-full min-h-0 flex-col">
      <FeaturePageHeader title="Memory" />
      <div className="flex min-h-0 flex-1">
        <section className="flex w-64 shrink-0 flex-col border-r border-neutral-200 bg-white">
          <div className="border-b border-neutral-100 px-3 py-2">
            <span className="text-[11px] font-medium uppercase tracking-wide text-neutral-500">
              Sections
            </span>
          </div>
          <ul className="min-h-0 flex-1 overflow-auto">
            {SECTIONS.map((s) => (
              <li key={s.id}>
                <button
                  type="button"
                  onClick={() => setSection(s.id)}
                  className={cn(
                    "block w-full border-b border-neutral-50 px-3 py-2.5 text-left",
                    section === s.id
                      ? "bg-neutral-100 font-medium text-neutral-900"
                      : "text-neutral-700 hover:bg-neutral-50"
                  )}
                >
                  <span className="block truncate text-sm">{s.label}</span>
                  <span className="mt-0.5 block truncate text-[10px] font-normal text-neutral-400">
                    {s.hint}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </section>

        <section className="flex min-w-0 flex-1 flex-col bg-white">
          <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-neutral-100 px-4 py-2">
            <div className="min-w-0 shrink-0">
              <div className="truncate text-sm font-medium text-neutral-900">
                {active.label}
              </div>
              <div className="truncate text-[11px] text-neutral-400">
                {active.hint}
              </div>
            </div>

            {showListChrome && (
              <>
                <div className="relative ml-auto min-w-0 max-w-md flex-1">
                  <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-neutral-400" />
                  <input
                    className="h-8 w-full rounded-md border border-neutral-200 bg-neutral-50 pl-8 pr-3 text-sm outline-none placeholder:text-neutral-400 focus:border-neutral-400 focus:bg-white"
                    placeholder={
                      section === "lessons"
                        ? "Filter lessons…"
                        : section === "prefs"
                          ? "Filter preferences…"
                          : "Filter project notes…"
                    }
                    value={q}
                    onChange={(e) => setQ(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" && section === "lessons") {
                        void searchCorpus();
                      }
                    }}
                  />
                </div>
                <button
                  type="button"
                  title="Add"
                  aria-label="Add"
                  className={cn(
                    "inline-flex size-8 shrink-0 items-center justify-center rounded-md border border-neutral-200 text-neutral-700 hover:bg-neutral-50",
                    composerOpen &&
                      !editingId &&
                      "border-neutral-900 bg-neutral-900 text-white hover:bg-neutral-800"
                  )}
                  onClick={() => {
                    if (composerOpen && !editingId) {
                      setComposerOpen(false);
                      return;
                    }
                    openCreate();
                  }}
                >
                  {composerOpen && !editingId ? (
                    <X className="size-4" />
                  ) : (
                    <Plus className="size-4" />
                  )}
                </button>
              </>
            )}

            {section === "active" && (
              <button
                type="button"
                className="ml-auto rounded border border-neutral-200 px-2.5 py-1 text-xs hover:bg-neutral-50"
                onClick={() => void refreshPack()}
              >
                Refresh
              </button>
            )}
          </div>

          {error && (
            <p className="border-b border-red-100 bg-red-50 px-4 py-1.5 text-xs text-red-700">
              {error}
            </p>
          )}

          {composerOpen && showListChrome && (
            <ComposerBar
              section={section}
              editing={Boolean(editingId)}
              verb={draftVerb}
              text={draftText}
              sectionId={draftSectionId}
              inputRef={draftRef}
              onVerb={setDraftVerb}
              onText={setDraftText}
              onSectionId={setDraftSectionId}
              onCancel={() => {
                setComposerOpen(false);
                setEditingId(null);
                setDraftText("");
              }}
              onSave={() => void saveComposer()}
            />
          )}

          <div className="min-h-0 flex-1 overflow-auto p-6">
            {section === "active" && (
              <div className="mx-auto max-w-3xl">
                <article className="rounded-lg border border-neutral-200 bg-white p-4">
                  <MarkdownView source={pack?.text || "Loading…"} />
                </article>
              </div>
            )}

            {section === "lessons" && (
              <div className="mx-auto flex max-w-3xl flex-col gap-3">
                <p className="text-xs text-neutral-500">
                  Lessons are corrections from real sessions. Preferences are standing
                  workspace rules - same verbs, different job.
                </p>
                {hits.length > 0 && (
                  <ul className="space-y-2 text-sm">
                    {hits.map((h, i) => (
                      <li
                        key={i}
                        className="rounded-xl border border-neutral-200 px-4 py-3"
                      >
                        <span className="text-xs text-neutral-500">{h.kind}</span>
                        <div>{h.snippet}</div>
                      </li>
                    ))}
                  </ul>
                )}
                <ItemList
                  empty="No lessons yet. Use + to add one with a verb."
                  items={filteredLessons.map((l) => {
                    const { verb, text } = lessonDisplay(l.text);
                    return {
                      id: l.id,
                      verb,
                      text: text || l.text,
                      onEdit: () => openEditLesson(l),
                      onDelete: () => void removeLesson(l.id),
                    };
                  })}
                />
              </div>
            )}

            {section === "patterns" && (
              <div className="min-h-0 flex-1 overflow-auto p-4">
                <div className="space-y-3">
                  {patterns.length === 0 && (
                    <p className="text-sm text-neutral-500">No repeated review patterns yet.</p>
                  )}
                  {patterns.map((pattern) => (
                    <article key={`${pattern.vendor}:${pattern.fingerprint}`} className="rounded-lg border border-neutral-200 p-4">
                      <div className="flex flex-wrap items-center gap-2 text-[11px] text-neutral-500">
                        <span className="font-semibold uppercase tracking-wide">{pattern.kind}</span>
                        <span>{pattern.vendor}</span>
                        <span>{pattern.occurrences} supporting session{pattern.occurrences === 1 ? "" : "s"}</span>
                        <span>{pattern.verified_sessions?.length || 0} verified</span>
                        <span>{pattern.status}</span>
                      </div>
                      <p className="mt-2 text-sm text-neutral-800">{pattern.summary}</p>
                      {pattern.target_path && <p className="mt-2 font-mono text-[11px] text-neutral-500">{pattern.target_path}</p>}
                    </article>
                  ))}
                </div>
              </div>
            )}

            {section === "prefs" && (
              <div className="mx-auto flex max-w-3xl flex-col gap-3">
                <p className="text-xs text-neutral-500">
                  Standing how-to-work rules for agents in this repo. Each line needs a
                  verb (Prefer, Never, Always…).
                </p>
                <ItemList
                  empty="No preferences yet. Use + to add one."
                  items={filteredPrefs.map((p) => ({
                    id: p.id,
                    verb: p.verb,
                    text: p.text,
                    onEdit: () => openEditPref(p),
                    onDelete: () => void removePref(p.id),
                  }))}
                />
              </div>
            )}

            {section === "projects" && (
              <div className="mx-auto flex max-w-3xl flex-col gap-6">
                <p className="text-xs text-neutral-500">
                  Fixed sections - add notes under each. Verbs are optional here.
                </p>
                {projectSections.map((s) => {
                  const needle = q.trim().toLowerCase();
                  const items = needle
                    ? s.items.filter((it) =>
                        `${it.verb} ${it.text}`.toLowerCase().includes(needle)
                      )
                    : s.items;
                  return (
                    <div key={s.id}>
                      <div className="mb-2 flex items-center justify-between gap-2">
                        <h3 className="text-xs font-semibold uppercase tracking-wide text-neutral-500">
                          {s.title}
                        </h3>
                        <button
                          type="button"
                          className="inline-flex items-center gap-1 rounded border border-neutral-200 px-2 py-0.5 text-[11px] text-neutral-600 hover:bg-neutral-50"
                          onClick={() => {
                            setDraftSectionId(s.id);
                            setEditingId(null);
                            setDraftVerb("");
                            setDraftText("");
                            setComposerOpen(true);
                          }}
                        >
                          <Plus className="size-3" />
                          Add
                        </button>
                      </div>
                      <ItemList
                        empty={`Nothing in ${s.title.toLowerCase()} yet.`}
                        items={items.map((it) => ({
                          id: it.id,
                          verb: it.verb,
                          text: it.text,
                          onEdit: () => openEditProject(s.id, it),
                          onDelete: () => void removeProjectItem(s.id, it.id),
                        }))}
                      />
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </section>
      </div>
    </div>
  );
}

function VerbBadge({ verb }: { verb: MemoryVerb | "" }) {
  if (!verb) return null;
  const label = MEMORY_VERBS.find((v) => v.value === verb)?.label || verb;
  return (
    <span className="shrink-0 rounded-full bg-neutral-100 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-neutral-600">
      {label}
    </span>
  );
}

function ItemList({
  items,
  empty,
}: {
  empty: string;
  items: {
    id: string;
    verb: MemoryVerb | "";
    text: string;
    onEdit: () => void;
    onDelete: () => void;
  }[];
}) {
  if (items.length === 0) {
    return <p className="text-sm text-neutral-500">{empty}</p>;
  }
  return (
    <ul className="space-y-2">
      {items.map((it) => (
        <li
          key={it.id}
          className="group flex items-start gap-3 rounded-xl border border-neutral-200 px-4 py-3 text-sm"
        >
          <VerbBadge verb={it.verb} />
          <div className="min-w-0 flex-1 text-neutral-900">{it.text}</div>
          <div className="flex shrink-0 gap-1 opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100">
            <button
              type="button"
              className="rounded p-1.5 text-neutral-400 hover:bg-neutral-100 hover:text-neutral-800"
              aria-label="Edit"
              onClick={it.onEdit}
            >
              <Pencil className="size-3.5" />
            </button>
            <button
              type="button"
              className="rounded p-1.5 text-neutral-400 hover:bg-red-50 hover:text-red-600"
              aria-label="Delete"
              onClick={it.onDelete}
            >
              <Trash2 className="size-3.5" />
            </button>
          </div>
        </li>
      ))}
    </ul>
  );
}

function ComposerBar({
  section,
  editing,
  verb,
  text,
  sectionId,
  inputRef,
  onVerb,
  onText,
  onSectionId,
  onCancel,
  onSave,
}: {
  section: Section;
  editing: boolean;
  verb: MemoryVerb | "";
  text: string;
  sectionId: string;
  inputRef: React.RefObject<HTMLInputElement | null>;
  onVerb: (v: MemoryVerb | "") => void;
  onText: (t: string) => void;
  onSectionId: (id: string) => void;
  onCancel: () => void;
  onSave: () => void;
}) {
  const needsVerb = section === "lessons" || section === "prefs";
  return (
    <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-neutral-100 bg-neutral-50/80 px-4 py-2">
      {section === "projects" && (
        <div className="w-40 shrink-0">
          <Dropdown
            size="sm"
            aria-label="Project section"
            value={sectionId}
            onChange={onSectionId}
            options={PROJECT_SECTIONS.map((s) => ({
              value: s.id,
              label: s.title,
            }))}
          />
        </div>
      )}
      {(needsVerb || section === "projects") && (
        <div className="w-28 shrink-0">
          <Dropdown
            size="sm"
            aria-label="Verb"
            value={verb || (needsVerb ? "prefer" : "__none__")}
            onChange={(v) =>
              onVerb(v === "__none__" ? "" : (v as MemoryVerb))
            }
            options={
              needsVerb
                ? VERB_OPTIONS
                : [{ value: "__none__", label: "None" }, ...VERB_OPTIONS]
            }
            placeholder="Verb"
          />
        </div>
      )}
      <input
        ref={inputRef as React.RefObject<HTMLInputElement>}
        className="h-8 min-w-[12rem] flex-1 rounded-md border border-neutral-200 bg-white px-3 text-sm outline-none focus:border-neutral-400"
        placeholder={
          needsVerb
            ? "the rest of the rule…"
            : "Note for this section…"
        }
        value={text}
        onChange={(e) => onText(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") onSave();
          if (e.key === "Escape") onCancel();
        }}
      />
      <button
        type="button"
        className="rounded-md bg-neutral-900 px-3 py-1.5 text-xs text-white disabled:opacity-50"
        disabled={!text.trim() || (needsVerb && !verb)}
        onClick={onSave}
      >
        {editing ? "Save" : "Add"}
      </button>
      <button
        type="button"
        className="rounded px-2 py-1.5 text-xs text-neutral-500"
        onClick={onCancel}
      >
        Cancel
      </button>
    </div>
  );
}
