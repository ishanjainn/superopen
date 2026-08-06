import { NextRequest, NextResponse } from "next/server";
import { soJSON, repoCwd } from "@/lib/so/exec";
import { projectIdFromRequest, runWithProjectAsync } from "@/lib/so/workspace";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function POST(
  req: NextRequest,
  ctx: { params: Promise<{ id: string; action: string }> | { id: string; action: string } }
) {
  const params = await Promise.resolve(ctx.params);
  const id = decodeURIComponent(params.id);
  const action = params.action;
  const project = projectIdFromRequest(req);
  return runWithProjectAsync(project, async () => {
    if (action !== "dismiss" && action !== "apply" && action !== "revert") {
      return NextResponse.json({ error: "unknown action" }, { status: 400 });
    }
    const res = await soJSON(["recommend", action, id], {
      cwd: repoCwd(),
      timeoutMs: 120_000,
    });
    if (res.ok === false) {
      return NextResponse.json(
        { error: res.error || `${action} failed` },
        { status: 400 }
      );
    }
    return NextResponse.json({ status: "ok", ...(res.data as object) });
  });
}
