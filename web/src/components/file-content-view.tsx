"use client";

import Link from "next/link";
import { useMemo, useState, type ReactNode } from "react";
import { MarkdownView } from "@/components/markdown-view";
import { Dropdown } from "@/components/ui/dropdown";
import DataTable from "@/components/data-table/table";
import type { Columns } from "@/components/data-table/columns";
import {
  parseGuardrailsDoc,
  parseEvaluationsDoc,
  harnessItemHref,
  type GuardrailsDoc,
  type EvaluationsDoc,
  type HarnessItemKind,
} from "@/lib/so/harness-yaml";
import { cn } from "@/lib/utils";
import { useRouter } from "next/navigation";

/** Renders harness file contents as structured UI instead of raw dumps. */
export function FileContentView({
  path,
  content,
  toolbarAction,
  domain,
}: {
  path: string;
  content: string;
  /** Optional control rendered at the end of structured-doc filter bars (e.g. Add). */
  toolbarAction?: ReactNode;
  /** When set, rows link to detail pages under this domain. */
  domain?: "guardrails" | "evaluations";
}) {
  const name = path.split("/").pop() || path;
  const lower = name.toLowerCase();

  if (lower.endsWith(".md") || lower.endsWith(".mdc") || lower.endsWith(".instructions.md")) {
    return (
      <article className="mx-auto max-w-3xl">
        <MarkdownView source={content} />
      </article>
    );
  }

  if (lower.endsWith(".json")) {
    let data: unknown = null;
    let ok = false;
    try {
      data = JSON.parse(content);
      ok = true;
    } catch {
      /* fall through to raw pre */
    }
    if (ok) return <JsonDocument data={data} />;
  }

  if (lower.endsWith(".yaml") || lower.endsWith(".yml")) {
    const guard = parseGuardrailsDoc(content);
    if (guard) {
      return (
        <GuardrailsDocument
          doc={guard}
          toolbarAction={toolbarAction}
          domain={domain === "guardrails" ? "guardrails" : undefined}
        />
      );
    }
    const evaluations = parseEvaluationsDoc(content);
    if (evaluations) {
      return (
        <EvaluationsDocument
          data={evaluations}
          toolbarAction={toolbarAction}
          domain={domain === "evaluations" ? "evaluations" : undefined}
        />
      );
    }
  }

  return (
    <article className="mx-auto max-w-3xl">
      <pre className="overflow-auto rounded-lg border border-neutral-200 bg-neutral-50 p-4 font-mono text-[12px] text-neutral-700 whitespace-pre-wrap">
        {content}
      </pre>
    </article>
  );
}

type GuardRowKind = "command" | "path" | "advisory";
type GuardRowMode = "enforced" | "advisory";

type GuardRow = {
  key: string;
  kind: GuardRowKind;
  mode: GuardRowMode;
  id: string;
  severity: string;
  source: string;
  detail: string;
  itemKey: string;
};

function buildGuardRows(doc: GuardrailsDoc): GuardRow[] {
  const rows: GuardRow[] = [];
  doc.denied_commands.forEach((c, i) => {
    rows.push({
      key: `cmd-${i}-${c}`,
      kind: "command",
      mode: "enforced",
      id: c,
      severity: "deny",
      source: "hooks",
      detail: "Denied at PreToolUse / beforeShell",
      itemKey: c,
    });
  });
  doc.sensitive_paths.forEach((p, i) => {
    rows.push({
      key: `path-${i}-${p}`,
      kind: "path",
      mode: "enforced",
      id: p,
      severity: "deny",
      source: "hooks",
      detail: "Denied at beforeRead / file hooks",
      itemKey: p,
    });
  });
  doc.rules.forEach((r) => {
    rows.push({
      key: `rule-${r.id}`,
      kind: "advisory",
      mode: "advisory",
      id: r.id,
      severity: r.severity || "warn",
      source: r.source || "-",
      detail: r.description,
      itemKey: r.id,
    });
  });
  return rows;
}

type GuardColumnKey =
  | "type"
  | "mode"
  | "id"
  | "severity"
  | "source"
  | "detail";

