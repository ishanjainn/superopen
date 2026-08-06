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
    good: number;
    ok: number;
    poor: number;
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
    pass_rate: number;
    tokens: number;
    cost_usd: number;
  }[];
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

function normalizeBadge(badge: string | undefined, score?: number): EvalBadge {
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
  dimensions?: Record<string, number>
): EvalFailurePoint[] {
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
      return {
        id: `history:${sid}:${r.at || i}`,
        session_id: sid,
        at: String(r.at || ""),
        score: typeof r.score === "number" ? r.score : undefined,
        badge: normalizeBadge(r.badge, r.score),
        notes,
        dimensions: r.dimensions,
        source: "history" as const,
        title: sessionTitle(meta, sid),
        vendor: meta?.vendor,
        tokens: Number(meta?.tokens || 0) || undefined,
        cost_usd: Number(meta?.cost_usd || 0) || undefined,
        failure_points: failurePointsFromEval(notes, r.dimensions),
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
        out.push({
          id: `eval:${sid}`,
          session_id: sid,
          at: String(r.at || ""),
          score: typeof r.score === "number" ? r.score : undefined,
          badge: normalizeBadge(
            r.badge || (meta?.eval_badge ? String(meta.eval_badge) : undefined),
            r.score
          ),
          notes,
          dimensions: r.dimensions,
          source: "eval.json",
          title: sessionTitle(meta, sid),
          vendor: meta?.vendor,
          tokens: Number(meta?.tokens || 0) || undefined,
          cost_usd: Number(meta?.cost_usd || 0) || undefined,
          failure_points: failurePointsFromEval(notes, r.dimensions),
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

/** Prefer newest artifact per session (report > eval.json > history). */
export function listEvalsDashboard(): EvalsDashboard {
  const bySession = new Map<string, EvalRun>();
  const rank = (s: EvalRun["source"]) =>
    s === "report.json" ? 3 : s === "eval.json" ? 2 : 1;

  for (const run of [...loadHistoryRuns(), ...loadSessionArtifactRuns()]) {
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

  const runs = Array.from(bySession.values()).sort((a, b) =>
    String(b.at || "").localeCompare(String(a.at || ""))
  );

  const summary = {
    total: runs.length,
    good: 0,
    ok: 0,
    poor: 0,
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
  for (const r of runs) {
    if (r.badge === "good") summary.good++;
    else if (r.badge === "ok") summary.ok++;
    else summary.poor++;
    if (typeof r.score === "number") {
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
    if (r.failure_points.length) {
      summary.with_failures++;
      for (const fp of r.failure_points) {
        reasonCount.set(fp.label, (reasonCount.get(fp.label) || 0) + 1);
      }
    }
  }
  if (scoreN) summary.avg_score = scoreSum / scoreN;
  if (summary.total) summary.pass_rate = summary.good / summary.total;
  if (tokenN) summary.avg_tokens = Math.round(summary.total_tokens / tokenN);
  summary.total_cost_usd = Math.round(summary.total_cost_usd * 10000) / 10000;

  const failure_reasons = Array.from(reasonCount.entries())
    .map(([reason, count]) => ({ reason, count }))
    .sort((a, b) => b.count - a.count)
    .slice(0, 12);

  const series = buildDailySeries(runs);
  const evaluators = buildEvaluatorStats(runs);
  summary.evaluator_count = evaluators.length;

  return { summary, failure_reasons, series, evaluators, runs };
}

function dayKey(iso: string): string {
  if (!iso) return "unknown";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso.slice(0, 10) || "unknown";
  return d.toISOString().slice(0, 10);
}

function buildDailySeries(runs: EvalRun[]) {
  const map = new Map<
    string,
    {
      runs: number;
      good: number;
      ok: number;
      poor: number;
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
      tokens: 0,
      cost_usd: 0,
    };
    cur.runs++;
    if (r.badge === "good") cur.good++;
    else if (r.badge === "ok") cur.ok++;
    else cur.poor++;
    cur.tokens += r.tokens || 0;
    cur.cost_usd += r.cost_usd || 0;
    map.set(k, cur);
  }
  return Array.from(map.entries())
    .filter(([d]) => d !== "unknown")
    .sort(([a], [b]) => a.localeCompare(b))
    .slice(-30)
    .map(([date, v]) => ({
      date,
      ...v,
      cost_usd: Math.round(v.cost_usd * 10000) / 10000,
      pass_rate: v.runs ? v.good / v.runs : 0,
    }));
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
      if (hit || failed || r.source === "eval.json") {
        bump(
          `check:${check}`,
          check,
          "check",
          !failed && r.badge !== "poor",
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
