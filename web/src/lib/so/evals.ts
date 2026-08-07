import { readdirSync, statSync } from "fs";
import { fileExists, readText } from "./nodeio";
import { join } from "path";
import { soPath, soRoot } from "./root";
import { humanizePromptPreview, type SessionMeta } from "./sessions";
import { parseEvaluationsDoc } from "./harness-yaml";

export type EvalBadge = "good" | "ok" | "poor" | "warning" | "problem" | "failed" | string;

export type EvalRun = {
  id: string;
  session_id: string;
  at: string;
  score?: number;
  badge: EvalBadge;
  notes: string[];
  dimensions?: Record<string, number>;
  source: "history" | "eval.json" | "report.json";
  title?: string;
  vendor?: string;
  tokens?: number;
  cost_usd?: number;
  evidence_status?: "sufficient" | "insufficient";
  session_status?: string;
  scope: "complete" | "snapshot";
  /** Timeline / evidence points where things went wrong */
  failure_points: EvalFailurePoint[];
};

export type EvalFailurePoint = {
  label: string;
  severity: "warning" | "problem" | "info";
  /** Map/timeline seq when known */
  seq?: number;
  detail?: string;
};

export type EvalsDashboard = {
  summary: {
    total: number;
    executions: number;
    good: number;
    ok: number;
    poor: number;
    unknown: number;
    with_failures: number;
    avg_score: number | null;
    pass_rate: number | null;
    evaluator_count: number;
    total_tokens: number;
    total_cost_usd: number;
    avg_tokens: number | null;
  };
  /** Aggregate note / check failure reasons */
  failure_reasons: { reason: string; count: number }[];
  /** Daily series for charts */
  series: {
    date: string;
    runs: number;
    good: number;
    ok: number;
    poor: number;
    unknown: number;
    pass_rate: number | null;
    tokens: number;
    cost_usd: number;
  }[];
  /** Daily count of actual evaluation executions, including reruns. */
  execution_series: { date: string; runs: number }[];
  evaluation_target: {
    session_id: string;
    title: string;
    status: string;
    whole_chat_evaluated: boolean;
    last_eval_at?: string;
  } | null;
  /** Per configured check / dimension performance */
  evaluators: {
    id: string;
    label: string;
    kind: "check" | "dimension" | "agent_rule";
    executions: number;
    pass_rate: number | null;
    trend: number[];
  }[];
  runs: EvalRun[];
};

type StoredEval = {
  session_id?: string;
  at?: string;
  score?: number;
  badge?: string;
  notes?: string[];
  dimensions?: Record<string, number>;
  evidence_status?: "sufficient" | "insufficient";
};

type StoredReport = {
  session?: { id?: string };
  judge?: { generatedAt?: string; cli?: string };
  taskSummary?: string;
  narrative?: string;
  dimensions?: { name?: string; verdict?: string; findings?: Finding[] }[];
  rubric?: {
    tasks?: {
      title?: string;
      criteria?: {
        title?: string;
        verdict?: string;
        findings?: Finding[];
      }[];
    }[];
  };
};

type Finding = {
  claim?: string;
  severity?: string;
  evidenceSeqs?: number[];
};

function readJSON<T>(path: string): T | null {
  try {
    return JSON.parse(readText(path)) as T;
  } catch {
    return null;
  }
}

function readMeta(sessionId: string): SessionMeta | null {
  return readJSON<SessionMeta>(join(soPath("sessions"), sessionId, "meta.json"));
}

function sessionTitle(meta: SessionMeta | null, fallback: string): string {
  if (!meta) return fallback;
  return (
    humanizePromptPreview(meta.title) ||
    humanizePromptPreview(meta.prompt_preview) ||
    meta.id ||
    fallback
  );
}