const GUARD_COLUMNS: Columns<GuardColumnKey, GuardRow> = {
  type: {
    header: () => "Type",
    width: "7.5rem",
    cell: ({ row }) => <KindBadge kind={row.kind} />,
  },
  mode: {
    header: () => "Mode",
    width: "7.5rem",
    cell: ({ row }) => <ModeBadge mode={row.mode} />,
  },
  id: {
    header: () => "Id / pattern",
    width: "minmax(12rem, 1.4fr)",
    cell: ({ row }) => (
      <span className="block truncate font-mono text-xs text-neutral-800">
        {row.id}
      </span>
    ),
  },
  severity: {
    header: () => "Severity",
    width: "6.5rem",
    cell: ({ row }) => <SeverityBadge severity={row.severity} />,
  },
  source: {
    header: () => "Source",
    width: "6rem",
    cell: ({ row }) => (
      <span className="rounded-md border border-neutral-200 bg-white px-2 py-0.5 text-[10px] text-neutral-600">
        {row.source}
      </span>
    ),
  },
  detail: {
    header: () => "Detail",
    width: "minmax(14rem, 2fr)",
    cell: ({ row }) => (
      <span className="line-clamp-2 text-sm text-neutral-700">{row.detail}</span>
    ),
  },
};

const GUARD_VISIBILITY: Record<GuardColumnKey, boolean> = {
  type: true,
  mode: true,
  id: true,
  severity: true,
  source: true,
  detail: true,
};

function GuardrailsDocument({
  doc,
  toolbarAction,
  domain,
}: {
  doc: GuardrailsDoc;
  toolbarAction?: ReactNode;
  domain?: "guardrails";
}) {
  const router = useRouter();
  const rows = useMemo(() => buildGuardRows(doc), [doc]);
  const [q, setQ] = useState("");
  const [kind, setKind] = useState<"all" | GuardRowKind>("all");
  const [mode, setMode] = useState<"all" | GuardRowMode>("all");
  const [severity, setSeverity] = useState("all");

  const severities = useMemo(() => {
    const s = new Set(rows.map((r) => r.severity.toLowerCase()));
    return ["all", ...Array.from(s).sort()];
  }, [rows]);

  const filtered = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return rows.filter((r) => {
      if (kind !== "all" && r.kind !== kind) return false;
      if (mode !== "all" && r.mode !== mode) return false;
      if (severity !== "all" && r.severity.toLowerCase() !== severity) return false;
      if (!needle) return true;
      return (
        r.id.toLowerCase().includes(needle) ||
        r.detail.toLowerCase().includes(needle) ||
        r.source.toLowerCase().includes(needle) ||
        r.kind.includes(needle) ||
        r.severity.toLowerCase().includes(needle)
      );
    });
  }, [rows, q, kind, mode, severity]);

  return (
    <div className="flex h-full min-h-0 w-full flex-col">
      <div className="flex shrink-0 flex-wrap items-center gap-3 border-b border-neutral-200 bg-white px-5 py-2.5">
        <input
          className="min-w-[14rem] flex-1 rounded-md border border-neutral-200 bg-white px-2.5 py-1.5 text-sm outline-none focus:border-neutral-400"
          placeholder="Filter by id, pattern, description…"
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
        <Dropdown
          size="sm"
          value={kind}
          onChange={(v) => setKind(v as typeof kind)}
          options={[
            { value: "all", label: "All types" },
            { value: "command", label: "Denied command" },
            { value: "path", label: "Sensitive path" },
            { value: "advisory", label: "Advisory rule" },
          ]}
        />
        <Dropdown
          size="sm"
          value={mode}
          onChange={(v) => setMode(v as typeof mode)}
          options={[
            { value: "all", label: "All modes" },
            { value: "enforced", label: "Hard stop" },
            { value: "advisory", label: "Soft guidance" },
          ]}
        />
        <Dropdown
          size="sm"
          value={severity}
          onChange={setSeverity}
          options={severities.map((s) => ({
            value: s,
            label: s === "all" ? "All severities" : s,
          }))}
        />
        {toolbarAction}
      </div>

      <div className="min-h-0 flex-1 overflow-auto px-5 py-4">
        <DataTable
          columns={GUARD_COLUMNS}
          data={filtered}
          isFetched
          isLoading={false}
          visibilityColumns={GUARD_VISIBILITY}
          emptyTitle="No guardrails match these filters"
          emptyBody="Clear search or change type / mode / severity to see rows."
          onClick={
            domain
              ? (row: GuardRow) => {
                  router.push(
                    harnessItemHref(domain, row.kind as HarnessItemKind, row.itemKey)
                  );
                }
              : undefined
          }
        />
      </div>
    </div>
  );
}

