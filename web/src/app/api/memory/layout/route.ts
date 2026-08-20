import { NextRequest, NextResponse } from "next/server";
import { projectIdFromRequest, runWithProjectAsync } from "@/lib/so/workspace";
import { soJSON } from "@/lib/so/exec";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(req: NextRequest) {
  const maxNodesRaw = Number(req.nextUrl.searchParams.get("max_nodes"));
  const maxNodes =
    Number.isFinite(maxNodesRaw) && maxNodesRaw > 0
      ? Math.min(Math.round(maxNodesRaw), 10_000)
      : 2000;
  const project = projectIdFromRequest(req);
  return runWithProjectAsync(project, async () => {
    const res = await soJSON<{
      nodes: unknown[];
      edges: unknown[];
      total_nodes: number;
      total_edges: number;
      project: string;
    }>(["memory", "layout", "--max-nodes", String(maxNodes)], {
      timeoutMs: 120_000,
    });
    if (!res.ok || !res.data) {
      return NextResponse.json(
        { error: res.error || "so memory layout failed", nodes: [], edges: [] },
        { status: 200 },
      );
    }
    return NextResponse.json(res.data);
  });
}
