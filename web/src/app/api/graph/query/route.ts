import { NextRequest, NextResponse } from "next/server";
import { projectIdFromRequest, runWithProjectAsync } from "@/lib/so/workspace";
import { soJSON } from "@/lib/so/exec";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

type QuerySeed = {
  qualified_name?: string;
  name?: string;
  file?: string;
};

type QueryPayload = {
  text?: string;
  seeds?: QuerySeed[];
};

export async function GET(req: NextRequest) {
  const q = req.nextUrl.searchParams.get("q")?.trim() || "";
  const project = projectIdFromRequest(req);
  return runWithProjectAsync(project, async () => {
    if (!q) {
      return NextResponse.json({ error: "q required" }, { status: 400 });
    }
    const res = await soJSON<QueryPayload>(["graph", "query", q]);
    if (!res.ok) {
      return NextResponse.json(
        { error: res.error, tip: "Ensure `so` is on PATH; try `so graph rebuild`" },
        { status: 500 },
      );
    }
    const payload = res.data ?? {};
    return NextResponse.json({
      query: q,
      answer: payload.text ?? "",
      seeds: Array.isArray(payload.seeds) ? payload.seeds : [],
    });
  });
}