function KindBadge({ kind }: { kind: GuardRowKind }) {
  const label =
    kind === "command"
      ? "Command"
      : kind === "path"
        ? "Path"
        : "Advisory";
  return (
    <span className="rounded-md border border-neutral-200 bg-white px-2 py-0.5 text-[10px] font-medium text-neutral-700">
      {label}
    </span>
  );
}

function ModeBadge({ mode }: { mode: GuardRowMode }) {
  return (
    <span
      className={cn(
        "rounded-md px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide",
        mode === "enforced"
          ? "bg-red-50 text-red-700"
          : "bg-neutral-100 text-neutral-600"
      )}
    >
      {mode === "enforced" ? "Hard stop" : "Soft"}
    </span>
  );
}

type EvalRowKind = "check" | "agent_rule" | "source" | "rubric";

type EvalRow = {
  key: string;
  kind: EvalRowKind;
  content: string;
  itemKey: string;
};

function buildEvalRows(data: EvaluationsDoc): EvalRow[] {
  const rows: EvalRow[] = [];
  data.checks.forEach((c, i) => {
    rows.push({ key: `check-${i}-${c}`, kind: "check", content: c, itemKey: c });
  });
  data.agentRules.forEach((r, i) => {
    rows.push({ key: `rule-${i}`, kind: "agent_rule", content: r, itemKey: r });
  });
  if (data.judgeRubric) {
    rows.push({
      key: "rubric",
      kind: "rubric",
      content: data.judgeRubric,
      itemKey: "judge_rubric",
    });
  }
  data.sources.forEach((s, i) => {
    rows.push({ key: `src-${i}-${s}`, kind: "source", content: s, itemKey: s });
  });
  return rows;
}

type EvalColumnKey = "type" | "content";

const EVAL_COLUMNS: Columns<EvalColumnKey, EvalRow> = {
  type: {
    header: () => "Type",
    width: "8.5rem",
    cell: ({ row }) => <EvalKindBadge kind={row.kind} />,
  },
  content: {
    header: () => "Rule / check",
    width: "minmax(16rem, 1fr)",
    cell: ({ row }) => (
      <span
        className={cn(
          "line-clamp-3 text-sm text-neutral-800",
          (row.kind === "check" || row.kind === "source") && "font-mono text-xs"
        )}
      >
        {row.content}
      </span>
    ),
  },
};

const EVAL_VISIBILITY: Record<EvalColumnKey, boolean> = {
  type: true,
  content: true,
};

function EvaluationsDocument({
  data,
  toolbarAction,
  domain,
}: {
  data: EvaluationsDoc;
  toolbarAction?: ReactNode;
  domain?: "evaluations";
}) {
  const router = useRouter();
  const rows = useMemo(() => buildEvalRows(data), [data]);
  const [q, setQ] = useState("");
  const [kind, setKind] = useState<"all" | EvalRowKind>("all");

  const filtered = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return rows.filter((r) => {
      if (kind !== "all" && r.kind !== kind) return false;
      if (!needle) return true;
      return r.content.toLowerCase().includes(needle) || r.kind.includes(needle);
    });
  }, [rows, q, kind]);

  return (
    <div className="flex h-full min-h-0 w-full flex-col">
      <div className="flex shrink-0 flex-wrap items-center gap-3 border-b border-neutral-200 bg-white px-5 py-2.5">
        <input
          className="min-w-[14rem] flex-1 rounded-md border border-neutral-200 bg-white px-2.5 py-1.5 text-sm outline-none focus:border-neutral-400"
          placeholder="Filter checks, rules, sources…"
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
        <Dropdown
          size="sm"
          value={kind}
          onChange={(v) => setKind(v as typeof kind)}
          options={[
            { value: "all", label: "All types" },
            { value: "check", label: "Check" },
            { value: "agent_rule", label: "Agent rule" },
            { value: "rubric", label: "Judge rubric" },
            { value: "source", label: "Source" },
          ]}
        />
        {toolbarAction}
      </div>

      <div className="min-h-0 flex-1 overflow-auto px-5 py-4">
        <DataTable
          columns={EVAL_COLUMNS}
          data={filtered}
          isFetched
          isLoading={false}
          visibilityColumns={EVAL_VISIBILITY}
          emptyTitle="No evaluation rules match"
          emptyBody="Clear search or change type to see rows."
          onClick={
            domain
              ? (row: EvalRow) => {
                  router.push(
                    harnessItemHref(domain, row.kind as HarnessItemKind, row.itemKey)
                  );
                }
              : undefined
          }
        />
      </div>
    </div>
  );
}

