import { NextResponse } from "next/server";
import { soJSON, repoCwd } from "@/lib/so/exec";
import { projectIdFromRequest, runWithProjectAsync } from "@/lib/so/workspace";

export async function POST(req: Request) {
  const body = await req.json().catch(() => ({}));
  const vendor = String(body.vendor || "");
  const query = String(body.query || "");
  const mode = String(body.mode || "persistent");
  if (!vendor) {
    return NextResponse.json({ ok: false, error: "vendor required" }, { status: 400 });
  }
  const project = projectIdFromRequest(req);
  return runWithProjectAsync(project, async () => {
    const args = ["sessions", "start", "--vendor", vendor, "--mode", mode, "--no-launch"];
    if (query) args.push("--query", query);
    const res = await soJSON(args, { cwd: repoCwd(), timeoutMs: 30_000 });
    return NextResponse.json(res);
  });
}
