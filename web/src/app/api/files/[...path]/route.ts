import { NextRequest, NextResponse } from "next/server";
import {
  createHarnessFile,
  deleteHarnessFile,
  listOrRead,
  writeHarnessFile,
} from "@/lib/so/files";
import { soJSON, repoCwd } from "@/lib/so/exec";
import { projectIdFromRequest, runWithProject } from "@/lib/so/workspace";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(
  req: NextRequest,
  ctx: { params: Promise<{ path: string[] }> | { path: string[] } }
) {
  const params = await Promise.resolve(ctx.params);
  const rel = (params.path || []).join("/");
  const project = projectIdFromRequest(req);
  const result = runWithProject(project, () => listOrRead(rel));
  if (!result) {
    return NextResponse.json({ error: "not found" }, { status: 404 });
  }
  if (result.type === "dir") {
    return NextResponse.json({ entries: result.entries || [] });
  }
  return new NextResponse(result.body ?? "", {
    status: 200,
    headers: { "Content-Type": "text/plain; charset=utf-8" },
  });
}

/** Save existing file body. */
export async function PUT(
  req: NextRequest,
  ctx: { params: Promise<{ path: string[] }> | { path: string[] } }
) {
  const params = await Promise.resolve(ctx.params);
  const rel = (params.path || []).join("/");
  const project = projectIdFromRequest(req);
  let body: { content?: string; text?: string; create?: boolean } = {};
  try {
    body = await req.json();
  } catch {
    return NextResponse.json({ error: "JSON body required" }, { status: 400 });
  }
  const content = typeof body.content === "string" ? body.content : body.text;
  if (typeof content !== "string") {
    return NextResponse.json({ error: "content required" }, { status: 400 });
  }
  try {
    const result = runWithProject(project, () =>
      writeHarnessFile(rel, content, { create: Boolean(body.create) })
    );
    if (/^(knowledge|rules|skills)\//.test(rel) || /^(knowledge|rules|skills)$/.test(rel)) {
      void soJSON(["sync", "--skip-graph"], { cwd: repoCwd(), timeoutMs: 120_000 }).catch(
        () => undefined
      );
    }
    return NextResponse.json({ ok: true, ...result });
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e);
    const status = msg.includes("not found")
      ? 404
      : msg.includes("already exists")
        ? 409
      : msg.includes("only ") || msg.includes("invalid")
        ? 400
        : 500;
    return NextResponse.json({ error: msg }, { status });
  }
}

/** Create a new file under an editable harness dir (path = dir). */
export async function POST(
  req: NextRequest,
  ctx: { params: Promise<{ path: string[] }> | { path: string[] } }
) {
  const params = await Promise.resolve(ctx.params);
  const dir = (params.path || []).join("/");
  const project = projectIdFromRequest(req);
  let body: { name?: string; filename?: string; content?: string } = {};
  try {
    body = await req.json();
  } catch {
    return NextResponse.json({ error: "JSON body required" }, { status: 400 });
  }
  const name = String(body.name || body.filename || "").trim();
  if (!name) {
    return NextResponse.json({ error: "name required" }, { status: 400 });
  }
  try {
    const result = runWithProject(project, () =>
      createHarnessFile(
        dir,
        name,
        typeof body.content === "string" ? body.content : undefined
      )
    );
    return NextResponse.json({ ok: true, ...result }, { status: 201 });
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e);
    const status = msg.includes("already exists")
      ? 409
      : msg.includes("cannot") ||
          msg.includes("filename") ||
          msg.includes("invalid")
        ? 400
        : 500;
    return NextResponse.json({ error: msg }, { status });
  }
}

export async function DELETE(
  req: NextRequest,
  ctx: { params: Promise<{ path: string[] }> | { path: string[] } }
) {
  const params = await Promise.resolve(ctx.params);
  const rel = (params.path || []).join("/");
  const project = projectIdFromRequest(req);
  try {
    runWithProject(project, () => deleteHarnessFile(rel));
    if (
      /^(knowledge|rules|skills|guardrails|evals)\//.test(rel) ||
      /^(knowledge|rules|skills|guardrails|evals)$/.test(rel)
    ) {
      void soJSON(["sync", "--skip-graph"], { cwd: repoCwd(), timeoutMs: 120_000 }).catch(
        () => undefined
      );
    }
    return NextResponse.json({ ok: true });
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e);
    const status = msg.includes("not found")
      ? 404
      : msg.includes("only ") || msg.includes("invalid")
        ? 400
        : 500;
    return NextResponse.json({ error: msg }, { status });
  }
}
