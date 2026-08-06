import { NextRequest, NextResponse } from "next/server";
import { listEvalsDashboard } from "@/lib/so/evals";
import { projectIdFromRequest, runWithProject } from "@/lib/so/workspace";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(req: NextRequest) {
  const project = projectIdFromRequest(req);
  const data = runWithProject(project, () => listEvalsDashboard());
  return NextResponse.json(data);
}