function evaluationScope(
  meta: SessionMeta | null,
  evaluatedAt: string
): "complete" | "snapshot" {
  if (meta?.status !== "ended" || !meta.ended_at || !evaluatedAt) return "snapshot";
  const ended = new Date(meta.ended_at).getTime();
  const evaluated = new Date(evaluatedAt).getTime();
  return Number.isFinite(ended) && Number.isFinite(evaluated) && evaluated >= ended
    ? "complete"
    : "snapshot";
}

function evidenceStatus(
  explicit: StoredEval["evidence_status"],
  notes: string[]
): "sufficient" | "insufficient" {
  if (explicit) return explicit;
  return notes.some((note) =>
    /no (session |tool )?activity recorded|insufficient (activity telemetry|signal)/i.test(note)
  )
    ? "insufficient"
    : "sufficient";
}

function normalizeBadge(
  badge: string | undefined,
  score?: number,
  evidence: "sufficient" | "insufficient" = "sufficient"
): EvalBadge {
  if (evidence === "insufficient") return "unknown";
  const b = String(badge || "").toLowerCase();
  if (b === "good" || b === "ok" || b === "poor") return b;
  if (b === "problem" || b === "failed") return "poor";
  if (b === "warning") return "ok";
  if (typeof score === "number") {
    if (score >= 0.75) return "good";
    if (score >= 0.45) return "ok";
    return "poor";
  }
  return b || "ok";
}

function failurePointsFromEval(
  notes: string[],
  dimensions?: Record<string, number>,
  evidence: "sufficient" | "insufficient" = "sufficient"
): EvalFailurePoint[] {
  if (evidence === "insufficient") {
    return [{
      label: "Insufficient activity telemetry to evaluate this session.",
      severity: "info",
    }];
  }
  const out: EvalFailurePoint[] = [];
  for (const n of notes) {
    const t = String(n || "").trim();
    if (!t || t.startsWith("model:")) continue;
    out.push({
      label: t.length > 120 ? t.slice(0, 117) + "…" : t,
      severity: /fail|secret|danger|without reads|high search/i.test(t)
        ? "problem"
        : "warning",
    });
  }
  if (dimensions) {
    for (const [k, v] of Object.entries(dimensions)) {
      if (k === "wandering" && v >= 0.7) {
        out.push({
          label: `High wandering (${v.toFixed(2)})`,
          severity: "warning",
          detail: "dimension",
        });
      } else if (k !== "wandering" && v <= 0.35) {
        out.push({
          label: `Weak ${k} (${v.toFixed(2)})`,
          severity: "problem",
          detail: "dimension",
        });
      }
    }
  }
  return out;
}

function failurePointsFromReport(report: StoredReport): EvalFailurePoint[] {
  const out: EvalFailurePoint[] = [];
  for (const dim of report.dimensions || []) {
    for (const f of dim.findings || []) {
      const sev = String(f.severity || "").toLowerCase();
      if (sev !== "problem" && sev !== "warning") continue;
      const seqs = f.evidenceSeqs || [];
      out.push({
        label: f.claim || `${dim.name} ${sev}`,
        severity: sev === "problem" ? "problem" : "warning",
        seq: seqs[0],
        detail: dim.name,
      });
    }
    const v = String(dim.verdict || "").toLowerCase();
    if ((v === "problem" || v === "warning") && !(dim.findings || []).length) {
      out.push({
        label: `Dimension ${dim.name}: ${v}`,
        severity: v === "problem" ? "problem" : "warning",
        detail: dim.name,
      });
    }
  }
  for (const task of report.rubric?.tasks || []) {
    for (const c of task.criteria || []) {
      const v = String(c.verdict || "").toLowerCase();
      if (v !== "problem" && v !== "warning") continue;
      for (const f of c.findings || []) {
        const seqs = f.evidenceSeqs || [];
        out.push({
          label: `${task.title || "Task"} · ${c.title || f.claim || v}`,
          severity: v === "problem" ? "problem" : "warning",
          seq: seqs[0],
          detail: f.claim,
        });
      }
      if (!(c.findings || []).length) {
        out.push({
          label: `${task.title || "Task"} · ${c.title || v}`,
          severity: v === "problem" ? "problem" : "warning",
        });
      }
    }
  }
  return out;
}

