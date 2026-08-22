"use client";

import { useCallback, useEffect, useState } from "react";
import { Check, FolderGit2, TriangleAlert } from "lucide-react";
import FeaturePageHeader from "@/components/shell/feature-page-header";
import { useProject } from "@/components/shell/project-context";
import {
  useTheme,
  type ThemePreference,
} from "@/components/shell/theme-provider";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";

const THEME_OPTIONS: { value: ThemePreference; label: string }[] = [
  { value: "light", label: "Light" },
  { value: "dark", label: "Dark" },
  { value: "system", label: "System" },
];

export default function SettingsPage() {
  const { projects, projectId, setProjectId, refreshProjects } = useProject();
  const { preference, setPreference } = useTheme();
  const [error, setError] = useState("");
  const [status, setStatus] = useState("");
  const [busyId, setBusyId] = useState("");
  const [sessionHours, setSessionHours] = useState("168");
  const [memoryHours, setMemoryHours] = useState("168");
  const [retentionLoaded, setRetentionLoaded] = useState(false);
  const [removeTarget, setRemoveTarget] = useState<{
    id: string;
    label: string;
  } | null>(null);
  const [confirmName, setConfirmName] = useState("");

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void (async () => {
        try {
          const r = await fetch("/api/settings/retention");
          const body = (await r.json()) as {
            session_hours?: number;
            memory_hours?: number;
          };
          if (!r.ok) return;
          setSessionHours(String(body.session_hours ?? 168));
          setMemoryHours(String(body.memory_hours ?? 168));
          setRetentionLoaded(true);
        } catch {
          setRetentionLoaded(true);
        }
      })();
    }, 0);
    return () => window.clearTimeout(timer);
  }, []);

  const saveRetention = useCallback(async (apply: boolean) => {
    setBusyId("retention");
    setError("");
    setStatus("");
    try {
      const r = await fetch("/api/settings/retention", {
        method: "PUT",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          session_hours: Number(sessionHours),
          memory_hours: Number(memoryHours),
          apply,
        }),
      });
      const body = (await r.json().catch(() => ({}))) as {
        error?: string;
        session_hours?: number;
        memory_hours?: number;
        sessions_deleted?: string[];
        memories_deleted?: number;
      };
      if (!r.ok) throw new Error(body.error || "save failed");
      setSessionHours(String(body.session_hours ?? sessionHours));
      setMemoryHours(String(body.memory_hours ?? memoryHours));
      if (apply) {
        const sessions = Array.isArray(body.sessions_deleted)
          ? body.sessions_deleted.length
          : 0;
        const memories = Number(body.memories_deleted ?? 0);
        setStatus(
          sessions === 0 && memories === 0
            ? "Saved. Nothing was old enough to delete."
            : `Saved. Deleted ${sessions} session(s) and ${memories} memor${memories === 1 ? "y" : "ies"}.`,
        );
      } else {
        setStatus("Saved. New sessions and memories will follow these hours.");
      }
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e));
    } finally {
      setBusyId("");
    }
  }, [sessionHours, memoryHours]);

  const removeProject = useCallback(async () => {
    if (!removeTarget || confirmName.trim() !== removeTarget.label) return;
    const { id, label } = removeTarget;
    setBusyId(id);
    setError("");
    setStatus("");
    try {
      const qs = new URLSearchParams({ id, purge: "1" });
      const r = await fetch(`/api/projects?${qs}`, { method: "DELETE" });
      const body = await r.json().catch(() => ({}));
      if (!r.ok) throw new Error(body.error || "remove failed");
      if (projectId === id) setProjectId("");
      setRemoveTarget(null);
      setConfirmName("");
      await refreshProjects();
      setStatus(`Removed ${label} and deleted its .so directory`);
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e));
    } finally {
      setBusyId("");
    }
  }, [
    removeTarget,
    confirmName,
    projectId,
    setProjectId,
    refreshProjects,
  ]);

  const pruneInvalid = useCallback(async () => {
    setBusyId("prune");
    setError("");
    setStatus("");
    try {
      const qs = new URLSearchParams({ prune: "1" });
      const r = await fetch(`/api/projects?${qs}`, { method: "DELETE" });
      const body = await r.json().catch(() => ({}));
      if (!r.ok) throw new Error(body.error || "prune failed");
      const pruned = Number(body.pruned ?? 0);
      if (pruned > 0 && body.results) {
        const removedIds = new Set(
          (body.results as { project?: { id?: string } }[])
            .map((item) => item.project?.id)
            .filter(Boolean),
        );
        if (removedIds.has(projectId)) setProjectId("");
      }
      await refreshProjects();
      setStatus(
        pruned === 0
          ? "Nothing to clean up"
          : `Removed ${pruned} invalid project(s)`,
      );
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e));
    } finally {
      setBusyId("");
    }
  }, [projectId, setProjectId, refreshProjects]);

  return (
    <div className="flex h-full min-h-0 flex-col">
      <FeaturePageHeader title="Settings" />
      <div className="min-h-0 flex-1 overflow-auto p-5">
        <div className="max-w-2xl space-y-4 text-sm">
          {error ? (
            <p className="rounded border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700">
              {error}
            </p>
          ) : null}
          {status ? (
            <p className="rounded border border-neutral-200 bg-neutral-50 px-3 py-2 text-xs text-neutral-600">
              {status}
            </p>
          ) : null}

          <section className="rounded border border-neutral-200 p-4">
            <h2 className="font-medium text-neutral-900">Appearance</h2>
            <p className="mt-1 text-xs text-neutral-500">
              Theme for the Superopen UI. System follows your OS preference. The
              session map always renders on its night stage.
            </p>
            <div className="mt-3 inline-flex flex-wrap gap-1 rounded-md border border-neutral-200 p-0.5">
              {THEME_OPTIONS.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  className={cn(
                    "rounded px-3 py-1.5 text-xs font-medium transition-colors",
                    preference === option.value
                      ? "bg-neutral-900 text-white"
                      : "text-neutral-600 hover:bg-neutral-50"
                  )}
                  onClick={() => setPreference(option.value)}
                >
                  {option.label}
                </button>
              ))}
            </div>
          </section>

          <section className="rounded border border-neutral-200 p-4">
            <h2 className="font-medium text-neutral-900">Retention</h2>
            <p className="mt-1 text-xs text-neutral-500">
              Auto-delete old session transcripts and unpinned memories.
              Hours, default 168 (7 days). 0 keeps that store forever.
              Teachings, pins, and the code graph are never deleted by age.
              Checkpoints live inside session folders.
            </p>
            <div className="mt-3 grid gap-3 sm:grid-cols-2">
              <label className="block text-xs text-neutral-600">
                Sessions (hours)
                <input
                  type="number"
                  min={0}
                  step={1}
                  disabled={!retentionLoaded || busyId === "retention"}
                  value={sessionHours}
                  onChange={(event) => setSessionHours(event.target.value)}
                  className="mt-1.5 w-full rounded border border-neutral-300 px-2 py-1.5 font-mono text-xs outline-none focus:border-neutral-500"
                />
              </label>
              <label className="block text-xs text-neutral-600">
                Memories (hours)
                <input
                  type="number"
                  min={0}
                  step={1}
                  disabled={!retentionLoaded || busyId === "retention"}
                  value={memoryHours}
                  onChange={(event) => setMemoryHours(event.target.value)}
                  className="mt-1.5 w-full rounded border border-neutral-300 px-2 py-1.5 font-mono text-xs outline-none focus:border-neutral-500"
                />
              </label>
            </div>
            <div className="mt-3 flex flex-wrap gap-2">
              <button
                type="button"
                disabled={busyId === "retention"}
                onClick={() => void saveRetention(false)}
                className="rounded border border-neutral-200 px-2.5 py-1.5 text-xs font-medium text-neutral-700 hover:bg-neutral-50 disabled:opacity-50"
              >
                Save
              </button>
              <button
                type="button"
                disabled={busyId === "retention"}
                onClick={() => void saveRetention(true)}
                className="rounded border border-neutral-200 px-2.5 py-1.5 text-xs font-medium text-neutral-700 hover:bg-neutral-50 disabled:opacity-50"
              >
                Save and delete now
              </button>
            </div>
          </section>

          <section className="rounded border border-neutral-200 p-4">
            <div className="flex flex-wrap items-start justify-between gap-2">
              <div className="min-w-0">
                <h2 className="font-medium text-neutral-900">Projects</h2>
                <p className="mt-1 text-xs text-neutral-500">
                  Repositories this UI can read. Selecting one scopes the
                  sessions list, the graph and the session map to it.
                </p>
              </div>
              <button
                type="button"
                disabled={busyId === "prune"}
                onClick={pruneInvalid}
                className="shrink-0 rounded border border-neutral-200 px-2.5 py-1.5 text-xs font-medium text-neutral-700 hover:bg-neutral-50 disabled:opacity-50"
              >
                Clean up
              </button>
            </div>

            <ul className="mt-3 divide-y divide-neutral-200 border-t border-neutral-200">
              {projects.map((project) => {
                const selected = projectId === project.id;
                const label = project.slug || project.name || project.id;
                return (
                  <li
                    key={project.id}
                    className="flex items-center gap-3 py-2.5"
                  >
                    <FolderGit2 className="size-4 shrink-0 text-neutral-400" />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-1.5">
                        <span className="truncate font-medium text-neutral-900">
                          {label}
                        </span>
                        {selected ? (
                          <Check className="size-3.5 shrink-0 text-neutral-900" />
                        ) : null}
                        {project.missing ? (
                          <span className="inline-flex shrink-0 items-center gap-1 text-[11px] text-amber-700">
                            <TriangleAlert className="size-3" />
                            missing
                          </span>
                        ) : null}
                      </div>
                      <p className="truncate font-mono text-[11px] text-neutral-500">
                        {project.repo_root}
                      </p>
                    </div>
                    <div className="flex shrink-0 items-center gap-2">
                      {!selected ? (
                        <button
                          type="button"
                          onClick={() => setProjectId(project.id)}
                          className="rounded border border-neutral-200 px-2.5 py-1 text-xs font-medium text-neutral-700 hover:bg-neutral-50"
                        >
                          Select
                        </button>
                      ) : null}
                      <button
                        type="button"
                        disabled={busyId === project.id}
                        onClick={() => {
                          setConfirmName("");
                          setRemoveTarget({ id: project.id, label });
                        }}
                        className="rounded border border-neutral-200 px-2.5 py-1 text-xs font-medium text-red-700 hover:bg-red-50 disabled:opacity-50"
                      >
                        Remove
                      </button>
                    </div>
                  </li>
                );
              })}
            </ul>
          </section>
        </div>
      </div>

      <Dialog
        open={removeTarget != null}
        onOpenChange={(open) => {
          if (!open) {
            setRemoveTarget(null);
            setConfirmName("");
          }
        }}
      >
        <DialogContent className="max-w-md">
          <DialogTitle>Remove {removeTarget?.label}</DialogTitle>
          <DialogDescription>
            This unregisters the project and permanently deletes its{" "}
            <code className="font-mono">.so</code> directory, including recorded
            sessions and the code graph. The repository itself is untouched.
          </DialogDescription>
          <label className="mt-3 block text-xs text-neutral-600">
            Type <span className="font-mono">{removeTarget?.label}</span> to
            confirm
            <input
              value={confirmName}
              onChange={(e) => setConfirmName(e.target.value)}
              className="mt-1.5 w-full rounded border border-neutral-300 px-2 py-1.5 font-mono text-xs outline-none focus:border-neutral-500"
              autoComplete="off"
            />
          </label>
          <div className="mt-4 flex justify-end gap-2">
            <button
              type="button"
              onClick={() => {
                setRemoveTarget(null);
                setConfirmName("");
              }}
              className="rounded border border-neutral-200 px-3 py-1.5 text-xs font-medium text-neutral-700 hover:bg-neutral-50"
            >
              Cancel
            </button>
            <button
              type="button"
              disabled={confirmName.trim() !== removeTarget?.label}
              onClick={removeProject}
              className="rounded bg-red-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-red-700 disabled:opacity-40"
            >
              Remove project
            </button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
