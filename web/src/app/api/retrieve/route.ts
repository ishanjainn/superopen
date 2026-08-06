import { NextResponse } from "next/server";
import { soJSON, repoCwd } from "@/lib/so/exec";
import { projectIdFromRequest, runWithProjectAsync } from "@/lib/so/workspace";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(req: Request) {
  const url = new URL(req.url);
  const q = url.searchParams.get("q") || "";
  const project = projectIdFromRequest(req);
  return runWithProjectAsync(project, async () => {
    if (!q) return NextResponse.json({ schema: 1, ok: true, data: [] });
    const res = await soJSON(["retrieve", q], { cwd: repoCwd() });
    return NextResponse.json(res);
  });
}
