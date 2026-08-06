import { NextRequest, NextResponse } from "next/server";
import {
  listRecommendations,
  listRecommendationsDashboard,
} from "@/lib/so/misc";
import { projectIdFromRequest, runWithProject } from "@/lib/so/workspace";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(req: NextRequest) {
  const project = projectIdFromRequest(req);
  const view = req.nextUrl.searchParams.get("view");
  return runWithProject(project, () => {
    if (view === "dashboard") {
      return NextResponse.json(listRecommendationsDashboard());
    }
    return NextResponse.json(listRecommendations());
  });
}
