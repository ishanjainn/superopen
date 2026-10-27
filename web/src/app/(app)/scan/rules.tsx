"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useProject } from "@/components/shell/project-context";
import { cn } from "@/lib/utils";
import type { ScanRule } from "@/app/api/scan/rules/route";

const NEW = "__new__";

const NEW_RULE_TEMPLATE = `id: my-rule
version: 1
title: What this rule catches
description: >
  One or two sentences on why it matters.
severity: medium        # info | low | medium | high | critical
status: experimental    # experimental | stable | deprecated
posture: detect

# CEL expression over the event \`e\`. e.event.action is command.executed,
# file.read or file.edited; commands are in e.command.command, paths in e.file.path.
match: >
  e.event.action == "command.executed" &&
  e.command.command.matches("(?i)\\\\bsudo\\\\s+rm\\\\b")

emit:
  reason: "A command used sudo rm"

# A rule is only saved when every test gives its stated verdict.
tests:
  - name: sudo_rm
    verdict: match
    events:
      - event: { action: command.executed }
        command: { command: "sudo rm -rf /var/log/x" }
        session: { id: s1 }
  - name: plain_rm
    verdict: no_match
    events:
      - event: { action: command.executed }
        command: { command: "rm build/out.o" }
        session: { id: s1 }
`;

/** The CLI reports a bad rule as "line 12: message". */
function parseRuleError(error: string): { line: number; message: string } | null {
  const m = /^line (\d+): ([\s\S]*)$/.exec(error);
  return m ? { line: Number(m[1]), message: m[2] } : null;
}

const SEVERITY_TONE: Record<string, string> = {
  critical: "bg-red-100 text-red-800",
  high: "bg-orange-100 text-orange-800",
  medium: "bg-amber-100 text-amber-800",
  low: "bg-sky-100 text-sky-800",
  info: "bg-neutral-100 text-neutral-600",
};

const SOURCE_LABEL: Record<ScanRule["source"], string> = {
  baseline: "built-in",
  store: "custom",
  override: "edited built-in",
};

