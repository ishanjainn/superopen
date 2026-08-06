import { NextRequest, NextResponse } from "next/server";
import { buildAgentTrace } from "@/lib/so/agents";
import { resolveMapSession } from "@/lib/so/trace";
import { projectIdFromRequest, runWithProject } from "@/lib/so/workspace";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(
  req: NextRequest,
  ctx: { params: Promise<{ key: string; id: string }> | { key: string; id: string } }
) {
  const params = await Promise.resolve(ctx.params);
  const key = decodeURIComponent(params.key);
  const id = decodeURIComponent(params.id);
  const project = projectIdFromRequest(req);
  return runWithProject(project, () => {
    if (!resolveMapSession(key)) {
      return NextResponse.json({ error: `session "${key}" not found` }, { status: 404 });
    }
    const trace = buildAgentTrace(key, id);
    if (!trace) {
      return NextResponse.json({ error: `agent "${id}" not found` }, { status: 404 });
    }
    return NextResponse.json(trace);
  });
}
