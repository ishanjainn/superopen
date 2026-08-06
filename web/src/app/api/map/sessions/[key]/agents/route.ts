import { NextRequest, NextResponse } from "next/server";
import { buildAgentGraph } from "@/lib/so/agents";
import { resolveMapSession } from "@/lib/so/trace";
import { projectIdFromRequest, runWithProject } from "@/lib/so/workspace";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(
  req: NextRequest,
  ctx: { params: Promise<{ key: string }> | { key: string } }
) {
  const params = await Promise.resolve(ctx.params);
  const key = decodeURIComponent(params.key);
  const project = projectIdFromRequest(req);
  return runWithProject(project, () => {
    if (!resolveMapSession(key)) {
      return NextResponse.json({ error: `session "${key}" not found` }, { status: 404 });
    }
    const graph = buildAgentGraph(key);
    if (!graph) {
      return NextResponse.json({ error: `session "${key}" not found` }, { status: 404 });
    }
    return NextResponse.json(graph);
  });
}
