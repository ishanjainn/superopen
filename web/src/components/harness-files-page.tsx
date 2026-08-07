"use client";

import { Suspense, useCallback, useEffect, useState } from "react";
import FeaturePageHeader from "@/components/shell/feature-page-header";
import { FileContentView } from "@/components/file-content-view";
import { useBreadcrumbCrumb } from "@/components/shell/breadcrumb-context";
import { useStringQueryParam } from "@/hooks/use-flag-query-param";

type FileEntry = { name: string; path: string; isDir: boolean };

/** Unique URL/query key under a harness dir (skills all share basename SKILL.md). */
function fileParamFromPath(dir: string, path: string): string {
  const prefix = `${dir}/`;
  if (path.startsWith(prefix)) {
    const rest = path.slice(prefix.length);
    // skills/<vendor>/<name>/SKILL.md → <vendor>/<name> (matches entry.name)
    if (dir === "skills" && rest.endsWith("/SKILL.md")) {
      return rest.slice(0, -"/SKILL.md".length);
    }
    return rest;
  }
  const parts = path.split("/");
  return parts[parts.length - 1] || path;
}

function displayLabel(dir: string, path: string, entries: FileEntry[]): string {
  const hit = entries.find((e) => e.path === path);
  if (hit?.name) return hit.name;
  return fileParamFromPath(dir, path);
}

function matchEntry(
  dir: string,
  list: FileEntry[],
  fileParam: string
): FileEntry | undefined {
  const q = fileParam.trim().replace(/^\/+/, "");
  if (!q) return undefined;
  const underDir = `${dir}/${q}`;
  return (
    list.find((e) => !e.isDir && e.path === q) ||
    list.find((e) => !e.isDir && e.path === underDir) ||
    list.find((e) => !e.isDir && e.name === q) ||
    // skills: ?file=cursor/foo matches path …/cursor/foo/SKILL.md
    list.find(
      (e) =>
        !e.isDir &&
        (e.path === `${underDir}/SKILL.md` || e.path.endsWith(`/${q}/SKILL.md`))
    ) ||
    // Unique basenames only (e.g. rules/coding.md) — never SKILL.md alone
    (q !== "SKILL.md" && !q.includes("/")
      ? list.find((e) => !e.isDir && e.path.endsWith("/" + q))
      : undefined)
  );
}

export function HarnessFilesPage(props: {
  title: string;
  dir: string;
  emptyHint: string;
  /** Optional per-filename helper text in the file list / header. */
  fileHints?: Record<string, string>;
}) {
  return (
    <Suspense
      fallback={
        <div className="grid h-full place-items-center text-sm text-neutral-500">
          Loading {props.title.toLowerCase()}…
        </div>
      }
    >
      <HarnessFilesPageInner {...props} />
    </Suspense>
  );
}

