import { NextResponse } from "next/server";
import { listAuditEvents } from "@/lib/so/audit";
import { projectIdFromRequest, runWithProject } from "@/lib/so/workspace";

export async function GET(req: Request) {
  const url = new URL(req.url);
  const limit = Number(url.searchParams.get("limit") || "100");
  const project = projectIdFromRequest(req);
  return runWithProject(project, () =>
    NextResponse.json(listAuditEvents(limit))
  );
}