function EvalKindBadge({ kind }: { kind: EvalRowKind }) {
  const label =
    kind === "check"
      ? "Check"
      : kind === "agent_rule"
        ? "Agent rule"
        : kind === "rubric"
          ? "Rubric"
          : "Source";
  return (
    <span className="rounded-md border border-neutral-200 bg-white px-2 py-0.5 text-[10px] font-medium text-neutral-700">
      {label}
    </span>
  );
}

function JsonDocument({ data }: { data: unknown }) {
  if (Array.isArray(data)) {
    return (
      <div className="mx-auto max-w-3xl space-y-3">
        {data.length === 0 && (
          <p className="text-sm text-neutral-500">Empty list.</p>
        )}
        {data.map((item, i) => (
          <div
            key={i}
            className="rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm"
          >
            {typeof item === "object" && item ? (
              <dl className="space-y-1.5">
                {Object.entries(item as Record<string, unknown>).map(([k, v]) => (
                  <div key={k} className="grid grid-cols-[7rem_1fr] gap-2">
                    <dt className="text-xs font-medium uppercase tracking-wide text-neutral-400">
                      {k}
                    </dt>
                    <dd className="min-w-0 break-words text-neutral-800">
                      {formatValue(v)}
                    </dd>
                  </div>
                ))}
              </dl>
            ) : (
              <span>{String(item)}</span>
            )}
          </div>
        ))}
      </div>
    );
  }
  if (typeof data === "object" && data) {
    return (
      <div className="mx-auto max-w-3xl space-y-3">
        <dl className="rounded-xl border border-neutral-200 bg-white px-4 py-3 space-y-2">
          {Object.entries(data as Record<string, unknown>).map(([k, v]) => (
            <div key={k} className="grid grid-cols-[8rem_1fr] gap-2 text-sm">
              <dt className="text-xs font-medium uppercase tracking-wide text-neutral-400">
                {k}
              </dt>
              <dd className="min-w-0 break-words text-neutral-800">{formatValue(v)}</dd>
            </div>
          ))}
        </dl>
      </div>
    );
  }
  return (
    <pre className="rounded-lg border border-neutral-200 bg-neutral-50 p-4 text-xs">
      {JSON.stringify(data, null, 2)}
    </pre>
  );
}

function formatValue(v: unknown): ReactNode {
  if (v == null) return "-";
  if (typeof v === "string") {
    if (v.startsWith("/sessions/") || /^[a-f0-9]{16,}$/i.test(v) || v.startsWith("ses_")) {
      return (
        <Link href={`/sessions/${encodeURIComponent(v)}`} className="text-teal-700 hover:underline">
          {v}
        </Link>
      );
    }
    return v;
  }
  if (typeof v === "number" || typeof v === "boolean") return String(v);
  if (Array.isArray(v)) {
    return (
      <ul className="list-disc pl-4">
        {v.map((x, i) => (
          <li key={i}>{formatValue(x)}</li>
        ))}
      </ul>
    );
  }
  return (
    <pre className="overflow-auto rounded bg-neutral-50 p-2 font-mono text-[11px]">
      {JSON.stringify(v, null, 2)}
    </pre>
  );
}

function SeverityBadge({ severity }: { severity: string }) {
  const s = severity.toLowerCase();
  return (
    <span
      className={cn(
        "rounded-md px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide",
        s === "block" || s === "deny"
          ? "bg-red-50 text-red-700"
          : s === "warn"
            ? "bg-amber-50 text-amber-800"
            : "bg-neutral-100 text-neutral-600"
      )}
    >
      {severity}
    </span>
  );
}
