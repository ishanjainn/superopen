import { NextRequest, NextResponse } from "next/server";
import { projectIdFromRequest, runWithProjectAsync } from "@/lib/so/workspace";
import { soJSON } from "@/lib/so/exec";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

type LayoutPayload = {
  nodes: unknown[];
  edges: unknown[];
  total_nodes: number;
  total_edges: number;
  project: string;
};

export async function GET(req: NextRequest) {
  const maxNodesRaw = Number(req.nextUrl.searchParams.get("max_nodes"));
  const maxNodes =
    Number.isFinite(maxNodesRaw) && maxNodesRaw > 0
      ? Math.min(Math.round(maxNodesRaw), 10_000_000)
      : 5000;
  const project = projectIdFromRequest(req);
  return runWithProjectAsync(project, async () => {
    const res = await soJSON<LayoutPayload>(
      ["graph", "layout", "--max-nodes", String(maxNodes)],
      { timeoutMs: 300_000 },
    );
    if (!res.ok || !res.data) {
      const message = res.error || "so graph layout failed";
      // An unindexed repo is a setup state, not a failure: the UI turns this
      // code into build instructions instead of an error banner.
      const notIndexed = /graph_not_indexed|no native graph exists/i.test(
        message,
      );
      return NextResponse.json(
        {
          error: message,
          code: notIndexed ? "graph_not_indexed" : undefined,
          tip: "Ensure `so` is on PATH; try `so graph rebuild`",
        },
        { status: notIndexed ? 404 : 500 },
      );
    }
    return NextResponse.json(res.data);
  });
}
