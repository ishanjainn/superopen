import { fileExists, readText } from "./nodeio";
import { soPath } from "./root";

export type AuditEvent = {
  at: string;
  action: string;
  key?: string;
  type?: string;
  detail?: string;
  vendor?: string;
  session_id?: string;
};

export function listAuditEvents(limit = 100): AuditEvent[] {
  const p = soPath("audit", "events.jsonl");
  if (!fileExists(p)) return [];
  const lines = readText(p).split("\n").filter(Boolean);
  const out: AuditEvent[] = [];
  for (const line of lines) {
    try {
      out.push(JSON.parse(line) as AuditEvent);
    } catch {
      /* skip */
    }
  }
  out.reverse();
  return out.slice(0, limit);
}
