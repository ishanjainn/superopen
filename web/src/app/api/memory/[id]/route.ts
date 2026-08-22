import { NextRequest, NextResponse } from "next/server";
import { projectIdFromRequest, runWithProjectAsync } from "@/lib/so/workspace";
import { soJSON } from "@/lib/so/exec";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

function episodeFromGet(data: unknown): Record<string, unknown> | null {
  const row = Array.isArray(data) ? data[0] : data;
  if (!row || typeof row !== "object") return null;
  return row as Record<string, unknown>;
}

export async function GET(
  req: NextRequest,
  ctx: { params: Promise<{ id: string }> },
) {
  const { id } = await ctx.params;
  const project = projectIdFromRequest(req);
  return runWithProjectAsync(project, async () => {
    const res = await soJSON<unknown>(["memory", "get", id]);
    if (!res.ok) {
      return NextResponse.json({ error: res.error }, { status: 404 });
    }
    const episode = episodeFromGet(res.data);
    if (!episode) {
      return NextResponse.json({ error: "memory not found" }, { status: 404 });
    }
    return NextResponse.json(episode);
  });
}
