import { NextRequest, NextResponse } from "next/server";
import { listSessionsPage } from "@/lib/so/sessions";
import { projectIdFromRequest } from "@/lib/so/workspace";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(req: NextRequest) {
  const q = req.nextUrl.searchParams.get("q") || "";
  const project = projectIdFromRequest(req);
  return NextResponse.json(listSessionsPage(q, project));
}
