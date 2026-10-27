import { NextRequest, NextResponse } from "next/server";
import { projectIdFromRequest, runWithProjectAsync } from "@/lib/so/workspace";
import { soJSON } from "@/lib/so/exec";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

type QueryNode = {
  qn?: string;
  kind?: string;
  file?: string;
  lines?: string;
};

type QueryBody = {
  qualified_name?: string;
  file?: string;
  start_line?: number;
  total_lines?: number;
  code?: string;
};

type QueryPayload = {
  count?: number;
  total?: number;
  nodes?: QueryNode[];
  bodies?: QueryBody[];
};

// answerText renders the ranked page the way the CLI prints it: a line per
// symbol, then the source of the top callables with line numbers.
function answerText(payload: QueryPayload): string {
  const nodes = payload.nodes ?? [];
  if (nodes.length === 0) return "No indexed symbol matches this question.";
  const lines = [`${nodes.length} of ${payload.total ?? nodes.length} matches`];
  for (const node of nodes) {
    lines.push(`${node.qn ?? ""}  ${node.file ?? ""}:${node.lines ?? ""}`);
  }
  for (const body of payload.bodies ?? []) {
    lines.push("", `${body.qualified_name ?? ""}  ${body.file ?? ""}`);
    const first = body.start_line ?? 1;
    (body.code ?? "").split("\n").forEach((line, i) => lines.push(`${first + i}| ${line}`));
  }
  return lines.join("\n");
}

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
        { error: res.error, tip: "Ensure `so` is on PATH; try `so graph build`" },
        { status: 500 },
      );
    }
    const payload = res.data ?? {};
    return NextResponse.json({
      query: q,
      answer: answerText(payload),
      seeds: (payload.nodes ?? []).map((node) => ({ qualified_name: node.qn, file: node.file })),
    });
  });
}