function reportBadge(report: StoredReport, failures: EvalFailurePoint[]): EvalBadge {
  if (failures.some((f) => f.severity === "problem")) return "poor";
  if (failures.some((f) => f.severity === "warning")) return "ok";
  const dims = report.dimensions || [];
  if (dims.length && dims.every((d) => String(d.verdict).toLowerCase() === "good")) {
    return "good";
  }
  return "ok";
}

function loadHistoryRuns(): EvalRun[] {
  const path = soPath("evals", "history.json");
  if (!fileExists(path)) return [];
  const hist = readJSON<StoredEval[]>(path);
  if (!Array.isArray(hist)) return [];
  return hist
    .filter((r) => r && r.session_id)
    .map((r, i) => {
      const sid = String(r.session_id);
      const meta = readMeta(sid);
      const notes = Array.isArray(r.notes) ? r.notes.map(String) : [];
      const evidence = evidenceStatus(r.evidence_status, notes);
      const at = String(r.at || "");
      return {
        id: `history:${sid}:${r.at || i}`,
        session_id: sid,
        at,
        score: typeof r.score === "number" ? r.score : undefined,
        badge: normalizeBadge(r.badge, r.score, evidence),
        notes,
        dimensions: r.dimensions,
        source: "history" as const,
        title: sessionTitle(meta, sid),
        vendor: meta?.vendor,
        tokens: Number(meta?.tokens || 0) || undefined,
        cost_usd: Number(meta?.cost_usd || 0) || undefined,
        evidence_status: evidence,
        session_status: meta?.status,
        scope: evaluationScope(meta, at),
        failure_points: failurePointsFromEval(notes, r.dimensions, evidence),
      };
    });
}

function loadSessionArtifactRuns(): EvalRun[] {
  const dir = soPath("sessions");
  if (!fileExists(dir)) return [];
  const out: EvalRun[] = [];
  for (const name of readdirSync(dir)) {
    if (name === "index.json" || name.startsWith(".") || name.endsWith(".json")) continue;
    const sessionPath = join(dir, name);
    try {
      if (!statSync(sessionPath).isDirectory()) continue;
    } catch {
      continue;
    }
    const meta = readJSON<SessionMeta>(join(sessionPath, "meta.json"));
    const sid = String(meta?.id || name);

    const evalPath = join(sessionPath, "eval.json");
    if (fileExists(evalPath)) {
      const r = readJSON<StoredEval>(evalPath);
      if (r) {
        const notes = Array.isArray(r.notes) ? r.notes.map(String) : [];
        const evidence = evidenceStatus(r.evidence_status, notes);
        const at = String(r.at || "");
        out.push({
          id: `eval:${sid}`,
          session_id: sid,
          at,
          score: typeof r.score === "number" ? r.score : undefined,
          badge: normalizeBadge(
            r.badge || (meta?.eval_badge ? String(meta.eval_badge) : undefined),
            r.score,
            evidence
          ),
          notes,
          dimensions: r.dimensions,
          source: "eval.json",
          title: sessionTitle(meta, sid),
          vendor: meta?.vendor,
          tokens: Number(meta?.tokens || 0) || undefined,
          cost_usd: Number(meta?.cost_usd || 0) || undefined,
          evidence_status: evidence,
          session_status: meta?.status,
          scope: evaluationScope(meta, at),
          failure_points: failurePointsFromEval(notes, r.dimensions, evidence),
        });
      }
    }

    const reportPath = join(sessionPath, "report.json");
    if (fileExists(reportPath)) {
      const report = readJSON<StoredReport>(reportPath);
      if (report) {
        const failures = failurePointsFromReport(report);
        out.push({
          id: `report:${sid}`,
          session_id: sid,
          at: String(report.judge?.generatedAt || ""),
          badge: reportBadge(report, failures),
          notes: [
            report.taskSummary,
            report.narrative ? report.narrative.slice(0, 160) : "",
            report.judge?.cli ? `judge:${report.judge.cli}` : "",
          ].filter(Boolean) as string[],
          source: "report.json",
          evidence_status: "sufficient",
          session_status: meta?.status,
          scope: evaluationScope(meta, String(report.judge?.generatedAt || "")),
          title: sessionTitle(meta, sid),
          vendor: meta?.vendor,
          tokens: Number(meta?.tokens || 0) || undefined,
          cost_usd: Number(meta?.cost_usd || 0) || undefined,
          failure_points: failures,
        });
      }
    }
  }
  return out;
}

