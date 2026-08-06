import { NextRequest, NextResponse } from "next/server";
import { listGuardrailsDashboard } from "@/lib/so/guardrails-dashboard";
import { projectIdFromRequest, runWithProject } from "@/lib/so/workspace";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(req: NextRequest) {
  const project = projectIdFromRequest(req);
  const data = runWithProject(project, () => listGuardrailsDashboard());
  return NextResponse.json(data);
}
