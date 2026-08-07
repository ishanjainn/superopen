"use client";

import { useCallback, useEffect, useState, type ReactNode } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import FeaturePageHeader, {
  FeatureBackLink,
} from "@/components/shell/feature-page-header";
import { useBreadcrumbCrumb } from "@/components/shell/breadcrumb-context";
import { cn } from "@/lib/utils";
import { recommendationTypeLabel } from "@/lib/recommendation-type-label";
import type { Recommendation } from "@/lib/so/misc";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog";

export default function RecDetailPage() {
  const params = useParams();
  const id = decodeURIComponent(String(params?.id || ""));
  const [rec, setRec] = useState<Recommendation | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState<"apply" | "dismiss" | "revert" | null>(null);
  const [decisionKind, setDecisionKind] = useState<
    "apply" | "dismiss" | "revert" | null
  >(null);
  const [decisionReason, setDecisionReason] = useState("");

  useBreadcrumbCrumb(rec?.title || (loading ? null : id.slice(0, 24) || null));

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const r = await fetch(`/api/recommendations/${encodeURIComponent(id)}`);
      if (r.status === 404) {
        setRec(null);
        setError("Recommendation not found");
        return;
      }
      if (!r.ok) throw new Error(await r.text());
      setRec((await r.json()) as Recommendation);
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e));
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    void load();
  }, [load]);

  function requestDecision(kind: "apply" | "dismiss" | "revert") {
    setDecisionReason("");
    setDecisionKind(kind);
  }

  async function action(kind: "apply" | "dismiss" | "revert") {
    const reason = decisionReason.trim();
    if (!reason) return;
    setBusy(kind);
    try {
      const r = await fetch(`/api/recommendations/${encodeURIComponent(id)}/${kind}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ reason }),
      });
      if (!r.ok) throw new Error(await r.text());
      await load();
      setDecisionKind(null);
      setDecisionReason("");
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e));
    } finally {
      setBusy(null);
    }
  }

  const open = String(rec?.status || "pending") === "pending";
  const applied = String(rec?.status || "") === "applied";
  const related = [
    ...(rec?.related_sessions || []),
    ...(rec?.session_id && !(rec?.related_sessions || []).includes(rec.session_id)
      ? [rec.session_id]
      : []),
  ];

  return (
    <div className="flex h-full min-h-0 flex-col bg-white">
      <FeaturePageHeader
        title={rec?.title || "Recommendation"}
        leading={
          <FeatureBackLink href="/recs" label="Back to recommendations" />
        }
      />
      <div className="min-h-0 flex-1 overflow-auto p-6">
        {loading && (
          <p className="text-sm text-neutral-500">Loading…</p>
        )}
        {error && !rec && (
          <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
            {error}
          </div>
        )}
        {rec && (
          <div className="mx-auto max-w-3xl space-y-6">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div className="min-w-0 space-y-2">
                <div className="flex flex-wrap items-center gap-2">
                  <StatusPill status={String(rec.status || "pending")} />
                  {rec.type && (
                    <span className="rounded border border-neutral-200 bg-white px-1.5 py-0.5 text-[10px] font-medium capitalize text-neutral-600">
                      {recommendationTypeLabel(rec.type)}
                    </span>
                  )}
                  <span className="font-mono text-[11px] text-neutral-400">
                    #{rec.id}
                  </span>
                </div>
                <h1 className="text-xl font-semibold text-neutral-900">
                  {rec.title || rec.id}
                </h1>
                {rec.created_at && (
                  <p className="text-xs text-neutral-500">
                    Opened {new Date(rec.created_at).toLocaleString()}
                  </p>
                )}
              </div>
              {open && (
                <div className="flex shrink-0 gap-2">
                  <button
                    type="button"
                    disabled={busy !== null}
                    onClick={() => requestDecision("apply")}
                    className="rounded-md bg-neutral-900 px-3 py-1.5 text-xs font-medium text-white disabled:opacity-50"
                  >
                    {busy === "apply" ? "Applying…" : "Approve"}
                  </button>
                  <button
                    type="button"
                    disabled={busy !== null}
                    onClick={() => requestDecision("dismiss")}
                    className="rounded-md bg-neutral-100 px-3 py-1.5 text-xs font-medium text-neutral-800 disabled:opacity-50"
                  >
                    {busy === "dismiss" ? "Dismissing…" : "Dismiss"}
                  </button>
                </div>
              )}
              {applied && (
                <div className="flex shrink-0 gap-2">
                  <button
                    type="button"
                    disabled={busy !== null}
                    onClick={() => requestDecision("revert")}
                    className="rounded-md border border-neutral-300 bg-white px-3 py-1.5 text-xs font-medium text-neutral-800 disabled:opacity-50"
                  >
                    {busy === "revert" ? "Reverting…" : "Revert"}
                  </button>
                </div>
              )}
            </div>

            {error && (
              <p className="text-sm text-red-600">{error}</p>
            )}

            {!open && (
              <dl className="rounded-lg border border-neutral-200 bg-neutral-50 p-4">
                <Field label={decisionLabel(String(rec.status || ""))}>
                  {rec.decision_reason || (
                    <span className="text-neutral-500">
                      No decision reason was recorded for this legacy action.
                    </span>
                  )}
                </Field>
                {(rec.decision_actor || rec.decision_at || rec.applied_at) && (
                  <div className="mt-3 flex flex-wrap gap-x-2 text-xs text-neutral-500">
                    {rec.decision_actor && <span>By {actorLabel(rec.decision_actor)}</span>}
                    {(rec.decision_at || rec.applied_at) && (
                      <span>
                        {new Date(String(rec.decision_at || rec.applied_at)).toLocaleString()}
                      </span>
                    )}
                  </div>
                )}
              </dl>
            )}

            <dl className="space-y-4 rounded-lg border border-neutral-200 p-4">
              {rec.rationale && <Field label="Reason">{rec.rationale}</Field>}
              {rec.why && rec.why !== rec.rationale && (
                <Field label="Why">{rec.why}</Field>
              )}
              {related.length > 0 && (
                <div>
                  <dt className="text-[11px] font-semibold uppercase tracking-wide text-neutral-400">
                    Related sessions
                  </dt>
                  <dd className="mt-1.5 flex flex-wrap gap-2">
                    {related.map((sid) => (
                      <Link
                        key={sid}
                        href={`/sessions/${encodeURIComponent(sid)}`}
                        className="rounded-md border border-neutral-200 bg-neutral-50 px-2 py-1 font-mono text-[11px] text-teal-800 hover:border-teal-300"
                      >
                        {sid.length > 24
                          ? sid.slice(0, 12) + "…" + sid.slice(-6)
                          : sid}
                      </Link>
                    ))}
                  </dd>
                </div>
              )}
              {rec.evidence && rec.evidence.length > 0 && (
                <div>
                  <dt className="text-[11px] font-semibold uppercase tracking-wide text-neutral-400">
                    Evidence
                  </dt>
                  <dd className="mt-1.5">
                    <ul className="list-disc space-y-1 pl-4 text-sm text-neutral-700">
                      {rec.evidence.map((e, i) => (
                        <li key={i}>{e}</li>
                      ))}
                    </ul>
                  </dd>
                </div>
              )}
              {rec.proposed_path && (
                <Field label="Applies to">
                  <span className="font-mono text-[12px] text-neutral-700">
                    {rec.proposed_path}
                  </span>
                </Field>
              )}
              {rec.proposed_body && (
                <div>
                  <dt className="text-[11px] font-semibold uppercase tracking-wide text-neutral-400">
                    Proposed change
                  </dt>
                  <dd className="mt-1.5">
                    <pre
                      className={cn(
                        "max-h-80 overflow-auto rounded-lg border border-neutral-200 bg-neutral-50 p-3",
                        "whitespace-pre-wrap font-mono text-[12px] text-neutral-700"
                      )}
                    >
                      {rec.proposed_body}
                    </pre>
                  </dd>
                </div>
              )}
            </dl>

            <Dialog
              open={decisionKind !== null}
              onOpenChange={(next) => {
                if (!next && busy === null) setDecisionKind(null);
              }}
            >
              <DialogContent className="max-w-lg">
                <DialogTitle>{decisionDialogTitle(decisionKind)}</DialogTitle>
                <DialogDescription>
                  This explanation is saved with the recommendation. Dismissal feedback also
                  becomes workspace memory for future recommendations.
                </DialogDescription>
                <label className="block space-y-2">
                  <span className="text-xs font-medium text-neutral-700">Reason</span>
                  <textarea
                    autoFocus
                    rows={4}
                    value={decisionReason}
                    onChange={(event) => setDecisionReason(event.target.value)}
                    placeholder={decisionPlaceholder(decisionKind)}
                    className="w-full resize-y rounded-md border border-neutral-300 bg-white px-3 py-2 text-sm text-neutral-900 outline-none focus:border-neutral-500"
                  />
                </label>
                <div className="flex justify-end gap-2">
                  <button
                    type="button"
                    disabled={busy !== null}
                    onClick={() => setDecisionKind(null)}
                    className="rounded-md border border-neutral-300 bg-white px-3 py-1.5 text-xs font-medium text-neutral-700 disabled:opacity-50"
                  >
                    Cancel
                  </button>
                  <button
                    type="button"
                    disabled={busy !== null || !decisionReason.trim() || !decisionKind}
                    onClick={() => decisionKind && void action(decisionKind)}
                    className="rounded-md bg-neutral-900 px-3 py-1.5 text-xs font-medium text-white disabled:opacity-50"
                  >
                    {busy ? "Saving…" : "Save decision"}
                  </button>
                </div>
              </DialogContent>
            </Dialog>
          </div>
        )}
      </div>
    </div>
  );
}

function decisionLabel(status: string): string {
  if (status === "dismissed") return "Why it was dismissed";
  if (status === "reverted") return "Why it was reverted";
  return "Why it was resolved";
}

function actorLabel(actor: string): string {
  if (actor === "human") return "a human";
  if (actor === "system") return "Superopen automation";
  return "a coding agent";
}

function decisionDialogTitle(kind: "apply" | "dismiss" | "revert" | null): string {
  if (kind === "dismiss") return "Why are you dismissing this?";
  if (kind === "revert") return "Why are you reverting this?";
  return "Why does this resolve the recommendation?";
}

function decisionPlaceholder(kind: "apply" | "dismiss" | "revert" | null): string {
  if (kind === "dismiss") return "For example: This duplicates an existing rule and would add noise.";
  if (kind === "revert") return "For example: The applied skill conflicted with the repository workflow.";
  return "For example: Added the skill and verified agents now load repository context before searching.";
}

function StatusPill({ status }: { status: string }) {
  const tone =
    status === "applied"
      ? "bg-emerald-50 text-emerald-800 border-emerald-200"
      : status === "dismissed"
        ? "bg-neutral-100 text-neutral-600 border-neutral-200"
        : "bg-amber-50 text-amber-900 border-amber-200";
  const label =
    status === "applied"
      ? "Resolved"
      : status === "dismissed"
        ? "Dismissed"
        : status === "reverted"
          ? "Reverted"
          : status === "stale"
            ? "Stale"
            : "Open";
  return (
    <span
      className={cn(
        "rounded border px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide",
        tone
      )}
    >
      {label}
    </span>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <dt className="text-[11px] font-semibold uppercase tracking-wide text-neutral-400">
        {label}
      </dt>
      <dd className="mt-1 text-sm text-neutral-800">{children}</dd>
    </div>
  );
}
