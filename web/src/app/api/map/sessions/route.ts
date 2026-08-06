import { NextRequest, NextResponse } from "next/server";
import { listMapSessions } from "@/lib/so/trace";
import { projectIdFromRequest, runWithProject } from "@/lib/so/workspace";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(req: NextRequest) {
  const project = projectIdFromRequest(req);
  return runWithProject(project, () => NextResponse.json(listMapSessions()));
}
