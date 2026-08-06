import { NextRequest, NextResponse } from "next/server";
import { currentRepoMeta } from "@/lib/so/git";
import { projectIdFromRequest, runWithProject } from "@/lib/so/workspace";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(req: NextRequest) {
  const project = projectIdFromRequest(req);
  return runWithProject(project, () => NextResponse.json(currentRepoMeta()));
}
