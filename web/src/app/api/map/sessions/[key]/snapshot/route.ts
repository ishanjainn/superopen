import { NextRequest, NextResponse } from "next/server";
import { getSessionMap } from "@/lib/so/session_map";
import { buildTrace, resolveMapSession } from "@/lib/so/trace";
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
    const session = resolveMapSession(key);
    if (!session) {
      return NextResponse.json(
        { error: `session "${key}" not found` },
        { status: 404 }
      );
    }
    const sessionMap = getSessionMap();
    const trace = buildTrace(session, sessionMap);
    return NextResponse.json({ trace, sessionMap });
  });
}
