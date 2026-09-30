import { NextRequest, NextResponse } from "next/server";
import { soJSON } from "@/lib/so/exec";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

type JevSettings = {
  enabled?: boolean;
  key_set?: boolean;
};

export async function GET() {
  const res = await soJSON<JevSettings>(["harvest", "jev-settings"]);
  if (!res.ok) {
    return NextResponse.json({ error: res.error }, { status: 500 });
  }
  const data = res.data ?? {};
  return NextResponse.json({
    enabled: Boolean(data.enabled),
    key_set: Boolean(data.key_set),
  });
}

export async function PUT(req: NextRequest) {
  const body = (await req.json().catch(() => ({}))) as {
    enabled?: boolean;
    api_key?: string;
  };
  const payload = JSON.stringify({
    enabled: Boolean(body.enabled),
    api_key: typeof body.api_key === "string" ? body.api_key : "",
  });
  const res = await soJSON<JevSettings>(["harvest", "jev-settings", "--write"], { stdin: payload });
  if (!res.ok) {
    return NextResponse.json({ error: res.error }, { status: 500 });
  }
  const data = res.data ?? {};
  return NextResponse.json({
    enabled: Boolean(data.enabled),
    key_set: Boolean(data.key_set),
  });
}
