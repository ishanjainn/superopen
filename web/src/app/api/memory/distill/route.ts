import { NextRequest, NextResponse } from "next/server";
import { projectIdFromRequest, runWithProjectAsync } from "@/lib/so/workspace";
import { soJSON } from "@/lib/so/exec";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function POST(req: NextRequest) {
  const project = projectIdFromRequest(req);
  const body = (await req.json().catch(() => ({}))) as {
    action?: string;
  };
  const action = body.action || "consolidate";
  return runWithProjectAsync(project, async () => {
    const args = ["memory", "distill"];
    if (action === "pause") args.push("--pause");
    else if (action === "resume") args.push("--resume");
    else if (action === "sleep") args.push("--sleep");
    else if (action === "restart") args.push("--restart");
    else args.push("--consolidate");
    const res = await soJSON<unknown>(args, { timeoutMs: 180_000 });
    if (!res.ok) {
      return NextResponse.json({ error: res.error }, { status: 500 });
    }
    return NextResponse.json(res.data ?? { ok: true });
  });
}
