
import { fileExists, readText } from "./nodeio";
import { listAuditEvents, type AuditEvent } from "./audit";
import {
  parseGuardrailsDoc,
  type GuardrailsDoc,
} from "./harness-yaml";
import { soPath } from "./root";

export type GuardLogStatus = "denied" | "triggered" | "passed" | "skipped";

export type GuardrailsDashboard = {
  summary: {
    total_guards: number;
    hard_stops: number;
    advisory: number;
    denials: number;
    denials_7d: number;
    triggers: number;
    passes: number;
    skips: number;
  };
  series: { date: string; denials: number; triggers: number; passes: number }[];
  guards: {
    id: string;
    kind: "command" | "path" | "advisory";
    label: string;
    detail: string;
    triggers: number;
    denials: number;
    passes: number;
    last_at?: string;
    trend: number[];
  }[];
  logs: {
    at: string;
    status: GuardLogStatus;
    guard: string;
    detail: string;
    vendor?: string;
    session_id?: string;
  }[];
  config: GuardrailsDoc | null;
};

function dayKey(iso: string): string {
  if (!iso) return "unknown";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return String(iso).slice(0, 10) || "unknown";
  return d.toISOString().slice(0, 10);
}

function loadConfig(): GuardrailsDoc | null {
  const p = soPath("guardrails", "guardrails.yaml");
  if (!fileExists(p)) return null;
  try {
    return parseGuardrailsDoc(readText(p));
  } catch {
    return null;
  }
}

function classify(ev: AuditEvent): GuardLogStatus | null {
  if (ev.action === "deny" || (ev.type === "policy" && ev.action === "deny")) {
    return "denied";
  }
  if (ev.action === "allow" && (ev.type === "policy" || ev.type === "guardrail")) {
    return "passed";
  }
  // Soft policy checks that did not hard-deny (approval ceiling, etc.)
  if (ev.action === "conflict_skip" && ev.type === "approval") {
    return "skipped";
  }
  if (ev.action === "injection_blocked") {
    return "triggered";
  }
  return null;
}

export function listGuardrailsDashboard(): GuardrailsDashboard {
  const config = loadConfig();
  const events = listAuditEvents(2000);
  const rawLogs: GuardrailsDashboard["logs"] = [];
  for (const e of events) {
    const status = classify(e);
    if (!status) continue;
    rawLogs.push({
      at: e.at,
      status,
      guard: e.key || e.detail || "-",
      detail: e.detail || "",
      vendor: e.vendor,
      session_id: e.session_id,
    });
  }

  // Collapse soft approval mismatches into one row per detail with count.
  const skipGroups = new Map<string, GuardrailsDashboard["logs"][0] & { count?: number }>();
  const logs: GuardrailsDashboard["logs"] = [];
  for (const l of rawLogs) {
    if (l.status !== "skipped") {
      logs.push(l);
      continue;
    }
    const key = `${l.detail}|${l.session_id || ""}|${l.vendor || ""}`;
    const prev = skipGroups.get(key);
    if (prev) {
      prev.count = (prev.count || 1) + 1;
      prev.detail = `${l.detail} (×${prev.count})`;
      if (l.at > prev.at) prev.at = l.at;
    } else {
      skipGroups.set(key, { ...l, count: 1 });
    }
  }
  for (const g of skipGroups.values()) {
    logs.push(g);
  }
  logs.sort((a, b) => (a.at < b.at ? 1 : a.at > b.at ? -1 : 0));

  const denials = logs.filter((l) => l.status === "denied");
  const passes = logs.filter((l) => l.status === "passed");
  const skips = logs.filter((l) => l.status === "skipped");
  const triggers = logs.filter(
    (l) => l.status === "denied" || l.status === "triggered"
  );
  const weekAgo = Date.now() - 7 * 24 * 60 * 60 * 1000;
  const denials_7d = denials.filter((e) => {
    const t = new Date(e.at).getTime();
    return !Number.isNaN(t) && t >= weekAgo;
  }).length;

  const hard_stops =
    (config?.denied_commands.length || 0) + (config?.sensitive_paths.length || 0);
  const advisory = config?.rules.length || 0;

  const byDay = new Map<
    string,
    { denials: number; triggers: number; passes: number }
  >();
  for (const e of logs) {
    const k = dayKey(e.at);
    const cur = byDay.get(k) || { denials: 0, triggers: 0, passes: 0 };
    if (e.status === "denied") cur.denials++;
    if (e.status === "denied" || e.status === "triggered") cur.triggers++;
    if (e.status === "passed") cur.passes++;
    byDay.set(k, cur);
  }
  const series = Array.from(byDay.entries())
    .filter(([d]) => d !== "unknown")
    .sort(([a], [b]) => a.localeCompare(b))
    .slice(-30)
    .map(([date, n]) => ({ date, ...n }));

  const denialByKey = new Map<string, number>();
  const passByKey = new Map<string, number>();
  const triggerByKey = new Map<string, number>();
  const lastByKey = new Map<string, string>();
  const trendByKey = new Map<string, number[]>();
  for (const e of [...logs].reverse()) {
    const key = e.guard;
    if (e.status === "denied") {
      denialByKey.set(key, (denialByKey.get(key) || 0) + 1);
    }
    if (e.status === "passed") {
      passByKey.set(key, (passByKey.get(key) || 0) + 1);
    }
    if (e.status === "denied" || e.status === "triggered") {
      triggerByKey.set(key, (triggerByKey.get(key) || 0) + 1);
      const trend = trendByKey.get(key) || [];
      trend.push(1);
      trendByKey.set(key, trend.slice(-12));
    }
    if (!lastByKey.has(key) || String(e.at) > String(lastByKey.get(key))) {
      lastByKey.set(key, e.at);
    }
  }

  const matchCount = (label: string, map: Map<string, number>) =>
    map.get(label) ||
    [...map.entries()]
      .filter(([k]) => k.includes(label) || label.includes(k))
      .reduce((s, [, n]) => s + n, 0);

  const guards: GuardrailsDashboard["guards"] = [];
  for (const c of config?.denied_commands || []) {
    guards.push({
      id: `command:${c}`,
      kind: "command",
      label: c,
      detail: "Denied at PreToolUse / beforeShell",
      triggers: matchCount(c, triggerByKey),
      denials: matchCount(c, denialByKey),
      passes: matchCount(c, passByKey),
      last_at: lastByKey.get(c),
      trend: trendByKey.get(c) || [],
    });
  }
  for (const p of config?.sensitive_paths || []) {
    guards.push({
      id: `path:${p}`,
      kind: "path",
      label: p,
      detail: "Denied at beforeRead / file hooks",
      triggers: matchCount(p, triggerByKey),
      denials: matchCount(p, denialByKey),
      passes: matchCount(p, passByKey),
      last_at: lastByKey.get(p),
      trend: trendByKey.get(p) || [],
    });
  }
  for (const r of config?.rules || []) {
    guards.push({
      id: `advisory:${r.id}`,
      kind: "advisory",
      label: r.id,
      detail: r.description,
      triggers: matchCount(r.id, triggerByKey),
      denials: matchCount(r.id, denialByKey),
      passes: matchCount(r.id, passByKey),
      last_at: lastByKey.get(r.id),
      trend: trendByKey.get(r.id) || [],
    });
  }

  return {
    summary: {
      total_guards: guards.length,
      hard_stops,
      advisory,
      denials: denials.length,
      denials_7d,
      triggers: triggers.length,
      passes: passes.length,
      skips: skips.length,
    },
    series,
    guards,
    logs: logs.slice(0, 100),
    config,
  };
}
