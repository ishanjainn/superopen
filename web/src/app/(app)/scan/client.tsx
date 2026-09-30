"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { Suspense } from "react";
import FeaturePageHeader from "@/components/shell/feature-page-header";
import { useProject } from "@/components/shell/project-context";

type Finding = {
  rule_id?: string;
  title?: string;
  severity?: string;
  session_id?: string;
  reason?: string;
};

const RANK: Record<string, number> = {
  critical: 4,
  high: 3,
  medium: 2,
  low: 1,
  info: 0,
};

export default function ScanPage() {
  return (
    <Suspense fallback={<div className="grid h-full place-items-center text-sm text-neutral-500">Loading scan…</div>}>
      <ScanList />
    </Suspense>
  );
}

function ScanList() {
  const { projectId } = useProject();
  const session = useSearchParams().get("session") || "";
  const [items, setItems] = useState<Finding[]>([]);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    try {
      const url = new URL("/api/scan", window.location.origin);
      if (projectId) url.searchParams.set("project", projectId);
      if (session) url.searchParams.set("session", session);
      const res = await fetch(url.toString());
      const body = (await res.json()) as { items?: Finding[]; error?: string };
      setError(body.error || "");
      const rows = Array.isArray(body.items) ? body.items : [];
      rows.sort((a, b) => (RANK[b.severity || ""] ?? 0) - (RANK[a.severity || ""] ?? 0));
      setItems(rows);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not load scan");
    }
  }, [projectId, session]);

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(timer);
  }, [load]);

  return (
    <div className="flex h-full min-h-0 flex-col">
      <FeaturePageHeader title="Scan" />
      <div className="min-h-0 flex-1 overflow-auto p-5">
        {error ? <div className="mb-4 rounded-lg bg-red-50 p-3 text-sm text-red-700">{error}</div> : null}
        {items.length === 0 ? (
          <p className="text-sm text-neutral-500">No findings</p>
        ) : (
          <div className="mx-auto flex max-w-3xl flex-col gap-3">
            {items.map((item, i) => (
              <article key={`${item.rule_id}-${item.session_id}-${i}`} className="rounded-xl border border-neutral-200 bg-white px-4 py-3">
                <header className="flex flex-wrap items-center gap-2">
                  <span className="rounded-full bg-neutral-100 px-2 py-0.5 text-[11px] uppercase tracking-wide text-neutral-600">
                    {item.severity || "info"}
                  </span>
                  <span className="font-mono text-sm text-neutral-800">{item.rule_id}</span>
                  {item.session_id ? (
                    <Link
                      href={`/sessions/${encodeURIComponent(item.session_id)}`}
                      className="ml-auto font-mono text-[11px] text-neutral-500 hover:text-neutral-800"
                    >
                      {item.session_id}
                    </Link>
                  ) : null}
                </header>
                <h2 className="mt-2 text-sm font-medium text-neutral-900">{item.title || item.reason}</h2>
                {item.reason && item.title ? <p className="mt-1 text-sm text-neutral-600">{item.reason}</p> : null}
              </article>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
