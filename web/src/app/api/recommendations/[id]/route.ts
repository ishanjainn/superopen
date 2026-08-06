import { NextRequest, NextResponse } from "next/server";
import { getRecommendation } from "@/lib/so/misc";
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
  return runWithProject(project, () => {
    const rec = getRecommendation(id);
    if (!rec) {
      return NextResponse.json({ error: "not found" }, { status: 404 });
    }
    return NextResponse.json(rec);
  });
}
