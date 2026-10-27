import { NextRequest, NextResponse } from "next/server";
import { projectIdFromRequest, runWithProjectAsync } from "@/lib/so/workspace";
import { soJSON } from "@/lib/so/exec";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

/** A detection rule as `so scan rules list --json` returns it. */
export type ScanRule = {
  id: string;
  title: string;
  description?: string;
  severity: string;
  status: string;
  posture: string;
  /** baseline (built in), store (custom), or override (custom replacing a built-in). */
  source: "baseline" | "store" | "override";
  enabled: boolean;
  yaml: string;
};

export async function GET(req: NextRequest) {
  const project = projectIdFromRequest(req);
  return runWithProjectAsync(project, async () => {
    const res = await soJSON<ScanRule[]>(["scan", "rules", "list"]);
    if (!res.ok) return NextResponse.json({ error: res.error, items: [] });
    return NextResponse.json({ items: Array.isArray(res.data) ? res.data : [] });
  });
}

/** Create a rule, or replace the one with the same id, from its YAML. */
export async function PUT(req: NextRequest) {
  const project = projectIdFromRequest(req);
  const body = (await req.json().catch(() => ({}))) as { yaml?: string };
  if (!body.yaml || !body.yaml.trim()) {
    return NextResponse.json({ error: "yaml is required" }, { status: 400 });
  }
  return runWithProjectAsync(project, async () => {
    const res = await soJSON<ScanRule>(["scan", "rules", "set", "-"], { stdin: body.yaml });
    if (!res.ok) return NextResponse.json({ error: res.error }, { status: 400 });
    return NextResponse.json({ rule: res.data });
  });
}
