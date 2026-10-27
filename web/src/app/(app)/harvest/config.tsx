"use client";

import { useCallback, useEffect, useState } from "react";

type JevSettings = { enabled?: boolean; key_set?: boolean; error?: string };

/** Harvest configuration: whether Jev reviews finished sessions, and its key. */
export default function HarvestConfig({ onChanged }: { onChanged: () => void }) {
  const [enabled, setEnabled] = useState(false);
  const [keySet, setKeySet] = useState(false);
  const [key, setKey] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [status, setStatus] = useState("");

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void (async () => {
        try {
          const res = await fetch("/api/settings/jev");
          const body = (await res.json()) as JevSettings;
          if (res.ok) {
            setEnabled(Boolean(body.enabled));
            setKeySet(Boolean(body.key_set));
          }
        } catch {
          /* stays at the default off position */
        }
      })();
    }, 0);
    return () => window.clearTimeout(timer);
  }, []);

  const save = useCallback(async () => {
    setBusy(true);
    setError("");
    setStatus("");
    try {
      const res = await fetch("/api/settings/jev", {
        method: "PUT",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ enabled, api_key: key }),
      });
      const body = (await res.json().catch(() => ({}))) as JevSettings;
      if (!res.ok) throw new Error(body.error || "save failed");
      setEnabled(Boolean(body.enabled));
      setKeySet(Boolean(body.key_set));
      setKey("");
      setStatus(body.enabled ? "Jev harvest is on." : "Jev harvest is off.");
      onChanged();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }, [enabled, key, onChanged]);

  return (
    <div className="max-w-2xl space-y-4 text-sm">
      {error ? (
        <p className="rounded border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700">{error}</p>
      ) : null}
      {status ? (
        <p className="rounded border border-neutral-200 bg-neutral-50 px-3 py-2 text-xs text-neutral-600">
          {status}
        </p>
      ) : null}
      <section className="rounded border border-neutral-200 p-4">
        <h2 className="font-medium text-neutral-900">Jev harvest</h2>
        <p className="mt-1 text-xs text-neutral-500">
          Off by default. When on, the Harvest page asks Jev whether the latest finished session is
          worth keeping. Coding agents are not asked to propose.
        </p>
        <label className="mt-3 flex items-center gap-2 text-sm text-neutral-800">
          <input
            type="checkbox"
            checked={enabled}
            disabled={busy}
            onChange={(event) => setEnabled(event.target.checked)}
          />
          Use Jev
        </label>
        <label className="mt-3 block text-xs text-neutral-600">
          TYPESAFE_API_KEY
          <input
            type="password"
            autoComplete="off"
            disabled={busy}
            value={key}
            placeholder={keySet ? "Saved" : "Required when Jev is on"}
            onChange={(event) => setKey(event.target.value)}
            className="mt-1.5 w-full rounded border border-neutral-300 px-2 py-1.5 font-mono text-xs outline-none focus:border-neutral-500"
          />
        </label>
        <button
          type="button"
          disabled={busy}
          onClick={() => void save()}
          className="mt-3 rounded border border-neutral-200 px-2.5 py-1.5 text-xs font-medium text-neutral-700 hover:bg-neutral-50 disabled:opacity-50"
        >
          Save
        </button>
      </section>
      <p className="text-xs leading-relaxed text-neutral-500">
        These are also set with the environment variables <code>SUPEROPEN_HARVEST_JEV=1</code> and{" "}
        <code>TYPESAFE_API_KEY</code>. Closed harvest history is deleted with session retention
        (Settings, Retention); open proposals are kept until you apply or decline them.
      </p>
    </div>
  );
}
