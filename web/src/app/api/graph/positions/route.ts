import { NextRequest, NextResponse } from "next/server";
import { writeCachedPositions } from "@/lib/so/graph-positions";
import { projectIdFromRequest, runWithProject } from "@/lib/so/workspace";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

type Body = {
  positions?: Record<string, { x?: unknown; y?: unknown }>;
};

/**
 * Receives the node layout vis-network computes the first time it stabilizes
 * a graph, so /api/graph/html can bake it in and skip re-stabilizing on every
 * subsequent /graph visit. See lib/so/graph-positions.ts.
 */
export async function POST(req: NextRequest) {
  const project = projectIdFromRequest(req);
  const body = (await req.json().catch(() => null)) as Body | null;
  const raw = body?.positions;
  if (!raw || typeof raw !== "object") {
    return NextResponse.json({ ok: false, error: "positions required" }, { status: 400 });
  }
  const positions: Record<string, { x: number; y: number }> = {};
  for (const [id, p] of Object.entries(raw)) {
    const x = Number(p?.x);
    const y = Number(p?.y);
    if (Number.isFinite(x) && Number.isFinite(y)) {
      positions[id] = { x, y };
    }
  }
  if (Object.keys(positions).length === 0) {
    return NextResponse.json({ ok: false, error: "no valid positions" }, { status: 400 });
  }
  const saved = runWithProject(project, () => writeCachedPositions(positions));
  return NextResponse.json({ ok: saved });
}
