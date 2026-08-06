import { NextRequest, NextResponse } from "next/server";
import {
  listProjects,
  pruneMissingProjects,
  removeProject,
} from "@/lib/so/misc";
import { processRepoRoot } from "@/lib/so/root";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET() {
  return NextResponse.json(listProjects(processRepoRoot()));
}

/** DELETE ?id=<id-or-path>&purge=1 - unregister (and optionally delete .so). */
export async function DELETE(req: NextRequest) {
  const id = req.nextUrl.searchParams.get("id")?.trim() || "";
  const purge = ["1", "true", "yes"].includes(
    (req.nextUrl.searchParams.get("purge") || "").toLowerCase()
  );
  const prune = ["1", "true", "yes"].includes(
    (req.nextUrl.searchParams.get("prune") || "").toLowerCase()
  );

  try {
    if (prune) {
      const removed = pruneMissingProjects(purge);
      return NextResponse.json({
        ok: true,
        pruned: removed.length,
        results: removed,
        ...listProjects(processRepoRoot()),
      });
    }
    if (!id) {
      return NextResponse.json(
        { error: "id query param required (or prune=1)" },
        { status: 400 }
      );
    }
    const result = removeProject(id, purge);
    return NextResponse.json({
      ok: true,
      result,
      ...listProjects(processRepoRoot()),
    });
  } catch (e) {
    return NextResponse.json(
      { error: String(e instanceof Error ? e.message : e) },
      { status: 400 }
    );
  }
}