/** Detection rules: every rule, with the same create/edit/disable/delete the CLI has. */
export default function RulesPanel() {
  const { projectId } = useProject();
  const [rules, setRules] = useState<ScanRule[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [selected, setSelected] = useState<string>("");
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const editorRef = useRef<HTMLTextAreaElement>(null);
  const gutterRef = useRef<HTMLDivElement>(null);

  const url = useCallback(
    (path: string) => {
      const u = new URL(path, window.location.origin);
      if (projectId) u.searchParams.set("project", projectId);
      return u.toString();
    },
    [projectId],
  );

  const load = useCallback(async () => {
    try {
      const res = await fetch(url("/api/scan/rules"));
      const body = (await res.json()) as { items?: ScanRule[]; error?: string };
      setError(body.error || "");
      setRules(Array.isArray(body.items) ? body.items : []);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not load rules");
    } finally {
      setLoaded(true);
    }
  }, [url]);

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(timer);
  }, [load]);

  const current = useMemo(() => rules.find((r) => r.id === selected), [rules, selected]);
  const isNew = selected === NEW;
  const original = isNew ? NEW_RULE_TEMPLATE : (current?.yaml ?? "");
  const dirty = Boolean(selected) && draft !== original;
  const located = parseRuleError(error);
  const lineCount = draft.split("\n").length;

  /** Select a line in the editor and scroll it into view. */
  const goToLine = useCallback(
    (line: number) => {
      const el = editorRef.current;
      if (!el) return;
      const lines = el.value.split("\n");
      const index = Math.min(Math.max(line, 1), lines.length) - 1;
      const start = lines.slice(0, index).reduce((n, l) => n + l.length + 1, 0);
      el.focus();
      el.setSelectionRange(start, start + lines[index].length);
      const lineHeight = parseFloat(getComputedStyle(el).lineHeight) || 20;
      el.scrollTop = Math.max(0, (index - 3) * lineHeight);
      if (gutterRef.current) gutterRef.current.scrollTop = el.scrollTop;
    },
    [],
  );

  // A rejected save jumps to the line that needs fixing.
  useEffect(() => {
    if (located) goToLine(located.line);
  }, [error]); // eslint-disable-line react-hooks/exhaustive-deps

  const open = (id: string, yaml: string) => {
    if (dirty && !window.confirm("Discard unsaved changes to this rule?")) return;
    setSelected(id);
    setDraft(yaml);
    setError("");
    setNotice("");
  };

  async function call(path: string, init: RequestInit): Promise<{ ok: boolean; body: Record<string, unknown> }> {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const res = await fetch(url(path), { ...init, headers: { "content-type": "application/json" } });
      const body = (await res.json().catch(() => ({}))) as Record<string, unknown>;
      if (!res.ok || body.error) {
        setError(String(body.error || "Request failed"));
        return { ok: false, body };
      }
      return { ok: true, body };
    } catch (err) {
      setError(err instanceof Error ? err.message : "Request failed");
      return { ok: false, body: {} };
    } finally {
      setBusy(false);
    }
  }

  async function save() {
    const { ok, body } = await call("/api/scan/rules", { method: "PUT", body: JSON.stringify({ yaml: draft }) });
    if (!ok) return;
    const saved = body.rule as ScanRule;
    await load();
    setSelected(saved.id);
    setDraft(saved.yaml);
    setNotice(`Saved ${saved.id}. The next scan uses it.`);
  }

  async function toggle(rule: ScanRule, enabled: boolean) {
    const { ok, body } = await call(`/api/scan/rules/${encodeURIComponent(rule.id)}`, {
      method: "PATCH",
      body: JSON.stringify({ enabled }),
    });
    if (!ok) return;
    await load();
    if (rule.id === selected && !dirty) setDraft((body.rule as ScanRule).yaml);
  }

  async function remove(rule: ScanRule) {
    const restore = rule.source === "override";
    const message = restore
      ? `Revert ${rule.id} to the built-in version? Your edits are deleted.`
      : `Delete ${rule.id}? This removes its file.`;
    if (!window.confirm(message)) return;
    const { ok } = await call(`/api/scan/rules/${encodeURIComponent(rule.id)}`, { method: "DELETE" });
    if (!ok) return;
    await load();
    setSelected("");
    setDraft("");
    setNotice(restore ? `${rule.id} is back to the built-in version.` : `Deleted ${rule.id}.`);
  }

  if (!loaded) {
    return <p className="p-5 text-sm text-neutral-500">Loading rules…</p>;
  }

  return (
    <div className="flex min-h-0 flex-1">
      <aside className="flex w-80 shrink-0 flex-col border-r border-neutral-200">
        <div className="flex items-center justify-between border-b border-neutral-200 px-4 py-2.5">
          <span className="text-xs text-neutral-500">
            {rules.filter((r) => r.enabled).length} of {rules.length} enabled
          </span>
          <button
            type="button"
            onClick={() => open(NEW, NEW_RULE_TEMPLATE)}
            className="rounded border border-neutral-300 px-2.5 py-1 text-xs text-neutral-800 hover:bg-neutral-50"
          >
            New rule
          </button>
        </div>
        <ul className="min-h-0 flex-1 overflow-auto">
          {rules.map((rule) => (
            <li key={rule.id} className={cn("border-b border-neutral-100", selected === rule.id && "bg-neutral-50")}>
              <div className="flex items-start gap-3 px-4 py-3">
                <input
                  type="checkbox"
                  aria-label={`${rule.enabled ? "Disable" : "Enable"} ${rule.id}`}
                  checked={rule.enabled}
                  disabled={busy}
                  onChange={(event) => void toggle(rule, event.target.checked)}
                  className="mt-1"
                />
                <button
                  type="button"
                  onClick={() => open(rule.id, rule.yaml)}
                  className="min-w-0 flex-1 text-left"
                >
                  <span className={cn("block truncate text-sm", rule.enabled ? "text-neutral-900" : "text-neutral-400")}>
                    {rule.title}
                  </span>
                  <span className="mt-0.5 block truncate font-mono text-[11px] text-neutral-500">{rule.id}</span>
                  <span className="mt-1.5 flex flex-wrap items-center gap-1.5">
                    <span
                      className={cn(
                        "rounded-full px-2 py-0.5 text-[10px] uppercase tracking-wide",
                        SEVERITY_TONE[rule.severity] ?? SEVERITY_TONE.info,
                      )}
                    >
                      {rule.severity}
                    </span>
                    <span className="text-[11px] text-neutral-500">{SOURCE_LABEL[rule.source]}</span>
                  </span>
                </button>
              </div>
            </li>
          ))}
        </ul>
      </aside>

      <section className="flex min-w-0 flex-1 flex-col">
        {!selected ? (
          <div className="grid flex-1 place-items-center p-6 text-center text-sm text-neutral-500">
            <div className="max-w-sm space-y-2">
              <p>Select a rule to read or edit it, or create a new one.</p>
              <p className="text-xs">
                Rules are YAML files in <code>.so/guards</code>. Changes here and in the CLI
                (<code>so scan rules</code>) are the same files, and the next scan uses them.
              </p>
              {notice ? <p className="text-xs text-emerald-600">{notice}</p> : null}
              {error ? <p className="text-xs text-red-600">{error}</p> : null}
            </div>
          </div>
        ) : (
          <>
            <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-neutral-200 px-4 py-2.5">
              <span className="font-mono text-sm text-neutral-900">{isNew ? "New rule" : selected}</span>
              {current ? (
                <span className="text-xs text-neutral-500">{SOURCE_LABEL[current.source]}</span>
              ) : null}
              <div className="ml-auto flex items-center gap-2">
                {current && current.source !== "baseline" ? (
                  <button
                    type="button"
                    disabled={busy}
                    onClick={() => void remove(current)}
                    className="rounded border border-neutral-300 px-2.5 py-1 text-xs text-neutral-700 hover:bg-neutral-50 disabled:opacity-50"
                  >
                    {current.source === "override" ? "Revert to built-in" : "Delete"}
                  </button>
                ) : null}
                <button
                  type="button"
                  disabled={busy || !dirty}
                  onClick={() => void save()}
                  className="rounded bg-neutral-900 px-3 py-1 text-xs font-medium text-white disabled:opacity-40"
                >
                  {busy ? "Saving…" : "Save"}
                </button>
              </div>
            </div>
            {error ? (
              <div className="flex shrink-0 items-start gap-3 border-b border-red-200 bg-red-50 px-4 py-2 text-xs text-red-700">
                <p className="min-w-0 flex-1 whitespace-pre-wrap">
                  {located ? <span className="font-semibold">Line {located.line}: </span> : null}
                  {located ? located.message : error}
                </p>
                {located ? (
                  <button
                    type="button"
                    onClick={() => goToLine(located.line)}
                    className="shrink-0 rounded border border-red-300 px-2 py-0.5 text-red-800 hover:bg-red-100"
                  >
                    Go to line {located.line}
                  </button>
                ) : null}
              </div>
            ) : null}
            {notice ? (
              <p className="shrink-0 border-b border-neutral-200 bg-neutral-50 px-4 py-2 text-xs text-neutral-600">
                {notice}
              </p>
            ) : null}
            <div className="flex min-h-0 flex-1">
              <div
                ref={gutterRef}
                aria-hidden
                className="w-12 shrink-0 select-none overflow-hidden border-r border-neutral-200 py-4 pr-2 text-right font-mono text-xs leading-relaxed text-neutral-400"
              >
                {Array.from({ length: lineCount }, (_, i) => (
                  <div
                    key={i}
                    className={cn(located?.line === i + 1 && "font-semibold text-red-600")}
                  >
                    {i + 1}
                  </div>
                ))}
              </div>
              <textarea
                ref={editorRef}
                aria-label="Rule YAML"
                spellCheck={false}
                wrap="off"
                value={draft}
                onChange={(event) => setDraft(event.target.value)}
                onScroll={(event) => {
                  if (gutterRef.current) gutterRef.current.scrollTop = event.currentTarget.scrollTop;
                }}
                className="min-h-0 min-w-0 flex-1 resize-none overflow-auto bg-transparent p-4 font-mono text-xs leading-relaxed text-neutral-900 outline-none"
              />
            </div>
            {current?.source === "baseline" ? (
              <p className="shrink-0 border-t border-neutral-200 px-4 py-2 text-xs text-neutral-500">
                Built-in rule. Saving an edit keeps your version as a custom copy; Revert brings the
                original back.
              </p>
            ) : null}
          </>
        )}
      </section>
    </div>
  );
}
