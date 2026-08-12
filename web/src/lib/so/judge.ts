import { readText, writeText } from "./nodeio";
import { spawnSync } from "child_process";
import { soPath } from "./root";
import type {
  JudgeChoice,
  Report,
  ReportDimension,
  ReportStatus,
  Severity,
  Verdict,
} from "@/map/types";
import type { Trace as LibTrace } from "./trace";

// Judge accepts the lib Trace shape (stats as Record) from buildTrace.
type Trace = LibTrace;

const JUDGE_CLIS = ["claude", "codex"] as const;
const PROMPT_VERSION = 1;

type RunningJob = {
  startedAt: number;
  promise: Promise<void>;
};

const running = new Map<string, RunningJob>();
const failures = new Map<string, string>();

type SessionReviewDocument = {
  report?: Report;
  report_error?: string;
  [key: string]: unknown;
};

function sessionDocument(sessionId: string): SessionReviewDocument | null {
  try {
    return JSON.parse(readText(soPath("sessions", sessionId, "session.json"))) as SessionReviewDocument;
  } catch {
    return null;
  }
}

function writeSessionReport(sessionId: string, report?: Report, error?: string): void {
  const path = soPath("sessions", sessionId, "session.json");
  const doc = sessionDocument(sessionId) || {
    _about: {
      purpose: "Materialized state, summary, footprint, review, recommendations, and replay metadata for one coding session.",
      authority: "authoritative session state derived from events.jsonl",
      updated_by: "session materializer and review worker",
    },
    id: sessionId,
  };
  if (report) doc.report = report;
  else delete doc.report;
  if (error) doc.report_error = error;
  else delete doc.report_error;
  writeText(path, JSON.stringify(doc, null, 2));
}

function which(cli: string): boolean {
  const r = spawnSync("which", [cli], { encoding: "utf8" });
  return r.status === 0 && Boolean(r.stdout.trim());
}

function availableJudgeClis(): string[] {
  return JUDGE_CLIS.filter((cli) => which(cli));
}

function digestTrace(trace: Trace): string {
  const lines: string[] = [];
  lines.push(`Session ${trace.session.id} (${trace.session.harness})`);
  if (trace.session.title) lines.push(`Title: ${trace.session.title}`);
  if (trace.session.model) lines.push(`Model: ${trace.session.model}`);
  lines.push(`Events: ${trace.events.length}`);
  lines.push("");
  lines.push("## Timeline digests");
  for (const ev of trace.events.slice(0, 200)) {
    const paths = ev.targets.map((t) => `${t.touch}:${t.path}`).join(", ");
    lines.push(
      `#${ev.seq + 1} ${ev.action} ${ev.tool}${ev.isError ? " ERROR" : ""} ${paths || ev.summary}`
    );
  }
  if (trace.events.length > 200) lines.push(`… ${trace.events.length - 200} more events`);
  const userMarks = trace.marks.filter((m) => m.type === "user-message");
  if (userMarks.length) {
    lines.push("");
    lines.push("## User turns");
    for (const m of userMarks.slice(0, 20)) {
      lines.push(`@${m.seq + 1}: ${m.note || "(user message)"}`);
    }
  }
  return lines.join("\n");
}

