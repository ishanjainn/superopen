import { NextRequest, NextResponse } from "next/server";
import { getAllPrefs, getPref, setPref } from "@/lib/so/prefs";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(req: NextRequest) {
  const key = req.nextUrl.searchParams.get("key");
  if (key) {
    return NextResponse.json({ key, value: getPref(key) });
  }
  return NextResponse.json(getAllPrefs());
}

export async function PUT(req: NextRequest) {
  const body = (await req.json()) as { key?: string; value?: string };
  if (!body.key) {
    return NextResponse.json({ error: "key required" }, { status: 400 });
  }
  setPref(body.key, String(body.value ?? ""));
  return NextResponse.json({ status: "ok" });
}

export async function POST(req: NextRequest) {
  return PUT(req);
}
