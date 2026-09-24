import { NextRequest, NextResponse } from "next/server";
import { projectIdFromRequest, runWithProjectAsync } from "@/lib/so/workspace";
import { soJSON } from "@/lib/so/exec";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export type JevDecision = {
  enabled?: boolean;
  key_set?: boolean;
  session_id?: string;
  task?: string;
  correction?: string;
  choice?: string;
  evidence?: number;
  correction_prob?: number;
  promote?: boolean;
  memory_id?: number;
  memory_title?: string;
  memory_text?: string;
  cached?: boolean;
  note?: string;
};

export async function GET(req: NextRequest) {
  const project = projectIdFromRequest(req);
  return runWithProjectAsync(project, async () => {
    const res = await soJSON<JevDecision>(["harvest", "jev"]);
    if (!res.ok) {
      return NextResponse.json({ error: res.error, enabled: true }, { status: 200 });
    }
    return NextResponse.json(res.data ?? {});
  });
}
