"use client";

import { useCallback, useEffect, useState } from "react";
import FeaturePageHeader from "@/components/shell/feature-page-header";
import { FileContentView } from "@/components/file-content-view";
import { Dropdown } from "@/components/ui/dropdown";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  appendDeniedTool,
  appendDeniedCommand,
  appendEvaluationAgentRule,
  appendEvaluationCheck,
  appendGuardrailRule,
  appendSensitivePath,
  defaultEvaluationsYaml,
  defaultGuardrailsYaml,
  slugifyId,
  type GuardrailCreateKind,
  type GuardrailSeverity,
} from "@/lib/so/harness-yaml";
import { cn } from "@/lib/utils";

type Kind = "guardrails" | "evaluations";

const CONFIG: Record<
  Kind,
  {
    title: string;
    path: string;
    emptyTitle: string;
    emptyBody: string;
    seedLabel: string;
    defaultYaml: () => string;
  }
> = {
  guardrails: {
    title: "Guardrails",
    path: "guardrails/guardrails.yaml",
    emptyTitle: "No guardrails yet",
    emptyBody:
      "Add denied commands and sensitive paths (hard stops in coding hooks), or advisory rules (soft guidance). Hooks already load this file - no reinstall after edits.",
    seedLabel: "Set up guardrails",
    defaultYaml: defaultGuardrailsYaml,
  },
  evaluations: {
    title: "Evaluations",
    path: "evals/configs.yaml",
    emptyTitle: "No evaluations yet",
    emptyBody:
      "Evaluations define what good sessions look like - automated checks plus agent guidance for judges.",
    seedLabel: "Set up evaluations",
    defaultYaml: defaultEvaluationsYaml,
  },
};

