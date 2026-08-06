import type {
  AgentGraph,
  CityMap,
  JudgeChoice,
  ReportStatus,
  SessionMeta,
  Trace,
} from "./types";

/** Always talk to Next map routes. */
function apiURL(path: string): string {
  const p = path.startsWith("/") ? path : `/${path}`;
  if (p.startsWith("/api/")) return `/api/map${p.slice(4)}`;
  return `/api/map${p}`;
}

async function getJSON<T>(url: string): Promise<T> {
  const res = await fetch(apiURL(url));
  if (!res.ok) {
    const detail = (await res.text()).trim();
    throw new Error(detail || `${res.status} ${res.statusText}`);
  }
  return res.json() as Promise<T>;
}

async function postJSON<T>(url: string, body: unknown): Promise<T> {
  const res = await fetch(apiURL(url), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body ?? {}),
  });
  if (!res.ok && res.status !== 202) {
    const detail = (await res.text()).trim();
    throw new Error(detail || `${res.status} ${res.statusText}`);
  }
  return res.json() as Promise<T>;
}

export function describeError(err: unknown, doing: string): string {
  if (err instanceof TypeError) {
    return `Can't reach the map server while ${doing} - is \`so dev\` running?`;
  }
  const detail = (err instanceof Error ? err.message : String(err)).trim();
  return detail ? `Couldn't finish ${doing}: ${detail}` : `Couldn't finish ${doing}`;
}

export function listSessions(fresh = false): Promise<SessionMeta[]> {
  return getJSON<SessionMeta[]>(fresh ? "/api/sessions?fresh=1" : "/api/sessions");
}

export function getSessionSnapshot(key: string): Promise<{ trace: Trace; city: CityMap }> {
  return getJSON<{ trace: Trace; city: CityMap }>(
    `/api/sessions/${encodeURIComponent(key)}/snapshot`
  );
}

export function getSessionAgents(key: string): Promise<AgentGraph> {
  return getJSON<AgentGraph>(`/api/sessions/${encodeURIComponent(key)}/agents`);
}

export function getAgentTrace(key: string, agentId: string): Promise<Trace> {
  return getJSON<Trace>(
    `/api/sessions/${encodeURIComponent(key)}/agents/${encodeURIComponent(agentId)}/trace`
  );
}

export function getSessionReport(key: string): Promise<ReportStatus> {
  return getJSON<ReportStatus>(`/api/sessions/${encodeURIComponent(key)}/report`);
}

export function startSessionAnalyze(key: string, choice: JudgeChoice): Promise<ReportStatus> {
  return postJSON<ReportStatus>(`/api/sessions/${encodeURIComponent(key)}/analyze`, choice);
}

/** Resolve a Superopen session id (or map key) to the map session key. */
export async function resolveSessionKey(selector: string): Promise<string> {
  const data = await listSessions(true);
  const exact = data.find((s) => s.key === selector);
  if (exact) return exact.key;
  const byId = data.filter((s) => s.id === selector);
  if (byId.length === 1) return byId[0].key;
  if (byId.length > 1) {
    byId.sort((a, b) => String(b.startedAt || "").localeCompare(String(a.startedAt || "")));
    return byId[0].key;
  }
  throw new Error(`session "${selector}" not found`);
}
