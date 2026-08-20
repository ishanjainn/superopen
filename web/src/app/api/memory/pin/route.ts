import { NextRequest, NextResponse } from "next/server";
import { projectIdFromRequest, runWithProjectAsync } from "@/lib/so/workspace";
import { soJSON } from "@/lib/so/exec";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function POST(req: NextRequest) {
  const project = projectIdFromRequest(req);
  const body = (await req.json().catch(() => ({}))) as { id?: number | string };
  const id = String(body.id ?? "");
  if (!id) {
    return NextResponse.json({ error: "id required" }, { status: 400 });
  }
  return runWithProjectAsync(project, async () => {
    const res = await soJSON<unknown>(["memory", "pin", id]);
    if (!res.ok) {
      return NextResponse.json({ error: res.error }, { status: 500 });
    }
    return NextResponse.json(res.data ?? {});
  });
}