/** Keep every execution while also deriving the latest result per session. */
export function listEvalsDashboard(): EvalsDashboard {
  const historyRuns = loadHistoryRuns();
  const artifactRuns = loadSessionArtifactRuns();
  const bySession = new Map<string, EvalRun>();
  const rank = (s: EvalRun["source"]) =>
    s === "report.json" ? 3 : s === "eval.json" ? 2 : 1;

  for (const run of [...historyRuns, ...artifactRuns]) {
    const prev = bySession.get(run.session_id);
    if (!prev) {
      bySession.set(run.session_id, run);
      continue;
    }
    const newer =
      rank(run.source) > rank(prev.source) ||
      (rank(run.source) === rank(prev.source) &&
        String(run.at) > String(prev.at));
    if (newer) bySession.set(run.session_id, run);
  }

  const currentRuns = Array.from(bySession.values()).sort((a, b) =>
    String(b.at || "").localeCompare(String(a.at || ""))
  );

  // eval.json mirrors the latest history entry, so do not count that artifact
  // twice. Reports are separate judge executions and remain distinct.
  const executions = [...historyRuns];
  for (const artifact of artifactRuns) {
    const duplicatedHistory = artifact.source === "eval.json" && historyRuns.some(
      (run) => run.session_id === artifact.session_id && run.at === artifact.at
    );
    if (!duplicatedHistory) executions.push(artifact);
  }
  executions.sort((a, b) => String(b.at || "").localeCompare(String(a.at || "")));

  const summary = {
    total: currentRuns.length,
    executions: executions.length,
    good: 0,
    ok: 0,
    poor: 0,
    unknown: 0,
    with_failures: 0,
    avg_score: null as number | null,
    pass_rate: null as number | null,
    evaluator_count: 0,
    total_tokens: 0,
    total_cost_usd: 0,
    avg_tokens: null as number | null,
  };
  const reasonCount = new Map<string, number>();
  let scoreSum = 0;
  let scoreN = 0;
  let tokenN = 0;
  for (const r of currentRuns) {
    if (r.badge === "good") summary.good++;
    else if (r.badge === "ok") summary.ok++;
    else if (r.badge === "unknown") summary.unknown++;
    else summary.poor++;
    if (r.evidence_status !== "insufficient" && typeof r.score === "number") {
      scoreSum += r.score;
      scoreN++;
    }
    if (typeof r.tokens === "number" && r.tokens > 0) {
      summary.total_tokens += r.tokens;
      tokenN++;
    }
    if (typeof r.cost_usd === "number" && r.cost_usd > 0) {
      summary.total_cost_usd += r.cost_usd;
    }
    const failures = r.failure_points.filter((fp) => fp.severity !== "info");
    if (failures.length) {
      summary.with_failures++;
      for (const fp of failures) {
        reasonCount.set(fp.label, (reasonCount.get(fp.label) || 0) + 1);
      }
    }
  }
  if (scoreN) summary.avg_score = scoreSum / scoreN;
  const scored = summary.good + summary.ok + summary.poor;
  if (scored) summary.pass_rate = summary.good / scored;
  if (tokenN) summary.avg_tokens = Math.round(summary.total_tokens / tokenN);
  summary.total_cost_usd = Math.round(summary.total_cost_usd * 10000) / 10000;

  const failure_reasons = Array.from(reasonCount.entries())
    .map(([reason, count]) => ({ reason, count }))
    .sort((a, b) => b.count - a.count)
    .slice(0, 12);

  const series = buildDailySeries(currentRuns);
  const execution_series = buildDailySeries(executions).map(({ date, runs }) => ({ date, runs }));
  const evaluators = buildEvaluatorStats(executions);
  summary.evaluator_count = evaluators.length;

  const evaluation_target = latestEvaluationTarget(currentRuns);

  return {
    summary,
    failure_reasons,
    series,
    execution_series,
    evaluation_target,
    evaluators,
    runs: executions,
  };
}

