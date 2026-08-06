"use client";

import { useCallback, useEffect, useState } from "react";
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

type SafeConfig = {
  memory?: { enabled?: boolean; idle_harvest_hours?: number; backend?: string };
  guardrails?: { enabled?: boolean };
  graph?: { code?: boolean; semantic?: boolean };
  recommendations?: { auto?: boolean; require_approval?: boolean };
  retention?: { days?: number };
  evals?: {
    on_session_end?: boolean;
    auto?: boolean;
    backend?: string;
    model_claude?: string;
    model_codex?: string;
  };
  advanced_llm?: {
    provider?: string;
    model?: string;
    api_key_env?: string;
    base_url?: string;
  };
};

const BACKENDS = ["auto", "agent_cli", "llm_api", "heuristics"] as const;

const THEME_OPTIONS: { value: ThemePreference; label: string }[] = [
  { value: "light", label: "Light" },
  { value: "dark", label: "Dark" },
  { value: "system", label: "System" },
];

export default function SettingsPage() {
  const { projects, projectId, setProjectId, refreshProjects } = useProject();
  const { preference, setPreference } = useTheme();
  const [config, setConfig] = useState<SafeConfig | null>(null);
  const [error, setError] = useState("");
  const [status, setStatus] = useState("");
  const [saving, setSaving] = useState(false);
  const [busyId, setBusyId] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<{
    id: string;
    label: string;
  } | null>(null);
  const [deleteConfirmName, setDeleteConfirmName] = useState("");

  const load = useCallback(async () => {
    try {
      const cfg = await fetch("/api/config").then(async (r) => {
        if (!r.ok) throw new Error(await r.text());
        return r.json();
      });
      setConfig(cfg);
      setError("");
    } catch (e: any) {
      setError(String(e.message || e));
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  async function patch(body: Record<string, unknown>) {
    setSaving(true);
    setStatus("");
    setError("");
    try {
      const r = await fetch("/api/config", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      if (!r.ok) throw new Error(await r.text());
      const j = await r.json();
      if (j.config) setConfig(j.config);
      setStatus("Saved to .so/config.yaml - CLI picks this up on the next command");
    } catch (e: any) {
      setError(String(e.message || e));
    } finally {
      setSaving(false);
    }
  }

  function openDeleteDialog(id: string) {
    const p = projects.find((x) => x.id === id);
    const label = p?.slug || p?.name || id;
    setDeleteConfirmName("");
    setDeleteTarget({ id, label });
  }

  async function confirmDeleteProject() {
    if (!deleteTarget) return;
    if (deleteConfirmName.trim() !== deleteTarget.label) return;

    const { id, label } = deleteTarget;
    setBusyId(id);
    setError("");
    setStatus("");
    try {
      const qs = new URLSearchParams({ id, purge: "1" });
      const r = await fetch(`/api/projects?${qs}`, { method: "DELETE" });
      const j = await r.json().catch(() => ({}));
      if (!r.ok) throw new Error(j.error || "remove failed");
      if (projectId === id) setProjectId("");
      setDeleteTarget(null);
      setDeleteConfirmName("");
      await refreshProjects();
      setStatus(`Deleted ${label} and its .so directory`);
    } catch (e: any) {
      setError(String(e.message || e));
    } finally {
      setBusyId("");
    }
  }

  async function pruneMissing() {
    const missing = projects.filter((p) => p.missing);
    if (missing.length === 0) {
      setStatus("No missing projects");
      return;
    }
    if (
      !window.confirm(
        `Unregister ${missing.length} missing project(s) and permanently delete any leftover .so data?`
      )
    ) {
      return;
    }

    setBusyId("prune");
    setError("");
    setStatus("");
    try {
      const qs = new URLSearchParams({ prune: "1", purge: "1" });
      const r = await fetch(`/api/projects?${qs}`, { method: "DELETE" });
      const j = await r.json().catch(() => ({}));
      if (!r.ok) throw new Error(j.error || "prune failed");
      if (missing.some((p) => p.id === projectId)) setProjectId("");
      await refreshProjects();
      setStatus(`Pruned ${j.pruned ?? missing.length} missing project(s)`);
    } catch (e: any) {
      setError(String(e.message || e));
    } finally {
      setBusyId("");
    }
  }

  const llm = config?.advanced_llm;
  const missingCount = projects.filter(
    (p) => p.missing && p.id !== "local"
  ).length;

  return (
    <div className="flex h-full min-h-0 flex-col">
      <FeaturePageHeader title="Settings" />
      <div className="min-h-0 flex-1 overflow-auto p-4">
        {error && <p className="mt-3 text-sm text-red-600">{error}</p>}
        {status && <p className="mt-3 text-sm text-neutral-500">{status}</p>}
        <div className="mt-4 max-w-2xl space-y-4 text-sm">
          <section className="rounded border border-neutral-200 p-4">
            <h3 className="font-medium">Appearance</h3>
            <p className="mt-1 text-xs text-neutral-500">
              Theme for the Superopen UI. System follows your OS preference.
            </p>
            <div className="mt-3 flex flex-wrap gap-1 rounded-md border border-neutral-200 p-0.5">
              {THEME_OPTIONS.map((opt) => (
                <button
                  key={opt.value}
                  type="button"
                  className={cn(
                    "rounded px-3 py-1.5 text-xs font-medium transition-colors",
                    preference === opt.value
                      ? "bg-neutral-900 text-white"
                      : "text-neutral-600 hover:bg-neutral-50"
                  )}
                  onClick={() => setPreference(opt.value)}
                >
                  {opt.label}
                </button>
              ))}
            </div>
          </section>

          <section className="rounded border border-neutral-200 p-4">
            <div className="flex flex-wrap items-start justify-between gap-2">
              <div>
                <h3 className="font-medium">Projects</h3>
                <p className="mt-1 text-xs text-neutral-500">
                  Registry:{" "}
                  <code className="rounded bg-neutral-100 px-1">
                    ~/.config/superopen/projects.json
                  </code>
                  . Delete unregisters the project and permanently wipes its{" "}
                  <code className="rounded bg-neutral-100 px-1">.so</code> data
                  (sessions, traces, graph). CLI:{" "}
                  <code className="rounded bg-neutral-100 px-1">
                    so projects remove &lt;id&gt; --purge
                  </code>
                  {" · "}
                  <code className="rounded bg-neutral-100 px-1">so projects prune --purge</code>
                  .
                </p>
              </div>
              {missingCount > 0 && (
                <button
                  type="button"
                  disabled={busyId === "prune"}
                  className="rounded border border-red-200 px-2 py-1 text-[11px] text-red-700 hover:bg-red-50 disabled:opacity-50"
                  onClick={() => void pruneMissing()}
                >
                  Prune missing
                </button>
              )}
            </div>
            <ul className="mt-3 divide-y divide-neutral-100 rounded border border-neutral-100">
              {projects.filter((p) => p.id !== "local").length === 0 && (
                <li className="px-3 py-2 text-xs text-neutral-500">
                  No registered projects - run{" "}
                  <code className="rounded bg-neutral-100 px-1">so projects add</code>
                  {" "}or <code className="rounded bg-neutral-100 px-1">so init</code>
                </li>
              )}
              {projects
                .filter((p) => p.id !== "local")
                .map((p) => (
                <li
                  key={p.id}
                  className="flex flex-wrap items-center justify-between gap-2 px-3 py-2"
                >
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-1.5 text-xs">
                      <span className="font-medium text-neutral-900">
                        {p.slug || p.name}
                      </span>
                      {p.missing && (
                        <span className="rounded bg-amber-50 px-1.5 py-0.5 text-[10px] font-medium text-amber-800">
                          missing
                        </span>
                      )}
                    </div>
                    <div className="truncate font-mono text-[11px] text-neutral-400">
                      {p.repo_root}
                    </div>
                  </div>
                  <button
                    type="button"
                    disabled={Boolean(busyId)}
                    className="rounded border border-red-200 px-2 py-1 text-[11px] text-red-700 hover:bg-red-50 disabled:opacity-50"
                    onClick={() => openDeleteDialog(p.id)}
                  >
                    Delete
                  </button>
                </li>
              ))}
            </ul>
          </section>

          <Dialog
            open={deleteTarget != null}
            onOpenChange={(open) => {
              if (!open) {
                setDeleteTarget(null);
                setDeleteConfirmName("");
              }
            }}
          >
            <DialogContent className="max-w-md">
              <DialogTitle>Delete project</DialogTitle>
              <DialogDescription>
                This permanently removes{" "}
                <span className="font-medium text-neutral-800">
                  {deleteTarget?.label}
                </span>{" "}
                from Superopen and deletes its{" "}
                <code className="rounded bg-neutral-100 px-1">.so</code> data
                (sessions, traces, graph). Type the project name to confirm.
              </DialogDescription>
              <label className="mt-1 block text-xs text-neutral-600">
                Type{" "}
                <code className="rounded bg-neutral-100 px-1 font-medium text-neutral-900">
                  {deleteTarget?.label}
                </code>{" "}
                to confirm
                <input
                  type="text"
                  autoFocus
                  autoComplete="off"
                  spellCheck={false}
                  className="mt-1.5 w-full rounded border border-neutral-200 px-2.5 py-1.5 text-sm text-neutral-900 outline-none focus:border-neutral-400"
                  value={deleteConfirmName}
                  onChange={(e) => setDeleteConfirmName(e.target.value)}
                  onKeyDown={(e) => {
                    if (
                      e.key === "Enter" &&
                      deleteConfirmName.trim() === deleteTarget?.label
                    ) {
                      e.preventDefault();
                      void confirmDeleteProject();
                    }
                  }}
                />
              </label>
              <div className="mt-2 flex justify-end gap-2">
                <button
                  type="button"
                  className="rounded border border-neutral-200 px-3 py-1.5 text-xs text-neutral-700 hover:bg-neutral-50"
                  onClick={() => {
                    setDeleteTarget(null);
                    setDeleteConfirmName("");
                  }}
                >
                  Cancel
                </button>
                <button
                  type="button"
                  disabled={
                    Boolean(busyId) ||
                    deleteConfirmName.trim() !== deleteTarget?.label
                  }
                  className="rounded bg-red-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-red-700 disabled:opacity-40"
                  onClick={() => void confirmDeleteProject()}
                >
                  {busyId === deleteTarget?.id ? "Deleting…" : "Delete forever"}
                </button>
              </div>
            </DialogContent>
          </Dialog>

          {config != null && (
            <>
            <p className="text-xs text-neutral-500">
              Config writes{" "}
              <code className="rounded bg-neutral-100 px-1">.so/config.yaml</code>
              - the same file <code className="rounded bg-neutral-100 px-1">so</code> reads.
            </p>

            <section className="rounded border border-neutral-200 p-4">
              <h3 className="font-medium">Memory</h3>
              <p className="mt-1 text-xs text-neutral-500">
                When enabled, SessionStart injects Active Context.
              </p>
              <label className="mt-3 flex items-center gap-2 text-xs">
                <input
                  type="checkbox"
                  checked={config.memory?.enabled ?? true}
                  disabled={saving}
                  onChange={(e) =>
                    void patch({ memory: { enabled: e.target.checked } })
                  }
                />
                memory.enabled
              </label>
              <dl className="mt-3 grid grid-cols-[10rem_1fr] items-center gap-2 text-xs">
                <dt className="text-neutral-500">idle_harvest_hours</dt>
                <dd>
                  <input
                    type="number"
                    min={1}
                    max={168}
                    className="w-20 rounded border border-neutral-200 px-1.5 py-0.5"
                    value={config.memory?.idle_harvest_hours ?? 6}
                    disabled={saving}
                    onChange={(e) => {
                      const n = parseInt(e.target.value, 10);
                      if (!Number.isFinite(n) || n < 1) return;
                      void patch({ memory: { idle_harvest_hours: n } });
                    }}
                  />
                </dd>
                <dt className="text-neutral-500">backend</dt>
                <dd>
                  <select
                    className="rounded border border-neutral-200 bg-white px-1.5 py-0.5"
                    value={config.memory?.backend || "auto"}
                    disabled={saving}
                    onChange={(e) =>
                      void patch({ memory: { backend: e.target.value } })
                    }
                  >
                    {BACKENDS.map((b) => (
                      <option key={b} value={b}>
                        {b}
                      </option>
                    ))}
                  </select>
                </dd>
              </dl>
            </section>

            <section className="rounded border border-neutral-200 p-4">
              <h3 className="font-medium">Retention</h3>
              <p className="mt-1 text-xs text-neutral-500">
                Empty sessions are dropped. Everything else older than this many days is
                pruned (sessions, eval history, recommendations, traces).
              </p>
              <label className="mt-3 flex items-center gap-2 text-xs">
                retention.days
                <input
                  type="number"
                  min={1}
                  max={365}
                  className="w-16 rounded border border-neutral-200 px-1.5 py-0.5"
                  value={config.retention?.days ?? 7}
                  disabled={saving}
                  onChange={(e) => {
                    const n = parseInt(e.target.value, 10);
                    if (!Number.isFinite(n) || n < 1) return;
                    void patch({ retention: { days: n } });
                  }}
                />
              </label>
            </section>

            <section className="rounded border border-neutral-200 p-4">
              <h3 className="font-medium">Harness</h3>
              <div className="mt-3 space-y-2 text-xs">
                <label className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    checked={config.guardrails?.enabled ?? true}
                    disabled={saving}
                    onChange={(e) =>
                      void patch({ guardrails: { enabled: e.target.checked } })
                    }
                  />
                  guardrails.enabled
                </label>
                <label className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    checked={config.evals?.on_session_end ?? true}
                    disabled={saving}
                    onChange={(e) =>
                      void patch({ evals: { on_session_end: e.target.checked } })
                    }
                  />
                  evaluations.on_session_end (finalize runs eval)
                </label>
                <label className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    checked={config.evals?.auto ?? true}
                    disabled={saving}
                    onChange={(e) =>
                      void patch({ evals: { auto: e.target.checked } })
                    }
                  />
                  evaluations.auto (also used on finalize)
                </label>
                <label className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    checked={config.recommendations?.auto ?? true}
                    disabled={saving}
                    onChange={(e) =>
                      void patch({ recommendations: { auto: e.target.checked } })
                    }
                  />
                  recommendations.auto (generate after scored eval)
                </label>
                <label className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    checked={config.recommendations?.require_approval ?? true}
                    disabled={saving}
                    onChange={(e) =>
                      void patch({
                        recommendations: { require_approval: e.target.checked },
                      })
                    }
                  />
                  recommendations.require_approval
                </label>
                <label className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    checked={config.graph?.code ?? true}
                    disabled={saving}
                    onChange={(e) =>
                      void patch({ graph: { code: e.target.checked } })
                    }
                  />
                  graph.code
                </label>
                <label className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    checked={config.graph?.semantic ?? true}
                    disabled={saving}
                    onChange={(e) =>
                      void patch({ graph: { semantic: e.target.checked } })
                    }
                  />
                  graph.semantic
                </label>
              </div>
              <dl className="mt-3 grid grid-cols-[10rem_1fr] items-center gap-2 text-xs">
                <dt className="text-neutral-500">evals.backend</dt>
                <dd>
                  <select
                    className="rounded border border-neutral-200 bg-white px-1.5 py-0.5"
                    value={config.evals?.backend || "auto"}
                    disabled={saving}
                    onChange={(e) =>
                      void patch({ evals: { backend: e.target.value } })
                    }
                  >
                    {BACKENDS.map((b) => (
                      <option key={b} value={b}>
                        {b}
                      </option>
                    ))}
                  </select>
                </dd>
                <dt className="text-neutral-500">evals.models.claude</dt>
                <dd>
                  <input
                    className="w-full max-w-md rounded border border-neutral-200 px-1.5 py-0.5 font-mono"
                    value={config.evals?.model_claude || "claude-sonnet-5"}
                    disabled={saving}
                    onBlur={(e) => {
                      const v = e.target.value.trim();
                      if (!v || v === config.evals?.model_claude) return;
                      void patch({ evals: { model_claude: v } });
                    }}
                    onChange={(e) =>
                      setConfig({
                        ...config,
                        evals: { ...config.evals, model_claude: e.target.value },
                      })
                    }
                  />
                </dd>
                <dt className="text-neutral-500">evals.models.codex</dt>
                <dd>
                  <input
                    className="w-full max-w-md rounded border border-neutral-200 px-1.5 py-0.5 font-mono"
                    value={config.evals?.model_codex || "gpt-5.6-luna"}
                    disabled={saving}
                    onBlur={(e) => {
                      const v = e.target.value.trim();
                      if (!v || v === config.evals?.model_codex) return;
                      void patch({ evals: { model_codex: v } });
                    }}
                    onChange={(e) =>
                      setConfig({
                        ...config,
                        evals: { ...config.evals, model_codex: e.target.value },
                      })
                    }
                  />
                </dd>
              </dl>
              <p className="mt-2 text-[11px] text-neutral-500">
                Backend AI uses Claude/Codex CLI on this machine (even for Cursor/OpenCode/Pi/Gemini sessions).
              </p>
            </section>

            <section className="rounded border border-neutral-200 p-4">
              <h3 className="font-medium">Headless LLM</h3>
              <p className="mt-1 text-xs text-neutral-500">
                Optional fallback for CI/servers without a coding-agent login. Prefer Claude/Codex/Cursor CLI when available. Maps to{" "}
                <code className="rounded bg-neutral-100 px-1">llm:</code> in config.
              </p>
              <dl className="mt-3 grid grid-cols-[10rem_1fr] items-center gap-2 text-xs">
                {(
                  [
                    ["provider", "provider"],
                    ["model", "model"],
                    ["api_key_env", "api_key_env"],
                    ["base_url", "base_url"],
                  ] as const
                ).map(([label, key]) => (
                  <FieldRow
                    key={key}
                    label={label}
                    value={llm?.[key] || ""}
                    disabled={saving}
                    placeholder={key === "base_url" ? "optional" : undefined}
                    onLocal={(v) =>
                      setConfig({
                        ...config,
                        advanced_llm: { ...config.advanced_llm, [key]: v },
                      })
                    }
                    onCommit={(v) => void patch({ llm: { [key]: v } })}
                  />
                ))}
              </dl>
            </section>
            </>
          )}
        </div>
      </div>
    </div>
  );
}

function FieldRow({
  label,
  value,
  disabled,
  placeholder,
  onLocal,
  onCommit,
}: {
  label: string;
  value: string;
  disabled?: boolean;
  placeholder?: string;
  onLocal: (v: string) => void;
  onCommit: (v: string) => void;
}) {
  return (
    <>
      <dt className="text-neutral-500">{label}</dt>
      <dd>
        <input
          className="w-full max-w-md rounded border border-neutral-200 px-1.5 py-0.5 font-mono"
          value={value}
          disabled={disabled}
          placeholder={placeholder}
          onChange={(e) => onLocal(e.target.value)}
          onBlur={(e) => onCommit(e.target.value.trim())}
        />
      </dd>
    </>
  );
}
