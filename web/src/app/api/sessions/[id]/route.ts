import { NextRequest, NextResponse } from "next/server";
import { getSessionDetail } from "@/lib/so/sessions";
import { projectIdFromRequest, runWithProject } from "@/lib/so/workspace";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(
  req: NextRequest,
  ctx: { params: Promise<{ id: string }> | { id: string } }
) {
  const params = await Promise.resolve(ctx.params);
  const id = decodeURIComponent(params.id);
  const project = projectIdFromRequest(req);
  const detail = runWithProject(project, () => getSessionDetail(id, project));
  if (!detail) {
    return NextResponse.json({ error: "session not found" }, { status: 404 });
  }
  return NextResponse.json(detail);
}