function latestEvaluationTarget(currentRuns: EvalRun[]): EvalsDashboard["evaluation_target"] {
  const dir = soPath("sessions");
  if (!fileExists(dir)) return null;
  let latest: SessionMeta | null = null;
  for (const name of readdirSync(dir)) {
    if (name.startsWith(".") || name.endsWith(".json")) continue;
    const meta = readJSON<SessionMeta>(join(dir, name, "meta.json"));
    if (!meta) continue;
    if (!latest) {
      latest = meta;
      continue;
    }
    const candidateAt = String(meta.ended_at || meta.started_at || "");
    const latestAt = String(latest.ended_at || latest.started_at || "");
    if (candidateAt > latestAt) latest = meta;
  }
  if (!latest) return null;
  const latestRun = currentRuns.find((run) => run.session_id === latest?.id);
  return {
    session_id: latest.id,
    title: sessionTitle(latest, latest.id),
    status: String(latest.status || "active"),
    whole_chat_evaluated: latestRun?.scope === "complete",
    last_eval_at: latestRun?.at || undefined,
  };
}

function dayKey(iso: string): string {
  if (!iso) return "unknown";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso.slice(0, 10) || "unknown";
  return d.toISOString().slice(0, 10);
}

export function buildDailySeries(runs: EvalRun[]) {
  const map = new Map<
    string,
    {
      runs: number;
      good: number;
      ok: number;
      poor: number;
      unknown: number;
      tokens: number;
      cost_usd: number;
    }
  >();
  for (const r of runs) {
    const k = dayKey(r.at);
    const cur = map.get(k) || {
      runs: 0,
      good: 0,
      ok: 0,
      poor: 0,
      unknown: 0,
      tokens: 0,
      cost_usd: 0,
    };
    cur.runs++;
    if (r.badge === "good") cur.good++;
    else if (r.badge === "unknown") cur.unknown++;
    else cur.poor++;
    cur.tokens += r.tokens || 0;
    cur.cost_usd += r.cost_usd || 0;
    map.set(k, cur);
  }
  const populated = Array.from(map.entries())
    .filter(([d]) => d !== "unknown")
    .sort(([a], [b]) => a.localeCompare(b));
  if (!populated.length) return [];

  // Preserve calendar spacing as the dashboard accumulates history. Returning
  // only days that have runs makes a week-long gap look like adjacent points.
  // Fill the last 30 calendar days between the first and latest observed run;
  // pass rate stays null on idle days so the line shows a truthful gap.
  const latest = new Date(`${populated[populated.length - 1][0]}T00:00:00Z`);
  const earliestObserved = new Date(`${populated[0][0]}T00:00:00Z`);
  const windowStart = new Date(latest);
  windowStart.setUTCDate(windowStart.getUTCDate() - 29);
  const start = earliestObserved > windowStart ? earliestObserved : windowStart;
  const byDate = new Map(populated);
  const dense: EvalsDashboard["series"] = [];
  for (const cursor = new Date(start); cursor <= latest; cursor.setUTCDate(cursor.getUTCDate() + 1)) {
    const date = cursor.toISOString().slice(0, 10);
    const v = byDate.get(date);
    if (!v) {
      dense.push({
        date,
        runs: 0,
        good: 0,
        ok: 0,
        poor: 0,
        unknown: 0,
        pass_rate: null,
        tokens: 0,
        cost_usd: 0,
      });
      continue;
    }
    dense.push({
      date,
      ...v,
      cost_usd: Math.round(v.cost_usd * 10000) / 10000,
      pass_rate: v.good + v.ok + v.poor ? v.good / (v.good + v.ok + v.poor) : null,
    });
  }
  return dense;
}

