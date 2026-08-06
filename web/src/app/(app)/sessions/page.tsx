"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useSoftPoll } from "@/hooks/use-soft-poll";
import FeaturePageHeader from "@/components/shell/feature-page-header";
import { useProject } from "@/components/shell/project-context";
import PortWizard from "@/components/port-wizard";
import SessionSearchBar from "@/components/session-search-bar";
import {
  displayUser,
  type SessionQueryFacets,
} from "@/lib/so/session-query";

type Session = {
  id: string;
  prompt_preview?: string;
  title?: string;
  vendor?: string;
  model?: string;
  user?: string;
  tokens?: number;
  cost_usd?: number;
  started_at?: string;
  checkpoints?: number;
  turns?: number;
  files?: string[];
  match?: string;
  project_id?: string;
  commits?: { sha?: string }[];
  pull_requests?: { url?: string; number?: number }[];
  branch?: string;
};

function vendorLabel(vendor?: string): string {
  const v = (vendor || "").toLowerCase();
  if (v.includes("claude")) return "Claude Code";
  if (v.includes("codex")) return "Codex";
  if (v.includes("cursor")) return "Cursor";
  if (v.includes("gemini")) return "Gemini";
  if (v.includes("opencode")) return "OpenCode";
  if (v.includes("copilot")) return "Copilot";
  if (v === "pi") return "Pi";
  return vendor || "Agent";
}

function modelLabel(model?: string): string {
  if (!model) return "";
  return model
    .replace(/^cursor-/i, "")
    .replace(/-high$/i, "")
    .replace(/-/g, " ");
}

function dateGroupLabel(iso?: string): string {
  if (!iso) return "Unknown";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return "Unknown";
  const today = new Date();
  const startOf = (x: Date) =>
    new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
  const diffDays = Math.round((startOf(today) - startOf(d)) / 86400000);
  if (diffDays === 0) return "Today";
  if (diffDays === 1) return "Yesterday";
  return d.toLocaleDateString(undefined, {
    weekday: "long",
    day: "numeric",
    month: "short",
  });
}

function VendorMark({ vendor }: { vendor?: string }) {
  const v = (vendor || "").toLowerCase();
  const src = v.includes("claude")
    ? "/vendors/claude.png"
    : v.includes("codex")
      ? "/vendors/codex.png"
      : v.includes("cursor")
        ? "/vendors/cursor.png"
        : null;
  if (!src) return null;
  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img src={src} alt="" className="size-4 object-contain" />
  );
}

