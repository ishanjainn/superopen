import { NextRequest, NextResponse } from "next/server";
import { projectIdFromRequest, runWithProjectAsync } from "@/lib/so/workspace";
import { soJSON } from "@/lib/so/exec";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(req: NextRequest) {
  const project = projectIdFromRequest(req);
  return runWithProjectAsync(project, async () => {
    const res = await soJSON<unknown>(["memory", "status"]);
    if (!res.ok) {
      return NextResponse.json({ error: res.error }, { status: 200 });
    }
    return NextResponse.json(res.data ?? {});
  });
}