function buildEvaluatorStats(runs: EvalRun[]) {
  type Acc = {
    id: string;
    label: string;
    kind: "check" | "dimension" | "agent_rule";
    executions: number;
    passes: number;
    trend: number[];
  };
  const byId = new Map<string, Acc>();

  const bump = (
    id: string,
    label: string,
    kind: Acc["kind"],
    passed: boolean,
    at: string
  ) => {
    let a = byId.get(id);
    if (!a) {
      a = { id, label, kind, executions: 0, passes: 0, trend: [] };
      byId.set(id, a);
    }
    a.executions++;
    if (passed) a.passes++;
    // crude chronological trend samples (pass=1 fail=0)
    a.trend.push(passed ? 1 : 0);
    if (a.trend.length > 12) a.trend = a.trend.slice(-12);
    void at;
  };

  // Configured checks from configs.yaml
  const cfgPath = evalsConfigPath();
  let checks: string[] = [];
  let agentRules: string[] = [];
  if (fileExists(cfgPath)) {
    try {
      const doc = parseEvaluationsDoc(readText(cfgPath));
      checks = doc?.checks || [];
      agentRules = doc?.agentRules || [];
    } catch {
      /* ignore */
    }
  }

  for (const check of checks) {
    if (!byId.has(`check:${check}`)) {
      byId.set(`check:${check}`, {
        id: `check:${check}`,
        label: check,
        kind: "check",
        executions: 0,
        passes: 0,
        trend: [],
      });
    }
  }
  for (const rule of agentRules) {
    const id = `agent_rule:${rule.slice(0, 48)}`;
    if (!byId.has(id)) {
      byId.set(id, {
        id,
        label: rule.length > 60 ? rule.slice(0, 57) + "…" : rule,
        kind: "agent_rule",
        executions: 0,
        passes: 0,
        trend: [],
      });
    }
  }

  for (const r of [...runs].reverse()) {
    if (r.evidence_status === "insufficient") continue;
    if (r.dimensions) {
      for (const [name, v] of Object.entries(r.dimensions)) {
        const passed = name === "wandering" ? v < 0.7 : v > 0.35;
        bump(`dim:${name}`, name, "dimension", passed, r.at);
      }
    }
    for (const check of checks) {
      const hit = r.notes.some((n) =>
        n.toLowerCase().includes(check.toLowerCase().replace(/_/g, " ")) ||
        n.toLowerCase().includes(check.toLowerCase())
      );
      const failed = r.failure_points.some((fp) =>
        fp.label.toLowerCase().includes(check.toLowerCase())
      );
      if (hit || failed) {
        bump(
          `check:${check}`,
          check,
          "check",
          !failed,
          r.at
        );
      }
    }
  }

  return Array.from(byId.values())
    .map((a) => ({
      id: a.id,
      label: a.label,
      kind: a.kind,
      executions: a.executions,
      pass_rate: a.executions ? a.passes / a.executions : null,
      trend: a.trend,
    }))
    .sort((a, b) => b.executions - a.executions || a.label.localeCompare(b.label));
}

function evalsConfigPath(): string {
  return join(soRoot(), "evals", "configs.yaml");
}