export function HarnessSingleDocPage({
  kind,
  embedded = false,
}: {
  kind: Kind;
  /** When true, omit FeaturePageHeader (parent page owns chrome/tabs). */
  embedded?: boolean;
}) {
  const cfg = CONFIG[kind];
  const [content, setContent] = useState("");
  const [exists, setExists] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [status, setStatus] = useState("");
  const [composerOpen, setComposerOpen] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const r = await fetch(`/api/files/${cfg.path}`);
      if (r.status === 404) {
        setExists(false);
        setContent("");
        return;
      }
      if (!r.ok) throw new Error(await r.text());
      const text = await r.text();
      setExists(true);
      setContent(text);
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e));
    } finally {
      setLoading(false);
    }
  }, [cfg.path]);

  useEffect(() => {
    void load();
  }, [load]);

  async function write(body: string) {
    setSaving(true);
    setError("");
    setStatus("");
    try {
      const r = await fetch(`/api/files/${cfg.path}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ content: body, create: !exists }),
      });
      if (!r.ok) {
        const j = await r.json().catch(() => ({}));
        throw new Error(j.error || (await r.text()));
      }
      setContent(body);
      setExists(true);
      setComposerOpen(false);
      setStatus("Saved to .so/ - commit to share with teammates.");
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e));
    } finally {
      setSaving(false);
    }
  }

  async function seed() {
    await write(cfg.defaultYaml());
  }

  async function applyMutation(next: string) {
    await write(next);
  }

  const addButton = (
    <button
      type="button"
      className="ml-auto rounded-md bg-neutral-900 px-2.5 py-1.5 text-xs font-medium text-white hover:bg-neutral-800"
      onClick={() => {
        setComposerOpen(true);
        setStatus("");
      }}
    >
      {kind === "guardrails" ? "Add guardrail" : "Add"}
    </button>
  );

  return (
    <div className={cn("flex h-full min-h-0 flex-col", embedded && "min-h-0 flex-1")}>
      {!embedded && <FeaturePageHeader title={cfg.title} />}
      <div className="flex min-h-0 flex-1 flex-col bg-white">
        {loading ? (
          <p className="p-6 text-sm text-neutral-500">Loading…</p>
        ) : !exists ? (
          <EmptySetup
            title={cfg.emptyTitle}
            body={cfg.emptyBody}
            cta={cfg.seedLabel}
            busy={saving}
            onSetup={() => void seed()}
            error={error}
          />
        ) : (
          <>
            {error && (
              <p className="border-b border-red-100 bg-red-50 px-5 py-1.5 text-xs text-red-700">
                {error}
              </p>
            )}
            {status && (
              <p className="border-b border-neutral-100 px-5 py-1.5 text-xs text-neutral-500">
                {status}
              </p>
            )}
            <Dialog open={composerOpen} onOpenChange={setComposerOpen}>
              <DialogContent className="max-w-xl">
                <DialogTitle>
                  {kind === "guardrails" ? "Add guardrail" : "Add evaluator"}
                </DialogTitle>
                <DialogDescription>
                  {kind === "guardrails"
                    ? "Hard stops run in coding hooks; advisory rules guide agents."
                    : "Checks and guidance used when scoring sessions."}
                </DialogDescription>
                {kind === "guardrails" ? (
                  <GuardrailComposer
                    busy={saving}
                    onCancel={() => setComposerOpen(false)}
                    onAddDeniedTool={(pattern) =>
                      void applyMutation(appendDeniedTool(content, pattern))
                    }
                    onAddDenyCommand={(pattern) =>
                      void applyMutation(appendDeniedCommand(content, pattern))
                    }
                    onAddSensitivePath={(pattern) =>
                      void applyMutation(appendSensitivePath(content, pattern))
                    }
                    onAddAdvisory={(rule) =>
                      void applyMutation(appendGuardrailRule(content, rule))
                    }
                  />
                ) : (
                  <EvaluationComposer
                    busy={saving}
                    onCancel={() => setComposerOpen(false)}
                    onAddCheck={(c) =>
                      void applyMutation(appendEvaluationCheck(content, c))
                    }
                    onAddRule={(r) =>
                      void applyMutation(appendEvaluationAgentRule(content, r))
                    }
                  />
                )}
              </DialogContent>
            </Dialog>
            <div className="flex min-h-0 flex-1 flex-col overflow-auto p-0">
              <FileContentView
                path={cfg.path}
                content={content}
                domain={kind}
                toolbarAction={addButton}
              />
            </div>
          </>
        )}
      </div>
    </div>
  );
}

function EmptySetup({
  title,
  body,
  cta,
  busy,
  onSetup,
  error,
}: {
  title: string;
  body: string;
  cta: string;
  busy: boolean;
  onSetup: () => void;
  error: string;
}) {
  return (
    <div className="flex flex-1 items-center justify-center p-8">
      <div className="max-w-md text-center">
        <h2 className="text-base font-semibold text-neutral-900">{title}</h2>
        <p className="mt-2 text-sm text-neutral-500">{body}</p>
        {error && <p className="mt-3 text-sm text-red-600">{error}</p>}
        <button
          type="button"
          disabled={busy}
          onClick={onSetup}
          className="mt-5 rounded-md bg-neutral-900 px-4 py-2 text-sm text-white hover:bg-neutral-800 disabled:opacity-50"
        >
          {busy ? "Creating…" : cta}
        </button>
      </div>
    </div>
  );
}

function GuardrailComposer({
  busy,
  onCancel,
  onAddDeniedTool,
  onAddDenyCommand,
  onAddSensitivePath,
  onAddAdvisory,
}: {
  busy: boolean;
  onCancel: () => void;
  onAddDeniedTool: (pattern: string) => void;
  onAddDenyCommand: (pattern: string) => void;
  onAddSensitivePath: (pattern: string) => void;
  onAddAdvisory: (rule: {
    id: string;
    description: string;
    severity: GuardrailSeverity;
    source: string;
  }) => void;
}) {
  const [kind, setKind] = useState<GuardrailCreateKind>("deny_command");
  const [pattern, setPattern] = useState("");
  const [description, setDescription] = useState("");
  const [severity, setSeverity] = useState<GuardrailSeverity>("warn");
  const id = slugifyId(description);

  const canSubmit =
    kind === "advisory" ? Boolean(description.trim()) : Boolean(pattern.trim());

  return (
    <div className="mx-auto max-w-xl space-y-3">
      <p className="text-xs font-medium text-neutral-700">Add a guardrail</p>
      <div className="flex flex-wrap gap-1.5">
        {(
          [
            {
              value: "deny_tool" as const,
              label: "Deny tool",
              hint: "Hook",
            },
            {
              value: "deny_command" as const,
              label: "Deny command",
              hint: "Hook",
            },
            {
              value: "sensitive_path" as const,
              label: "Sensitive path",
              hint: "Hook",
            },
            {
              value: "advisory" as const,
              label: "Advisory rule",
              hint: "Soft",
            },
          ] as const
        ).map((opt) => (
          <button
            key={opt.value}
            type="button"
            onClick={() => setKind(opt.value)}
            className={cn(
              "rounded-md border px-2.5 py-1 text-xs",
              kind === opt.value
                ? "border-neutral-900 bg-neutral-900 text-white"
                : "border-neutral-200 bg-white text-neutral-700 hover:bg-neutral-50"
            )}
          >
            {opt.label}
            <span
              className={cn(
                "ml-1.5 text-[10px]",
                kind === opt.value ? "text-neutral-300" : "text-neutral-400"
              )}
            >
              {opt.hint}
            </span>
          </button>
        ))}
      </div>

      {kind === "deny_tool" && (
        <>
          <label className="block space-y-1">
            <span className="text-[11px] text-neutral-500">
              Vendor tool-name pattern to deny (`*` wildcards supported)
            </span>
            <input
              autoFocus
              className="w-full rounded-md border border-neutral-200 bg-white px-3 py-2 font-mono text-sm outline-none focus:border-neutral-400"
              placeholder="e.g. mcp__production__delete_*"
              value={pattern}
              onChange={(e) => setPattern(e.target.value)}
            />
          </label>
          <p className="text-[11px] text-neutral-400">
            Saved under <span className="font-mono">denied_tools</span> and enforced at the next pre-tool hook.
          </p>
        </>
      )}

      {kind === "deny_command" && (
        <>
          <label className="block space-y-1">
            <span className="text-[11px] text-neutral-500">
              Shell pattern to deny (glob-style, matched by hooks)
            </span>
            <input
              autoFocus
              className="w-full rounded-md border border-neutral-200 bg-white px-3 py-2 font-mono text-sm outline-none focus:border-neutral-400"
              placeholder="e.g. rm -rf / or curl *| bash"
              value={pattern}
              onChange={(e) => setPattern(e.target.value)}
            />
          </label>
          <p className="text-[11px] text-neutral-400">
            Saved under <span className="font-mono">denied_commands</span> -
            enforced on the next tool call (no hook reinstall).
          </p>
        </>
      )}

      {kind === "sensitive_path" && (
        <>
          <label className="block space-y-1">
            <span className="text-[11px] text-neutral-500">
              Path glob agents must not read/write
            </span>
            <input
              autoFocus
              className="w-full rounded-md border border-neutral-200 bg-white px-3 py-2 font-mono text-sm outline-none focus:border-neutral-400"
              placeholder="e.g. **/.env or **/secrets/**"
              value={pattern}
              onChange={(e) => setPattern(e.target.value)}
            />
          </label>
          <p className="text-[11px] text-neutral-400">
            Saved under <span className="font-mono">sensitive_paths</span> -
            enforced on the next tool call (no hook reinstall).
          </p>
        </>
      )}

      {kind === "advisory" && (
        <>
          <label className="block space-y-1">
            <span className="text-[11px] text-neutral-500">
              Soft guidance for agents (not a hard deny)
            </span>
            <textarea
              autoFocus
              rows={2}
              className="w-full rounded-md border border-neutral-200 bg-white px-3 py-2 text-sm outline-none focus:border-neutral-400"
              placeholder="e.g. Prefer so graph query before broad Grep"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </label>
          <div className="flex flex-wrap items-end gap-3">
            <label className="min-w-[8rem] space-y-1">
              <span className="text-[11px] text-neutral-500">Severity label</span>
              <Dropdown
                size="sm"
                value={severity}
                onChange={(v) => setSeverity(v as GuardrailSeverity)}
                options={[
                  { value: "warn", label: "Warn" },
                  { value: "block", label: "Block" },
                  { value: "info", label: "Info" },
                ]}
              />
            </label>
            <p className="flex-1 text-[11px] text-neutral-400">
              id: <span className="font-mono text-neutral-600">{id}</span>
            </p>
          </div>
        </>
      )}

      <div className="flex justify-end gap-1.5">
        <button
          type="button"
          className="rounded px-2.5 py-1 text-xs text-neutral-500 hover:text-neutral-800"
          onClick={onCancel}
        >
          Cancel
        </button>
        <button
          type="button"
          disabled={busy || !canSubmit}
          className="rounded-md bg-neutral-900 px-2.5 py-1 text-xs text-white disabled:opacity-50"
          onClick={() => {
            if (kind === "advisory") {
              onAddAdvisory({
                id,
                description: description.trim(),
                severity,
                source: "ui",
              });
            } else if (kind === "deny_tool") {
              onAddDeniedTool(pattern.trim());
            } else if (kind === "sensitive_path") {
              onAddSensitivePath(pattern.trim());
            } else {
              onAddDenyCommand(pattern.trim());
            }
          }}
        >
          {busy
            ? "Adding…"
            : kind === "deny_tool"
              ? "Add tool denial"
              : kind === "deny_command"
              ? "Add denial"
              : kind === "sensitive_path"
                ? "Add path"
                : "Add advisory"}
        </button>
      </div>
    </div>
  );
}

function EvaluationComposer({
  busy,
  onCancel,
  onAddCheck,
  onAddRule,
}: {
  busy: boolean;
  onCancel: () => void;
  onAddCheck: (check: string) => void;
  onAddRule: (rule: string) => void;
}) {
  const [mode, setMode] = useState<"check" | "guidance">("check");
  const [value, setValue] = useState("");

  return (
    <div className="mx-auto max-w-xl space-y-3">
      <p className="text-xs font-medium text-neutral-700">Add to evaluations</p>
      <div className="flex gap-1 rounded-md border border-neutral-200 bg-white p-0.5">
        {(
          [
            ["check", "Automated check"],
            ["guidance", "Agent guidance"],
          ] as const
        ).map(([id, label]) => (
          <button
            key={id}
            type="button"
            onClick={() => setMode(id)}
            className={cn(
              "flex-1 rounded px-2 py-1 text-xs",
              mode === id
                ? "bg-neutral-900 text-white"
                : "text-neutral-600 hover:bg-neutral-50"
            )}
          >
            {label}
          </button>
        ))}
      </div>
      <label className="block space-y-1">
        <span className="text-[11px] text-neutral-500">
          {mode === "check"
            ? "Check id (e.g. tests, lint, no_secrets)"
            : "Guidance the judge / agent should follow"}
        </span>
        {mode === "check" ? (
          <input
            autoFocus
            className="w-full rounded-md border border-neutral-200 bg-white px-3 py-2 font-mono text-sm outline-none focus:border-neutral-400"
            placeholder="no_secrets"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && value.trim()) onAddCheck(slugifyId(value).replace(/-/g, "_"));
            }}
          />
        ) : (
          <textarea
            autoFocus
            rows={2}
            className="w-full rounded-md border border-neutral-200 bg-white px-3 py-2 text-sm outline-none focus:border-neutral-400"
            placeholder="Prefer .so/graph query before broad Grep"
            value={value}
            onChange={(e) => setValue(e.target.value)}
          />
        )}
      </label>
      <div className="flex justify-end gap-1.5">
        <button
          type="button"
          className="rounded px-2.5 py-1 text-xs text-neutral-500"
          onClick={onCancel}
        >
          Cancel
        </button>
        <button
          type="button"
          disabled={busy || !value.trim()}
          className="rounded-md bg-neutral-900 px-2.5 py-1 text-xs text-white disabled:opacity-50"
          onClick={() => {
            if (mode === "check") {
              onAddCheck(slugifyId(value).replace(/-/g, "_"));
            } else {
              onAddRule(value.trim());
            }
            setValue("");
          }}
        >
          {busy ? "Adding…" : mode === "check" ? "Add check" : "Add guidance"}
        </button>
      </div>
    </div>
  );
}
