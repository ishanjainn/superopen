import { NextRequest, NextResponse } from "next/server";
import { projectIdFromRequest, runWithProjectAsync } from "@/lib/so/workspace";
import { soJSON } from "@/lib/so/exec";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function POST(
  req: NextRequest,
  ctx: { params: Promise<{ id: string }> },
) {
  const { id } = await ctx.params;
  const project = projectIdFromRequest(req);
  return runWithProjectAsync(project, async () => {
    const res = await soJSON<unknown>(["harvest", "decline", id]);
    if (!res.ok) {
      return NextResponse.json({ error: res.error }, { status: 400 });
    }
    return NextResponse.json(res.data ?? { ok: true });
  });
}
