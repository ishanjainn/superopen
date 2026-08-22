import { NextRequest, NextResponse } from "next/server";
import { projectIdFromRequest, runWithProjectAsync } from "@/lib/so/workspace";
import { soJSON, soJSONRows } from "@/lib/so/exec";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(req: NextRequest) {
  const project = projectIdFromRequest(req);
  const q = req.nextUrl.searchParams.get("q") || "";
  const kind = req.nextUrl.searchParams.get("kind") || "";
  return runWithProjectAsync(project, async () => {
    const args = ["memory", "search"];
    if (q) args.push(q);
    if (kind) args.push("--kind", kind);
    const res = await soJSON<unknown>(args);
    if (!res.ok) {
      return NextResponse.json({ error: res.error, items: [] }, { status: 200 });
    }
    return NextResponse.json({ items: soJSONRows(res) });
  });
}
