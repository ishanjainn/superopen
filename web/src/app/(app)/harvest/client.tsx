"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import FeaturePageHeader from "@/components/shell/feature-page-header";
import HarvestDiff from "@/components/harvest-diff";
import { diffStats } from "@/lib/harvest-diff";

type Evidence = {
  kind: string;
  id?: string;
  span_id?: string;
  qn?: string;
  path?: string;
  label?: string;
};

type Proposal = {
  id: number;
  session_id?: string;
  status: string;
  kind: string;
  target: string;
  title: string;
  reason: string;
  issue?: string;
  suggestion?: string;
  diff?: string;
  plus?: number;
  minus?: number;
  evidence?: Evidence[];
  source?: string;
  created_at?: string;
};

type Tab = "open" | "history";

export default function HarvestPage() {
  const [tab, setTab] = useState<Tab>("open");
  const [items, setItems] = useState<Proposal[]>([]);
  const [history, setHistory] = useState<Proposal[]>([]);
  const [latest, setLatest] = useState<Proposal | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState<number | null>(null);

  const load = useCallback(async () => {
    try {
      const [openRes, histRes] = await Promise.all([
        fetch("/api/harvest"),
        fetch("/api/harvest?history=1"),
      ]);
      const openBody = (await openRes.json()) as {
        items?: Proposal[];
        latest?: Proposal | null;
        error?: string;
      };
      const histBody = (await histRes.json()) as { items?: Proposal[]; error?: string };
      if (openBody.error) setError(openBody.error);
      else if (histBody.error) setError(histBody.error);
      else setError("");
      setItems(Array.isArray(openBody.items) ? openBody.items : []);
      setLatest(openBody.latest ?? null);
      setHistory(Array.isArray(histBody.items) ? histBody.items : []);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not load harvest");
    }
  }, []);

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(timer);
  }, [load]);

  async function act(id: number, action: "apply" | "decline", force?: boolean) {
    setBusy(id);
    setError("");
    try {
      const res = await fetch(`/api/harvest/${id}/${action}`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ force: Boolean(force) }),
      });
      const body = (await res.json().catch(() => ({}))) as { error?: string };
      if (!res.ok || body.error) {
        setError(body.error || `Could not ${action}`);
      } else {
        await load();
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : `Could not ${action}`);
    } finally {
      setBusy(null);
    }
  }

  const skippedEmpty =
    items.length === 0 && (latest?.status === "skipped" || history[0]?.status === "skipped");

  return (
    <div className="flex h-full min-h-0 flex-col">
      <FeaturePageHeader title="Harvest" />
      <div className="flex shrink-0 gap-2 border-b border-neutral-200 px-5 py-2">
        <button
          type="button"
          className={tab === "open" ? "rounded-full bg-neutral-900 px-3 py-1 text-xs text-white" : "rounded-full border border-neutral-300 px-3 py-1 text-xs text-neutral-700"}
          onClick={() => setTab("open")}
        >
          Open
        </button>
        <button
          type="button"
          className={tab === "history" ? "rounded-full bg-neutral-900 px-3 py-1 text-xs text-white" : "rounded-full border border-neutral-300 px-3 py-1 text-xs text-neutral-700"}
          onClick={() => setTab("history")}
        >
          History
        </button>
      </div>
      <div className="min-h-0 flex-1 overflow-auto p-5">
        {error ? (
          <div className="mb-4 rounded-lg bg-red-50 p-3 text-sm text-red-700">
            {error}
          </div>
        ) : null}
        {tab === "open" ? (
          items.length === 0 ? (
            <p className="text-sm text-neutral-500">
              {skippedEmpty
                ? "No patches waiting. Last review skipped — nothing to change."
                : "No open playbook proposals."}
            </p>
          ) : (
            <div className="mx-auto flex max-w-3xl flex-col gap-4">
              {items.map((p) => (
                <ProposalCard
                  key={p.id}
                  proposal={p}
                  busy={busy === p.id}
                  onApply={(force) => void act(p.id, "apply", force)}
                  onDecline={() => void act(p.id, "decline")}
                />
              ))}
            </div>
          )
        ) : history.length === 0 ? (
          <p className="text-sm text-neutral-500">
            No harvest history in the session retention window (default 7 days).
          </p>
        ) : (
          <div className="mx-auto flex max-w-3xl flex-col gap-3">
            <p className="text-xs text-neutral-500">
              Closed reviews from the session retention window (default 7 days).
            </p>
            {history.map((p) => (
              <HistoryCard key={`${p.source || "item"}-${p.id}`} item={p} />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

function HistoryCard({ item }: { item: Proposal }) {
  return (
    <article className="rounded-xl border border-neutral-200 bg-white px-4 py-3">
      <header className="flex flex-wrap items-center gap-2">
        <span className="font-mono text-sm text-neutral-800">{item.target || "session"}</span>
        <span className="rounded-full bg-neutral-100 px-2 py-0.5 text-[11px] uppercase tracking-wide text-neutral-600">
          {item.status}
        </span>
        {item.kind ? (
          <span className="rounded-full bg-neutral-50 px-2 py-0.5 text-[11px] text-neutral-500">
            {item.kind}
          </span>
        ) : null}
        {item.created_at ? (
          <span className="ml-auto font-mono text-[11px] text-neutral-400">{item.created_at}</span>
        ) : null}
      </header>
      <h2 className="mt-2 text-sm font-medium text-neutral-900">{item.title}</h2>
      {item.reason ? <p className="mt-1 text-sm text-neutral-600">{item.reason}</p> : null}
    </article>
  );
}

function ProposalCard({
  proposal,
  busy,
  onApply,
  onDecline,
}: {
  proposal: Proposal;
  busy: boolean;
  onApply: (force: boolean) => void;
  onDecline: () => void;
}) {
  const stats = diffStats(proposal.diff || "");
  const plus = proposal.plus || stats.plus;
  const minus = proposal.minus || stats.minus;
  const stale = proposal.status === "stale";
  const simplify = proposal.kind === "simplify";
  const needsForce =
    simplify || proposal.kind === "create" || minus > 0;

  return (
    <article className="rounded-xl border border-neutral-200 bg-white">
      <header className="flex flex-wrap items-center gap-2 border-b border-neutral-100 px-4 py-3">
        <span className="font-mono text-sm text-neutral-800">{proposal.target}</span>
        <span className="rounded-full bg-neutral-100 px-2 py-0.5 text-[11px] uppercase tracking-wide text-neutral-600">
          {proposal.kind}
        </span>
        {simplify ? (
          <span className="rounded-full bg-amber-50 px-2 py-0.5 text-[11px] text-amber-800">
            simplify
          </span>
        ) : null}
        {stale ? (
          <span className="rounded-full bg-neutral-200 px-2 py-0.5 text-[11px] text-neutral-700">
            stale
          </span>
        ) : null}
        <span className="ml-auto font-mono text-xs">
          <span className="text-emerald-600">+{plus}</span>{" "}
          <span className="text-red-500">-{minus}</span>
        </span>
      </header>
      <div className="space-y-3 px-4 py-3">
        <h2 className="text-sm font-medium text-neutral-900">{proposal.title}</h2>
        <p className="text-sm text-neutral-700">{proposal.reason}</p>
        {proposal.evidence?.length ? (
          <div className="flex flex-wrap gap-1.5">
            {proposal.evidence.map((ev, i) => (
              <EvidenceChip key={i} evidence={ev} />
            ))}
          </div>
        ) : null}
        <HarvestDiff diff={proposal.diff || ""} />
        <div className="flex gap-2 pt-1">
          <button
            type="button"
            disabled={busy || stale}
            onClick={() => {
              if (needsForce && !window.confirm("Apply a non-additive playbook patch?")) {
                return;
              }
              onApply(needsForce);
            }}
            className="rounded-md bg-neutral-900 px-3 py-1.5 text-xs font-medium text-white disabled:opacity-40"
          >
            Apply
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={onDecline}
            className="rounded-md border border-neutral-200 px-3 py-1.5 text-xs text-neutral-700 disabled:opacity-40"
          >
            Decline
          </button>
        </div>
      </div>
    </article>
  );
}

function EvidenceChip({ evidence }: { evidence: Evidence }) {
  const href = evidenceHref(evidence);
  const label =
    evidence.label ||
    [evidence.kind, evidence.id || evidence.qn || evidence.path]
      .filter(Boolean)
      .join(" ");
  if (!href) {
    return (
      <span className="rounded-full bg-neutral-100 px-2 py-0.5 text-[11px] text-neutral-600">
        {label}
      </span>
    );
  }
  return (
    <Link
      href={href}
      className="rounded-full bg-neutral-100 px-2 py-0.5 text-[11px] text-neutral-700 hover:bg-neutral-200"
    >
      {label}
    </Link>
  );
}

function evidenceHref(evidence: Evidence): string {
  if (evidence.kind === "session" && evidence.id) {
    return `/sessions/${encodeURIComponent(evidence.id)}`;
  }
  if (evidence.kind === "memory" && evidence.id) {
    return `/memory?id=${encodeURIComponent(evidence.id)}`;
  }
  if (evidence.kind === "graph") {
    const q = evidence.qn || evidence.path || "";
    return q ? `/graph?q=${encodeURIComponent(q)}` : "/graph";
  }
  return "";
}
