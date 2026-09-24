import { NextRequest, NextResponse } from "next/server";
import { projectIdFromRequest, runWithProjectAsync } from "@/lib/so/workspace";
import { soJSON } from "@/lib/so/exec";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export type ScanFinding = {
  rule_id?: string;
  title?: string;
  severity?: string;
  posture?: string;
  session_id?: string;
  reason?: string;
};

export async function GET(req: NextRequest) {
  const project = projectIdFromRequest(req);
  const session = req.nextUrl.searchParams.get("session") || "";
  return runWithProjectAsync(project, async () => {
    const args = ["scan"];
    if (session) args.push("--session", session);
    const res = await soJSON<ScanFinding[]>(args);
    if (!res.ok) {
      return NextResponse.json({ error: res.error, items: [] }, { status: 200 });
    }
    const rows = Array.isArray(res.data) ? res.data : [];
    return NextResponse.json({ items: rows });
  });
}