function heuristicReport(trace: Trace, cli: string, model: string): Report {
  const stats = trace.stats as {
    edited?: number;
    fovea?: number;
    parafovea?: number;
    filesInRepo?: number;
    errorRate?: number;
    churnFiles?: number;
    actions?: { edit?: number; verify?: number; read?: number; search?: number };
  };
  const edited = stats.edited ?? 0;
  const seen = (stats.fovea ?? 0) + (stats.parafovea ?? 0);
  const files = stats.filesInRepo ?? 1;
  const footprint = seen / Math.max(1, files);

  const dim = (
    name: ReportDimension["name"],
    verdict: Verdict,
    claim: string,
    severity: Severity,
    seqs?: number[]
  ): ReportDimension => ({
    name,
    verdict,
    findings: [{ claim, severity, evidenceSeqs: seqs }],
  });

  const firstEdit = trace.events.findIndex((e) => e.action === "edit");
  const exploration: ReportDimension = dim(
    "exploration",
    firstEdit > 8 || firstEdit < 0 ? "good" : firstEdit < 3 ? "warning" : "good",
    firstEdit < 0
      ? "No edits in this trace."
      : `First edit at step #${firstEdit + 1} after ${firstEdit} prior events.`,
    firstEdit >= 0 && firstEdit < 3 ? "warning" : "info",
    firstEdit >= 0 ? [firstEdit] : undefined
  );

  const scope: ReportDimension = dim(
    "scope",
    footprint < 0.35 ? "good" : footprint < 0.6 ? "warning" : "problem",
    `Touched ${seen} of ${files} mapped files (${Math.round(footprint * 100)}% footprint).`,
    footprint < 0.6 ? "info" : "warning"
  );

  const churn = stats.churnFiles ?? 0;
  const wandering: ReportDimension = dim(
    "wandering",
    churn === 0 ? "good" : churn < 3 ? "warning" : "problem",
    churn === 0
      ? "No high-churn files (3+ edits)."
      : `${churn} file(s) were edited three or more times.`,
    churn ? "warning" : "info"
  );

  const verifies = stats.actions?.verify ?? 0;
  const edits = stats.actions?.edit ?? edited;
  const verification: ReportDimension = dim(
    "verification",
    verifies > 0 || edits === 0 ? "good" : "warning",
    verifies > 0
      ? `Saw ${verifies} verify action(s) alongside ${edits} edit(s).`
      : edits === 0
        ? "No edits to verify."
        : "Edits present but no verify actions were recorded in the trace.",
    verifies > 0 || edits === 0 ? "info" : "warning"
  );

  const dimensions = [exploration, scope, wandering, verification];
  const worst = dimensions.some((d) => d.verdict === "problem")
    ? "problem"
    : dimensions.some((d) => d.verdict === "warning")
      ? "warning"
      : "good";

  return {
    version: 1,
    session: {
      id: trace.session.id,
      harness: trace.session.harness,
      model: trace.session.model,
      eventCount: trace.events.length,
    },
    judge: {
      cli,
      model: model || undefined,
      promptVersion: PROMPT_VERSION,
      generatedAt: new Date().toISOString(),
    },
    taskSummary: trace.session.title || "Session evaluation",
    dimensions,
    narrative:
      worst === "good"
        ? "Heuristic pass: exploration, scope, and verification look reasonable from the footprint."
        : "Heuristic pass: some process signals need attention - open findings for timeline jumps.",
    notableMoments: trace.marks
      .filter((m) => m.type === "user-message" || m.type === "subagent")
      .slice(0, 8)
      .map((m) => ({ seq: m.seq, note: m.note || m.type })),
  };
}

function extractJSON(raw: string): unknown {
  const fenced = raw.match(/```(?:json)?\s*([\s\S]*?)```/);
  const text = fenced ? fenced[1] : raw;
  const start = text.indexOf("{");
  const end = text.lastIndexOf("}");
  if (start < 0 || end <= start) throw new Error("judge returned no JSON object");
  return JSON.parse(text.slice(start, end + 1));
}

function normalizeReport(raw: unknown, trace: Trace, cli: string, model: string): Report {
  const obj = raw as Partial<Report>;
  if (!obj.dimensions || !Array.isArray(obj.dimensions)) {
    throw new Error("judge JSON missing dimensions");
  }
  return {
    version: 1,
    session: {
      id: trace.session.id,
      harness: trace.session.harness,
      model: trace.session.model,
      eventCount: trace.events.length,
    },
    judge: {
      cli,
      model: (obj.judge as { model?: string } | undefined)?.model || model || undefined,
      promptVersion: PROMPT_VERSION,
      generatedAt: new Date().toISOString(),
    },
    taskSummary: String(obj.taskSummary || trace.session.title || "Session evaluation"),
    dimensions: obj.dimensions as ReportDimension[],
    rubric: obj.rubric,
    notableMoments: obj.notableMoments,
    narrative: String(obj.narrative || ""),
  };
}

function judgePrompt(trace: Trace): string {
  return `You are evaluating a coding-agent session. Return ONLY a JSON object with this shape:
{
  "taskSummary": string,
  "narrative": string,
  "dimensions": [
    {
      "name": "exploration" | "scope" | "wandering" | "verification",
      "verdict": "good" | "warning" | "problem" | "insufficient-data",
      "findings": [{ "claim": string, "severity": "info" | "warning" | "problem", "evidenceSeqs": number[] }]
    }
  ],
  "notableMoments": [{ "seq": number, "note": string }]
}

Rules:
- Verdicts must be grounded in the timeline digests (0-based seqs in evidenceSeqs).
- Cover all four dimensions.
- Be concise.

SESSION DIGEST:
${digestTrace(trace)}
`;
}

