import { NextResponse } from "next/server";
import { listAuditEvents } from "@/lib/so/audit";

export async function GET(req: Request) {
  const url = new URL(req.url);
  const limit = Number(url.searchParams.get("limit") || "100");
  return NextResponse.json(listAuditEvents(limit));
}
