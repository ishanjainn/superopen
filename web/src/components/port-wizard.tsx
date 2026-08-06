"use client";

import { useCallback, useEffect, useState } from "react";
import { Dropdown } from "@/components/ui/dropdown";

const HARNESSES = [
  { id: "claude", label: "Claude Code" },
  { id: "codex", label: "Codex" },
  { id: "opencode", label: "OpenCode" },
  { id: "cursor", label: "Cursor" },
  { id: "pi", label: "Pi" },
  { id: "so", label: ".so hub" },
] as const;

type SessionRef = {
  harness?: string;
  source_session_id: string;
  title?: string;
  cwd?: string;
  imported?: boolean;
  source_changed?: boolean;
};

type PortWizardProps = {
  open: boolean;
  onClose: () => void;
};

export default function PortWizard({ open, onClose }: PortWizardProps) {
  const [from, setFrom] = useState("claude");
  const [to, setTo] = useState("codex");
  const [refs, setRefs] = useState<SessionRef[]>([]);
  const [selected, setSelected] = useState<Record<string, boolean>>({});
  const [all, setAll] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState("");
  const [detect, setDetect] = useState<Record<string, { import?: boolean; export?: boolean }>>({});

  const loadDetect = useCallback(async () => {
    try {
      const r = await fetch("/api/sessions/port");
      const j = await r.json();
      if (j?.ok && j.data) setDetect(j.data);
    } catch {
      /* ignore */
    }
  }, []);

  useEffect(() => {
    if (open) void loadDetect();
  }, [open, loadDetect]);

  const preview = async () => {
    setLoading(true);
    setError("");
    setResult("");
    try {
      const r = await fetch("/api/sessions/port", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ op: "preview", from, to }),
      });
      const j = await r.json();
      if (!j.ok) throw new Error(j.error || "preview failed");
      const list = (j.data?.refs || []) as SessionRef[];
      setRefs(list);
      const sel: Record<string, boolean> = {};
      for (const ref of list.slice(0, 5)) {
        sel[ref.source_session_id] = true;
      }
      setSelected(sel);
    } catch (e: unknown) {
      setError(String((e as Error).message || e));
    } finally {
      setLoading(false);
    }
  };

  const runPort = async () => {
    setLoading(true);
    setError("");
    setResult("");
    try {
      const ids = Object.entries(selected)
        .filter(([, v]) => v)
        .map(([k]) => k);
      const r = await fetch("/api/sessions/port", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          op: "port",
          from,
          to,
          all,
          ids: all ? undefined : ids,
        }),
      });
      const j = await r.json();
      if (!j.ok) throw new Error(j.error || "port failed");
      const d = j.data || {};
      let msg = `Ported ${d.ported ?? 0}, skipped ${d.skipped ?? 0}, failed ${d.failed ?? 0}.`;
      if (d.resume_armed && d.resume_id) {
        msg += ` Next coding-agent SessionStart will inject ${d.resume_id} automatically.`;
      } else {
        msg += ` Memory pack refreshed.`;
      }
      setResult(msg);
    } catch (e: unknown) {
      setError(String((e as Error).message || e));
    } finally {
      setLoading(false);
    }
  };

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
      <div className="flex max-h-[90vh] w-full max-w-lg flex-col overflow-hidden rounded-lg border border-neutral-200 bg-white shadow-xl">
        <div className="flex shrink-0 items-center justify-between border-b border-neutral-200 px-4 py-3">
          <h2 className="text-sm font-medium text-neutral-900">Port sessions</h2>
          <button
            type="button"
            onClick={onClose}
            className="text-xs text-neutral-500 hover:text-neutral-800"
          >
            Close
          </button>
        </div>
        <div className="shrink-0 space-y-3 overflow-visible px-4 py-3 text-sm">
          <p className="text-xs text-neutral-500">
            Copies conversation text between coding agents (Claude ↔ Codex ↔ OpenCode ↔
            Cursor ↔ Pi ↔ .so). Tools/thinking are dropped in v1. After Port, the next
            SessionStart in the destination agent injects the transcript automatically.
          </p>
          <div className="grid grid-cols-2 gap-3">
            <div className="min-w-0 space-y-1">
              <span className="text-xs text-neutral-500">From</span>
              <Dropdown
                size="sm"
                aria-label="Port from harness"
                value={from}
                onChange={setFrom}
                options={HARNESSES.map((h) => ({
                  value: h.id,
                  label:
                    detect[h.id]?.import === false
                      ? `${h.label} (missing)`
                      : h.label,
                }))}
              />
            </div>
            <div className="min-w-0 space-y-1">
              <span className="text-xs text-neutral-500">To</span>
              <Dropdown
                size="sm"
                aria-label="Port to harness"
                value={to}
                onChange={setTo}
                options={HARNESSES.map((h) => ({
                  value: h.id,
                  label:
                    detect[h.id]?.export === false
                      ? `${h.label} (missing)`
                      : h.label,
                }))}
              />
            </div>
          </div>
          <div className="flex gap-2">
            <button
              type="button"
              disabled={loading || from === to}
              onClick={() => void preview()}
              className="rounded-md border border-neutral-200 px-2.5 py-1 text-xs hover:bg-neutral-50 disabled:opacity-40"
            >
              Preview
            </button>
            <label className="flex items-center gap-1.5 text-xs text-neutral-600">
              <input
                type="checkbox"
                checked={all}
                onChange={(e) => setAll(e.target.checked)}
              />
              Port all
            </label>
          </div>
        </div>
        {(refs.length > 0 || error || result) && (
          <div className="min-h-0 flex-1 space-y-3 overflow-auto px-4 pb-3 text-sm">
            {refs.length > 0 && (
              <ul className="max-h-48 space-y-1 overflow-auto rounded-md border border-neutral-100 p-2">
                {refs.map((r) => (
                  <li key={r.source_session_id} className="flex items-start gap-2">
                    <input
                      type="checkbox"
                      disabled={all}
                      checked={all || !!selected[r.source_session_id]}
                      onChange={(e) =>
                        setSelected((s) => ({
                          ...s,
                          [r.source_session_id]: e.target.checked,
                        }))
                      }
                      className="mt-0.5"
                    />
                    <div className="min-w-0">
                      <div className="truncate text-neutral-900">
                        {r.title || r.source_session_id}
                        {r.imported ? " · already ported" : ""}
                        {r.source_changed ? " · source changed" : ""}
                      </div>
                      <div className="truncate font-mono text-[10px] text-neutral-400">
                        {r.source_session_id}
                      </div>
                    </div>
                  </li>
                ))}
              </ul>
            )}
            {error && <p className="text-xs text-red-600">{error}</p>}
            {result && <p className="text-xs text-emerald-700">{result}</p>}
          </div>
        )}
        <div className="flex shrink-0 justify-end gap-2 border-t border-neutral-200 px-4 py-3">
          <button
            type="button"
            onClick={onClose}
            className="rounded-md px-2.5 py-1 text-xs text-neutral-600 hover:bg-neutral-50"
          >
            Cancel
          </button>
          <button
            type="button"
            disabled={loading || from === to}
            onClick={() => void runPort()}
            className="rounded-md bg-neutral-900 px-2.5 py-1 text-xs text-white hover:bg-neutral-800 disabled:opacity-40"
          >
            {loading ? "Working…" : "Port"}
          </button>
        </div>
      </div>
    </div>
  );
}