function HarnessFilesPageInner({
  title,
  dir,
  emptyHint,
  fileHints,
}: {
  title: string;
  dir: string;
  emptyHint: string;
  fileHints?: Record<string, string>;
}) {
  const [fileParam, setFileParam] = useStringQueryParam("file");
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [content, setContent] = useState("");
  const [draft, setDraft] = useState("");
  const [editing, setEditing] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [status, setStatus] = useState("");
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState("");

  const crumbLabel = selected ? displayLabel(dir, selected, entries) : null;
  useBreadcrumbCrumb(crumbLabel);

  const reloadList = useCallback(async () => {
    const r = await fetch(`/api/files/${dir}`);
    if (!r.ok) throw new Error(await r.text());
    const data = await r.json();
    const list: FileEntry[] = Array.isArray(data.entries) ? data.entries : [];
    setEntries(list);
    return list;
  }, [dir]);

  const openFile = useCallback(
    async (path: string, edit = false, syncUrl = true) => {
      setSelected(path);
      setError("");
      setStatus("");
      setEditing(edit);
      if (syncUrl) setFileParam(fileParamFromPath(dir, path));
      const r = await fetch(`/api/files/${path}`);
      if (!r.ok) {
        setError(await r.text());
        return;
      }
      const text = await r.text();
      setContent(text);
      setDraft(text);
    },
    [dir, setFileParam]
  );

  useEffect(() => {
    setEditing(false);
    setCreating(false);
    setError("");
    setStatus("");
    let cancelled = false;
    reloadList()
      .then((list) => {
        if (cancelled) return;
        const fromUrl = matchEntry(dir, list, fileParam);
        const first = list.find((e) => !e.isDir);
        const pick = fromUrl || first;
        if (pick) {
          void openFile(pick.path, false, !fromUrl);
        } else {
          setSelected(null);
          setContent("");
          setDraft("");
          if (fileParam) setFileParam(null);
        }
      })
      .catch((e) => {
        if (!cancelled) setError(String(e.message || e));
      });
    return () => {
      cancelled = true;
    };
    // Only re-bootstrap when the directory changes; URL-driven picks run once per dir load.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dir, reloadList, openFile]);

  // When the user navigates via back/forward and `file` changes, open that file.
  useEffect(() => {
    if (!fileParam || entries.length === 0) return;
    const match = matchEntry(dir, entries, fileParam);
    if (match && match.path !== selected) {
      void openFile(match.path, false, false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fileParam]);

  async function save() {
    if (!selected) return;
    setSaving(true);
    setError("");
    try {
      const r = await fetch(`/api/files/${selected}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ content: draft }),
      });
      if (!r.ok) {
        const j = await r.json().catch(() => ({}));
        throw new Error(j.error || (await r.text()));
      }
      setContent(draft);
      setEditing(false);
      setStatus(
        "Saved to .so/ - shared via git; teammates pick it up on pull (git hook / so refresh) or so sync."
      );
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e));
    } finally {
      setSaving(false);
    }
  }

  async function createFile() {
    setError("");
    try {
      let name = newName.trim();
      if (!name) throw new Error("Name required");
      const r = await fetch(`/api/files/${dir}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name }),
      });
      if (!r.ok) {
        const j = await r.json().catch(() => ({}));
        throw new Error(j.error || (await r.text()));
      }
      const j = (await r.json()) as { path?: string };
      setCreating(false);
      setNewName("");
      await reloadList();
      if (j.path) await openFile(j.path, true);
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e));
    }
  }

  async function removeFile() {
    if (!selected) return;
    if (!window.confirm(`Delete ${selected}?`)) return;
    setError("");
    try {
      const r = await fetch(`/api/files/${selected}`, { method: "DELETE" });
      if (!r.ok) throw new Error(await r.text());
      setSelected(null);
      setContent("");
      setDraft("");
      setFileParam(null);
      const list = await reloadList();
      const first = list.find((e) => !e.isDir);
      if (first) void openFile(first.path);
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e));
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <FeaturePageHeader title={title} />
      <div className="flex min-h-0 flex-1">
        <section className="flex w-56 shrink-0 flex-col border-r border-neutral-200 bg-white">
          <div className="flex items-center justify-between border-b border-neutral-100 px-3 py-2">
            <span className="text-[10px] font-semibold uppercase tracking-wide text-neutral-400">
              Files
            </span>
            <button
              type="button"
              className="rounded border border-neutral-200 px-1.5 py-0.5 text-[10px] text-neutral-600 hover:bg-neutral-50"
              onClick={() => {
                setCreating(true);
                setNewName("");
                setStatus("");
              }}
            >
              New
            </button>
          </div>
          {creating && (
            <div className="border-b border-neutral-100 px-2 py-2">
              <input
                autoFocus
                className="w-full rounded border border-neutral-200 px-2 py-1 text-xs outline-none focus:border-neutral-400"
                placeholder="name.md"
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") void createFile();
                  if (e.key === "Escape") setCreating(false);
                }}
              />
              <div className="mt-1.5 flex gap-1">
                <button
                  type="button"
                  className="rounded bg-neutral-900 px-2 py-0.5 text-[10px] text-white"
                  onClick={() => void createFile()}
                >
                  Create
                </button>
                <button
                  type="button"
                  className="rounded border border-neutral-200 px-2 py-0.5 text-[10px]"
                  onClick={() => setCreating(false)}
                >
                  Cancel
                </button>
              </div>
            </div>
          )}
          <div className="min-h-0 flex-1 overflow-auto">
            {error && !selected && (
              <p className="px-3 py-2 text-sm text-red-600">{error}</p>
            )}
            {!error && entries.length === 0 && !content && (
              <p className="px-3 py-3 text-sm text-neutral-500">{emptyHint}</p>
            )}
            <ul>
              {entries.map((e) => (
                <li key={e.path}>
                  {e.isDir ? (
                    <span className="block truncate px-3 py-2 text-sm text-neutral-400">
                      {e.name}/
                    </span>
                  ) : (
                    <button
                      type="button"
                      onClick={() => void openFile(e.path)}
                      className={`block w-full border-b border-neutral-50 px-3 py-2.5 text-left text-sm ${
                        selected === e.path
                          ? "bg-neutral-100 font-medium text-neutral-900"
                          : "text-neutral-700 hover:bg-neutral-50"
                      }`}
                    >
                      <span className="block truncate">{e.name}</span>
                      {fileHints?.[e.name] ? (
                        <span className="mt-0.5 block truncate text-[10px] font-normal text-neutral-400">
                          {fileHints[e.name]}
                        </span>
                      ) : null}
                    </button>
                  )}
                </li>
              ))}
            </ul>
          </div>
        </section>
        <section className="flex min-w-0 flex-1 flex-col bg-white">
          {selected ? (
            <>
              <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-neutral-100 px-4 py-2">
                <div className="min-w-0 flex-1">
                  <div className="truncate font-mono text-xs text-neutral-500">
                    {selected}
                  </div>
                  {fileHints?.[selected.split("/").pop() || ""] ? (
                    <div className="truncate text-[11px] text-neutral-400">
                      {fileHints[selected.split("/").pop() || ""]}
                    </div>
                  ) : null}
                </div>
                {!editing ? (
                  <>
                    <button
                      type="button"
                      className="rounded border border-neutral-200 px-2.5 py-1 text-xs hover:bg-neutral-50"
                      onClick={() => {
                        setDraft(content);
                        setEditing(true);
                        setStatus("");
                      }}
                    >
                      Edit
                    </button>
                    <button
                      type="button"
                      className="rounded border border-neutral-200 px-2.5 py-1 text-xs text-neutral-500 hover:border-red-200 hover:text-red-700"
                      onClick={() => void removeFile()}
                    >
                      Delete
                    </button>
                  </>
                ) : (
                  <>
                    <button
                      type="button"
                      disabled={saving}
                      className="rounded bg-neutral-900 px-2.5 py-1 text-xs text-white disabled:opacity-50"
                      onClick={() => void save()}
                    >
                      {saving ? "Saving…" : "Save"}
                    </button>
                    <button
                      type="button"
                      className="rounded border border-neutral-200 px-2.5 py-1 text-xs hover:bg-neutral-50"
                      onClick={() => {
                        setDraft(content);
                        setEditing(false);
                        setStatus("");
                      }}
                    >
                      Cancel
                    </button>
                  </>
                )}
              </div>
              {error && (
                <p className="border-b border-red-100 bg-red-50 px-4 py-1.5 text-xs text-red-700">
                  {error}
                </p>
              )}
              {status && (
                <p className="border-b border-neutral-100 px-4 py-1.5 text-xs text-neutral-500">
                  {status}
                </p>
              )}
              <div className="min-h-0 flex-1 overflow-auto p-4">
                {editing ? (
                  <textarea
                    className="h-full min-h-[24rem] w-full resize-none rounded-lg border border-neutral-200 bg-neutral-50 p-3 font-mono text-[12px] leading-relaxed text-neutral-800 outline-none focus:border-neutral-400"
                    value={draft}
                    onChange={(e) => setDraft(e.target.value)}
                  />
                ) : (
                  <FileContentView path={selected} content={content} />
                )}
              </div>
            </>
          ) : (
            <div className="grid flex-1 place-items-center text-sm text-neutral-400">
              Select a file
            </div>
          )}
        </section>
      </div>
    </div>
  );
}