function runClaude(prompt: string, model: string): string {
  const args = ["-p", "--output-format", "json", "--dangerously-skip-permissions"];
  if (model) args.push("--model", model);
  args.push(prompt);
  const r = spawnSync("claude", args, {
    encoding: "utf8",
    maxBuffer: 8 * 1024 * 1024,
    timeout: 180_000,
    env: { ...process.env, CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: "1" },
  });
  if (r.error) throw r.error;
  if (r.status !== 0) throw new Error(r.stderr || `claude exited ${r.status}`);
  try {
    const envelope = JSON.parse(r.stdout) as { result?: string; content?: string };
    return String(envelope.result || envelope.content || r.stdout);
  } catch {
    return r.stdout;
  }
}

function runCodex(prompt: string, model: string, workdir: string): string {
  const args = [
    "exec",
    "--skip-git-repo-check",
    "--sandbox",
    "read-only",
    "--color",
    "never",
  ];
  if (model) args.push("-m", model);
  args.push("-");
  const r = spawnSync("codex", args, {
    encoding: "utf8",
    input: prompt,
    cwd: workdir,
    maxBuffer: 8 * 1024 * 1024,
    timeout: 180_000,
  });
  if (r.error) throw r.error;
  if (r.status !== 0) throw new Error(r.stderr || `codex exited ${r.status}`);
  return r.stdout || r.stderr;
}

async function runJudge(trace: Trace, choice: JudgeChoice): Promise<Report> {
  const clis = availableJudgeClis();
  const cli = clis.includes(choice.cli) ? choice.cli : clis[0];
  if (!cli) {
    return heuristicReport(trace, "heuristic", "");
  }
  const prompt = judgePrompt(trace);
  const workdir = soPath("sessions", trace.session.id);
  try {
    const raw =
      cli === "codex"
        ? runCodex(prompt, choice.model, workdir)
        : runClaude(prompt, choice.model);
    return normalizeReport(extractJSON(raw), trace, cli, choice.model);
  } catch (err) {
    // Fall back so the panel still shows something useful
    const base = heuristicReport(trace, cli, choice.model);
    base.narrative = `Judge CLI failed (${err instanceof Error ? err.message : String(err)}). Showing heuristic dimensions instead.`;
    return base;
  }
}

export function getReportStatus(sessionId: string, eventCount: number): ReportStatus {
  const clis = availableJudgeClis();
  const job = running.get(sessionId);
  if (job) {
    return {
      state: "running",
      stale: false,
      judgeAvailable: clis.length > 0,
      judgeCli: clis[0],
      judgeClis: clis,
    };
  }
	const doc = sessionDocument(sessionId);
	const error = failures.get(sessionId) || doc?.report_error;
	if (error && !doc?.report) {
    return {
      state: "failed",
      stale: false,
		error,
      judgeAvailable: clis.length > 0 || true,
      judgeCli: clis[0],
      judgeClis: clis.length ? clis : ["heuristic"],
    };
  }
	const report = doc?.report;
  if (!report) {
    return {
      state: "none",
      stale: false,
      judgeAvailable: true,
      judgeCli: clis[0] || "heuristic",
      judgeClis: clis.length ? clis : ["heuristic"],
    };
  }
  const stale = report.session.eventCount < eventCount;
  return {
    state: "done",
    stale,
    report,
    judgeAvailable: true,
    judgeCli: clis[0] || report.judge.cli,
    judgeClis: clis.length ? clis : [report.judge.cli, "heuristic"].filter(Boolean),
  };
}

export function startAnalyze(
  sessionId: string,
  trace: Trace,
  choice: JudgeChoice
): ReportStatus {
  if (running.has(sessionId)) {
    return getReportStatus(sessionId, trace.events.length);
  }
	failures.delete(sessionId);

  const promise = (async () => {
    try {
      const report = await runJudge(trace, choice);
		writeSessionReport(sessionId, report);
	} catch (err) {
		const message = err instanceof Error ? err.message : String(err);
		failures.set(sessionId, message);
		writeSessionReport(sessionId, undefined, message);
    } finally {
      running.delete(sessionId);
    }
  })();

  running.set(sessionId, { startedAt: Date.now(), promise });
  // Detach; API returns 202 immediately
  void promise;
  return {
    state: "running",
    stale: false,
    judgeAvailable: true,
    judgeCli: choice.cli || availableJudgeClis()[0] || "heuristic",
    judgeClis: availableJudgeClis().length
      ? availableJudgeClis()
      : ["heuristic"],
  };
}

/** Keep process spawning sync-only for sealed judge calls. */
