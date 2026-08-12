"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import FeaturePageHeader, {
  FeatureBackLink,
} from "@/components/shell/feature-page-header";
import { useBreadcrumbCrumb } from "@/components/shell/breadcrumb-context";
import { Dropdown } from "@/components/ui/dropdown";
import {
  decodeHarnessItemId,
  parseEvaluationsDoc,
  parseGuardrailsDoc,
  removeDeniedCommand,
  removeDeniedTool,
  removeEvaluationAgentRule,
  removeEvaluationCheck,
  removeEvaluationSource,
  removeGuardrailRule,
  removeSensitivePath,
  replaceListItem,
  setJudgeRubric,
  updateGuardrailRule,
  type GuardrailSeverity,
  type HarnessItemKind,
} from "@/lib/so/harness-yaml";
import { cn } from "@/lib/utils";

type Domain = "guardrails" | "evaluations";

const FILE: Record<Domain, string> = {
  guardrails: "guardrails/guardrails.yaml",
  evaluations: "evals/configs.yaml",
};

const KIND_BLURB: Record<HarnessItemKind, string> = {
  tool: "Hard stop: coding hooks deny matching vendor tool names before invocation.",
  command:
    "Hard stop: coding hooks deny matching shell commands at PreToolUse / beforeShell.",
  path: "Hard stop: coding hooks deny reads/writes matching this path glob.",
  advisory:
    "Soft guidance injected for agents - not a hard deny. Severity is a label for humans.",
  check: "Automated check id used when scoring sessions (so eval / Map judge).",
  agent_rule: "Guidance the judge and agents should follow when evaluating sessions.",
  source: "Reference document path used to ground evaluation criteria.",
  rubric: "Judge rubric text used when scoring sessions.",
};