export default function SessionsPage() {
  const { projectId } = useProject();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [facets, setFacets] = useState<SessionQueryFacets>({ users: [], agents: [] });
  const [query, setQuery] = useState("");
  const [debounced, setDebounced] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [portOpen, setPortOpen] = useState(false);

  useEffect(() => {
    const t = setTimeout(() => setDebounced(query.trim()), 200);
    return () => clearTimeout(t);
  }, [query]);

  const load = useCallback(async (q: string, proj: string, opts?: { quiet?: boolean }) => {
    if (!opts?.quiet) setLoading(true);
    try {
      const params = new URLSearchParams();
      if (q) params.set("q", q);
      if (proj) params.set("project", proj);
      const qs = params.toString();
      const url = qs ? `/api/sessions?${qs}` : "/api/sessions";
      const r = await fetch(url);
      if (!r.ok) throw new Error(await r.text());
      const data = await r.json();
      if (Array.isArray(data)) {
        setSessions(data);
      } else {
        setSessions(Array.isArray(data?.sessions) ? data.sessions : []);
        if (data?.facets) {
          setFacets({
            users: Array.isArray(data.facets.users) ? data.facets.users : [],
            agents: Array.isArray(data.facets.agents) ? data.facets.agents : [],
          });
        }
      }
      setError("");
    } catch (e: any) {
      setError(String(e.message || e));
    } finally {
      if (!opts?.quiet) setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load(debounced, projectId);
  }, [debounced, projectId, load]);

  useSoftPoll(
    useCallback(() => {
      void load(debounced, projectId, { quiet: true });
    }, [load, debounced, projectId]),
    8000
  );

  const groups = useMemo(() => {
    const map = new Map<string, Session[]>();
    for (const s of sessions) {
      const key = dateGroupLabel(s.started_at);
      const list = map.get(key) || [];
      list.push(s);
      map.set(key, list);
    }
    return Array.from(map.entries());
  }, [sessions]);

  return (
    <div className="flex h-full min-h-0 flex-col bg-white">
      <FeaturePageHeader
        title="Sessions"
        actions={
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setPortOpen(true)}
              className="rounded-md border border-neutral-200 px-2.5 py-1 text-xs text-neutral-700 hover:bg-neutral-50"
            >
              Port
            </button>
          </div>
        }
      />
      <PortWizard open={portOpen} onClose={() => setPortOpen(false)} />

      <div className="flex shrink-0 items-center gap-2 border-b border-neutral-200 px-4 py-2">
        <SessionSearchBar value={query} onChange={setQuery} facets={facets} />
      </div>

      <div className="min-h-0 flex-1 overflow-auto">
        {error && <p className="px-4 py-3 text-sm text-red-600">{error}</p>}
        {!error && !loading && sessions.length === 0 && (
          <p className="px-4 py-6 text-sm text-neutral-500">
            {debounced
              ? `No sessions match “${debounced}”.`
              : "No sessions yet. Run an instrumented agent or `so sessions demo`."}
          </p>
        )}

        {groups.map(([label, rows]) => (
          <section key={label}>
            <div className="flex items-center justify-between border-b border-neutral-100 bg-neutral-50/80 px-4 py-2">
              <h2 className="text-xs font-medium text-neutral-500">{label}</h2>
              <span className="text-xs text-neutral-400">
                {rows.length} session{rows.length === 1 ? "" : "s"}
              </span>
            </div>
            <ul>
              {rows.map((s) => (
                <li
                  key={`${s.project_id || ""}:${s.id}`}
                  className="border-b border-neutral-100"
                >
                  <Link
                    href={`/sessions/${encodeURIComponent(s.id)}${
                      s.project_id
                        ? `?project=${encodeURIComponent(s.project_id)}`
                        : ""
                    }`}
                    className="flex items-center gap-3 px-4 py-2.5 hover:bg-neutral-50"
                  >
                    {/* eslint-disable-next-line @next/next/no-img-element */}
                    <img
                      src="/vendors/user.png"
                      alt=""
                      className="size-7 shrink-0 rounded-full object-cover bg-neutral-200"
                      onError={(e) => {
                        (e.target as HTMLImageElement).style.display = "none";
                      }}
                    />
                    <div className="min-w-0 flex-1">
                      <div className="flex min-w-0 items-center gap-2">
                        <span className="truncate text-sm text-neutral-900">
                          {s.title || s.prompt_preview || s.id}
                        </span>
                        {s.match && debounced && (
                          <span
                            className="hidden max-w-[14rem] truncate rounded bg-amber-50 px-1.5 py-0.5 text-[10px] text-amber-800 sm:inline"
                            title={s.match}
                          >
                            {s.match.startsWith("file:")
                              ? `file · ${s.match.slice(5)}`
                              : s.match.startsWith("tool:")
                                ? `tool · ${s.match.slice(5)}`
                              : s.match.startsWith("chat:")
                                ? `chat · ${s.match.slice(5)}`
                                : s.match.startsWith("from:")
                                  ? s.match
                                  : s.match.startsWith("agent:")
                                    ? s.match
                                    : s.match}
                          </span>
                        )}
                      </div>
                      {s.user && (
                        <div className="truncate text-[11px] text-neutral-400" title={s.user}>
                          {displayUser(s.user)}
                        </div>
                      )}
                    </div>
                    <div className="flex shrink-0 items-center gap-3 text-[11px] text-neutral-500">
                      <span className="inline-flex items-center gap-1.5">
                        <VendorMark vendor={s.vendor} />
                        <span className="text-neutral-700">
                          {vendorLabel(s.vendor)}
                        </span>
                      </span>
                      {s.model && (
                        <span className="hidden font-mono md:inline">
                          {modelLabel(s.model)}
                        </span>
                      )}
                      {(s.tokens || 0) > 0 && (
                        <span className="hidden tabular-nums text-neutral-400 lg:inline">
                          {s.tokens! >= 1000
                            ? `${(s.tokens! / 1000).toFixed(1)}k tok`
                            : `${s.tokens} tok`}
                        </span>
                      )}
                      <span
                        className="tabular-nums text-neutral-400"
                        title="Checkpoints created on git commit / finalize"
                      >
                        {s.checkpoints || 0} cp
                      </span>
                    </div>
                  </Link>
                </li>
              ))}
            </ul>
          </section>
        ))}
      </div>
    </div>
  );
}
