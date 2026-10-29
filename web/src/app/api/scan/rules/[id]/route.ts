import { NextRequest, NextResponse } from "next/server";
import { projectIdFromRequest, runWithProjectAsync } from "@/lib/so/workspace";
import { soJSON } from "@/lib/so/exec";
import type { ScanRule } from "../route";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

type Ctx = { params: Promise<{ id: string }> };

/** Enable or disable a rule (built-ins too): body { enabled: boolean }. */
export async function PATCH(req: NextRequest, ctx: Ctx) {
  const { id } = await ctx.params;
  const project = projectIdFromRequest(req);
  const body = (await req.json().catch(() => ({}))) as { enabled?: boolean };
  if (typeof body.enabled !== "boolean") {
    return NextResponse.json({ error: "enabled must be true or false" }, { status: 400 });
  }
  return runWithProjectAsync(project, async () => {
    const res = await soJSON<ScanRule>(["scan", "rules", body.enabled ? "enable" : "disable", id]);
    if (!res.ok) return NextResponse.json({ error: res.error }, { status: 400 });
    return NextResponse.json({ rule: res.data });
  });
}

/** Delete a custom rule. Removing an override brings the built-in back. */
export async function DELETE(req: NextRequest, ctx: Ctx) {
  const { id } = await ctx.params;
  const project = projectIdFromRequest(req);
  return runWithProjectAsync(project, async () => {
    const res = await soJSON<{ id: string; built_in_restored: boolean }>(["scan", "rules", "remove", id]);
    if (!res.ok) return NextResponse.json({ error: res.error }, { status: 400 });
    return NextResponse.json(res.data ?? { id });
  });
}
