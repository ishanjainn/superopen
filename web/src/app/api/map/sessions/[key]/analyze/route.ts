import { NextRequest, NextResponse } from "next/server";
import { getCityMap } from "@/lib/so/citymap";
import { startAnalyze } from "@/lib/so/judge";
import { buildTrace, resolveMapSession } from "@/lib/so/trace";
import { projectIdFromRequest, runWithProject } from "@/lib/so/workspace";
import type { JudgeChoice } from "@/map/types";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function POST(
  req: NextRequest,
  ctx: { params: Promise<{ key: string }> | { key: string } }
) {
  const params = await Promise.resolve(ctx.params);
  const key = decodeURIComponent(params.key);
  const project = projectIdFromRequest(req);
  let body: Partial<JudgeChoice> = {};
  try {
    body = (await req.json()) as Partial<JudgeChoice>;
  } catch {
    /* empty body ok */
  }
  return runWithProject(project, () => {
    const session = resolveMapSession(key);
    if (!session) {
      return NextResponse.json({ error: `session "${key}" not found` }, { status: 404 });
    }
    const trace = buildTrace(session, getCityMap());
    const status = startAnalyze(session.id, trace, {
      cli: typeof body.cli === "string" ? body.cli : "",
      model: typeof body.model === "string" ? body.model : "",
    });
    return NextResponse.json(status, { status: 202 });
  });
}
