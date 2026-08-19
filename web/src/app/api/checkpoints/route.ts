import { NextRequest, NextResponse } from "next/server";
import { soJSON, repoCwd } from "@/lib/so/exec";
import { projectIdFromRequest, runWithProjectAsync } from "@/lib/so/workspace";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

/** Restore a checkpoint created on session finalize. Manual create stays CLI-only. */
export async function POST(req: NextRequest) {
  const body = await req.json().catch(() => ({}));
  const op = String(body.op || "");
  const sessionId = String(body.session_id || body.sessionId || "").trim();
  const checkpointId = String(body.checkpoint_id || body.checkpointId || "").trim();
  const project = projectIdFromRequest(req);

  if (!sessionId) {
    return NextResponse.json({ error: "session_id required" }, { status: 400 });
  }

  return runWithProjectAsync(project, async () => {
    if (op === "create") {
      return NextResponse.json(
        {
          error:
            "Manual checkpoints are created with `so sessions checkpoint create`. Snapshots are also created automatically on session finalize.",
        },
        { status: 400 }
      );
    }
    if (op === "restore") {
      if (!checkpointId) {
        return NextResponse.json({ error: "checkpoint_id required" }, { status: 400 });
      }
      const res = await soJSON(
        ["sessions", "checkpoint", "restore", sessionId, checkpointId],
        {
          cwd: repoCwd(),
          timeoutMs: 60_000,
        }
      );
      if (res.ok === false) {
        return NextResponse.json({ error: res.error || "restore failed" }, { status: 400 });
      }
      return NextResponse.json({ ok: true, ...(res.data as object) });
    }
    return NextResponse.json({ error: "unknown op" }, { status: 400 });
  });
}