export function HarnessItemDetailPage({ domain }: { domain: Domain }) {
  const params = useParams();
  const router = useRouter();
  const rawId = String(params.id || "");
  const parsed = useMemo(() => decodeHarnessItemId(rawId), [rawId]);

  const [yaml, setYaml] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [status, setStatus] = useState("");
  const [draft, setDraft] = useState("");
  const [severity, setSeverity] = useState<GuardrailSeverity>("warn");
  const [description, setDescription] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const r = await fetch(`/api/files/${FILE[domain]}`);
      if (!r.ok) throw new Error(await r.text());
      const text = await r.text();
      setYaml(text);
      if (!parsed) return;
      hydrate(domain, text, parsed.kind, parsed.key, setDraft, setSeverity, setDescription);
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e));
    } finally {
      setLoading(false);
    }
  }, [domain, parsed]);

  useEffect(() => {
    void load();
  }, [load]);

  async function write(next: string, thenNavigate?: string) {
    setSaving(true);
    setError("");
    setStatus("");
    try {
      const r = await fetch(`/api/files/${FILE[domain]}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ content: next, create: false }),
      });
      if (!r.ok) {
        const j = await r.json().catch(() => ({}));
        throw new Error(j.error || (await r.text()));
      }
      setYaml(next);
      setStatus("Saved.");
      if (thenNavigate) router.push(thenNavigate);
      else if (parsed) {
        hydrate(domain, next, parsed.kind, parsed.key, setDraft, setSeverity, setDescription);
      }
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e));
    } finally {
      setSaving(false);
    }
  }

  async function onSave() {
    if (!parsed) return;
    const { kind, key } = parsed;
    let next = yaml;
    if (domain === "guardrails") {
      if (kind === "tool") {
        next = replaceListItem(yaml, "denied_tools", key, draft.trim());
      } else if (kind === "command") {
        next = replaceListItem(yaml, "denied_commands", key, draft.trim());
      } else if (kind === "path") {
        next = replaceListItem(yaml, "sensitive_paths", key, draft.trim(), true);
      } else if (kind === "advisory") {
        next = updateGuardrailRule(yaml, key, {
          id: draft.trim() || key,
          description: description.trim(),
          severity,
          source: "ui",
        });
      }
    } else {
      if (kind === "check") {
        next = replaceListItem(yaml, "checks", key, draft.trim());
      } else if (kind === "agent_rule") {
        next = replaceListItem(yaml, "agent_rules", key, draft.trim(), true);
      } else if (kind === "source") {
        next = replaceListItem(yaml, "sources", key, draft.trim(), true);
      } else if (kind === "rubric") {
        next = setJudgeRubric(yaml, draft);
      }
    }
    const newKey =
      kind === "advisory"
        ? draft.trim() || key
        : kind === "rubric"
          ? "judge_rubric"
          : draft.trim() || key;
    const href =
      kind === "advisory" && newKey !== key
        ? `/${domain}/items/${encodeURIComponent(`${kind}::${newKey}`)}`
        : kind !== "rubric" && kind !== "advisory" && newKey !== key
          ? `/${domain}/items/${encodeURIComponent(`${kind}::${newKey}`)}`
          : undefined;
    await write(next, href);
  }

  async function onDelete() {
    if (!parsed) return;
    if (!confirm("Delete this item from .so/ config?")) return;
    const { kind, key } = parsed;
    let next = yaml;
    if (kind === "tool") next = removeDeniedTool(yaml, key);
    else if (kind === "command") next = removeDeniedCommand(yaml, key);
    else if (kind === "path") next = removeSensitivePath(yaml, key);
    else if (kind === "advisory") next = removeGuardrailRule(yaml, key);
    else if (kind === "check") next = removeEvaluationCheck(yaml, key);
    else if (kind === "agent_rule") next = removeEvaluationAgentRule(yaml, key);
    else if (kind === "source") next = removeEvaluationSource(yaml, key);
    else if (kind === "rubric") next = setJudgeRubric(yaml, "");
    await write(next, `/${domain}`);
  }

  const backHref = `/${domain}`;
  const title = domain === "guardrails" ? "Guard detail" : "Evaluator detail";
  const crumbLabel = parsed
    ? String(draft || parsed.key || parsed.kind).slice(0, 80)
    : null;
  useBreadcrumbCrumb(loading ? null : crumbLabel);

  if (!parsed) {
    return (
      <div className="flex h-full flex-col">
        <FeaturePageHeader
          title={title}
          leading={<FeatureBackLink href={backHref} label="Back" />}
        />
        <p className="p-6 text-sm text-red-600">Invalid item id.</p>
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col bg-white">
      <FeaturePageHeader
        title={title}
        leading={<FeatureBackLink href={backHref} label="Back" />}
        actions={
          <div className="flex gap-1.5">
            <button
              type="button"
              disabled={saving}
              onClick={() => void onDelete()}
              className="rounded-md border border-red-200 px-2.5 py-1 text-xs text-red-700 hover:bg-red-50 disabled:opacity-50"
            >
              Delete
            </button>
            <button
              type="button"
              disabled={saving}
              onClick={() => void onSave()}
              className="rounded-md bg-neutral-900 px-2.5 py-1 text-xs text-white hover:bg-neutral-800 disabled:opacity-50"
            >
              {saving ? "Saving…" : "Save"}
            </button>
          </div>
        }
      />
      <div className="min-h-0 flex-1 overflow-auto px-5 py-5">
        {loading ? (
          <p className="text-sm text-neutral-500">Loading…</p>
        ) : (
          <div className="mx-auto max-w-2xl space-y-5">
            {error && <p className="text-sm text-red-600">{error}</p>}
            {status && <p className="text-xs text-neutral-500">{status}</p>}

            <div className="flex flex-wrap items-center gap-2">
              <span className="rounded-md border border-neutral-200 bg-neutral-50 px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide text-neutral-600">
                {parsed.kind.replace("_", " ")}
              </span>
              {domain === "guardrails" &&
                (parsed.kind === "tool" || parsed.kind === "command" || parsed.kind === "path") && (
                  <span className="rounded-md bg-red-50 px-2 py-0.5 text-[10px] font-medium uppercase text-red-700">
                    Hard stop
                  </span>
                )}
            </div>

            <p className="text-sm text-neutral-600">{KIND_BLURB[parsed.kind]}</p>

            {parsed.kind === "advisory" ? (
              <>
                <label className="block space-y-1">
                  <span className="text-[11px] text-neutral-500">Id</span>
                  <input
                    className="w-full rounded-md border border-neutral-200 px-3 py-2 font-mono text-sm outline-none focus:border-neutral-400"
                    value={draft}
                    onChange={(e) => setDraft(e.target.value)}
                  />
                </label>
                <label className="block space-y-1">
                  <span className="text-[11px] text-neutral-500">Description</span>
                  <textarea
                    rows={4}
                    className="w-full rounded-md border border-neutral-200 px-3 py-2 text-sm outline-none focus:border-neutral-400"
                    value={description}
                    onChange={(e) => setDescription(e.target.value)}
                  />
                </label>
                <label className="block max-w-[10rem] space-y-1">
                  <span className="text-[11px] text-neutral-500">Severity</span>
                  <Dropdown
                    size="sm"
                    value={severity}
                    onChange={(v) => setSeverity(v as GuardrailSeverity)}
                    options={[
                      { value: "info", label: "info" },
                      { value: "warn", label: "warn" },
                      { value: "block", label: "block" },
                    ]}
                  />
                </label>
              </>
            ) : (
              <label className="block space-y-1">
                <span className="text-[11px] text-neutral-500">
                  {parsed.kind === "rubric"
                    ? "Rubric"
                    : parsed.kind === "check"
                      ? "Check id"
                      : parsed.kind === "tool"
                        ? "Tool name pattern"
                        : parsed.kind === "path"
                        ? "Path glob"
                        : parsed.kind === "command"
                          ? "Command pattern"
                          : "Value"}
                </span>
                {parsed.kind === "rubric" || parsed.kind === "agent_rule" ? (
                  <textarea
                    rows={parsed.kind === "rubric" ? 8 : 4}
                    className={cn(
                      "w-full rounded-md border border-neutral-200 px-3 py-2 text-sm outline-none focus:border-neutral-400",
                      parsed.kind !== "agent_rule" && "font-mono text-xs"
                    )}
                    value={draft}
                    onChange={(e) => setDraft(e.target.value)}
                  />
                ) : (
                  <input
                    className="w-full rounded-md border border-neutral-200 px-3 py-2 font-mono text-sm outline-none focus:border-neutral-400"
                    value={draft}
                    onChange={(e) => setDraft(e.target.value)}
                  />
                )}
              </label>
            )}

            <div className="rounded-lg border border-neutral-200 bg-neutral-50 px-3 py-2 text-[11px] text-neutral-500">
              Edits write to{" "}
              <code className="rounded bg-white px-1">.so/{FILE[domain]}</code>. Commit to
              share with teammates. Hook-enforced guards apply on the next tool call.
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

function hydrate(
  domain: Domain,
  yaml: string,
  kind: HarnessItemKind,
  key: string,
  setDraft: (v: string) => void,
  setSeverity: (v: GuardrailSeverity) => void,
  setDescription: (v: string) => void
) {
  if (domain === "guardrails") {
    const doc = parseGuardrailsDoc(yaml);
    if (kind === "tool" || kind === "command" || kind === "path") {
      setDraft(key);
      return;
    }
    if (kind === "advisory") {
      const rule = doc?.rules.find((r) => r.id === key);
      setDraft(rule?.id || key);
      setDescription(rule?.description || "");
      setSeverity((rule?.severity as GuardrailSeverity) || "warn");
    }
    return;
  }
  const doc = parseEvaluationsDoc(yaml);
  if (kind === "rubric") {
    setDraft(doc?.judgeRubric || "");
    return;
  }
  setDraft(key);
}
