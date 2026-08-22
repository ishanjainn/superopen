import { NextRequest, NextResponse } from "next/server";
import { projectIdFromRequest, runWithProjectAsync } from "@/lib/so/workspace";
import { soJSON } from "@/lib/so/exec";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

type RetentionSettings = {
  session_hours?: number;
  memory_hours?: number;
};

export async function GET() {
  const res = await soJSON<RetentionSettings>(["gc", "--show"]);
  if (!res.ok) {
    return NextResponse.json({ error: res.error }, { status: 500 });
  }
  const data = res.data ?? {};
  return NextResponse.json({
    session_hours: data.session_hours ?? 168,
    memory_hours: data.memory_hours ?? 168,
  });
}

export async function PUT(req: NextRequest) {
  const body = (await req.json().catch(() => ({}))) as {
    session_hours?: number;
    memory_hours?: number;
    apply?: boolean;
  };
  const sessionHours = Number.isFinite(body.session_hours)
    ? Math.max(0, Math.floor(Number(body.session_hours)))
    : 168;
  const memoryHours = Number.isFinite(body.memory_hours)
    ? Math.max(0, Math.floor(Number(body.memory_hours)))
    : 168;
  const args = [
    "gc",
    "--sessions-hours",
    String(sessionHours),
    "--memory-hours",
    String(memoryHours),
  ];
  if (!body.apply) {
    args.push("--show");
  }
  const project = projectIdFromRequest(req);
  return runWithProjectAsync(project, async () => {
    const res = await soJSON<Record<string, unknown>>(args);
    if (!res.ok) {
      return NextResponse.json({ error: res.error }, { status: 500 });
    }
    const data = res.data ?? {};
    return NextResponse.json({
      session_hours: data.session_hours ?? sessionHours,
      memory_hours: data.memory_hours ?? memoryHours,
      sessions_deleted: data.sessions_deleted ?? [],
      memories_deleted: data.memories_deleted ?? 0,
    });
  });
}
