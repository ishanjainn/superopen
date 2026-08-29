import { NextRequest, NextResponse } from "next/server";
import { projectIdFromRequest, runWithProjectAsync } from "@/lib/so/workspace";
import { soJSON, soJSONRows } from "@/lib/so/exec";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(req: NextRequest) {
  const project = projectIdFromRequest(req);
  return runWithProjectAsync(project, async () => {
    const res = await soJSON<unknown>(["memory", "timeline"]);
    if (!res.ok) {
      return NextResponse.json({ error: res.error, buckets: [] }, { status: 200 });
    }
    const items = soJSONRows(res);
    return NextResponse.json({ buckets: [{ when: "recent", items }] });
  });
}
