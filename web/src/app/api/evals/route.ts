import { NextRequest, NextResponse } from "next/server";
import { listEvalsDashboard } from "@/lib/so/evals";
import { repoCwd, soJSON } from "@/lib/so/exec";
import {
  projectIdFromRequest,
  runWithProject,
  runWithProjectAsync,
} from "@/lib/so/workspace";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(req: NextRequest) {
  const project = projectIdFromRequest(req);
  const data = runWithProject(project, () => listEvalsDashboard());
  return NextResponse.json(data);
}

export async function POST(req: NextRequest) {
  const project = projectIdFromRequest(req);
  let body: { session_id?: unknown; force?: unknown } = {};
  try {
    body = (await req.json()) as { session_id?: unknown; force?: unknown };
  } catch {
    // Empty body evaluates the latest captured session.
  }
  const sessionId =
    typeof body.session_id === "string" ? body.session_id.trim() : "";
  const force = body.force === true;
  return runWithProjectAsync(project, async () => {
    const args = sessionId ? ["eval", sessionId] : ["eval"];
    if (force) args.push("--force");
    const result = await soJSON(args, { cwd: repoCwd(), timeoutMs: 180_000 });
    if (!result.ok) {
      return NextResponse.json(
        { error: result.error || "evaluation failed" },
        { status: 400 }
      );
    }
    return NextResponse.json(result.data || { ok: true });
  });
}
