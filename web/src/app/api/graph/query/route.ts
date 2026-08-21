import { NextRequest, NextResponse } from "next/server";
import { spawn } from "child_process";
import { projectIdFromRequest, runWithProjectAsync } from "@/lib/so/workspace";
import { repoCwd, soBinary } from "@/lib/so/exec";
import { repoRoot } from "@/lib/so/root";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

async function runGraphQuery(question: string): Promise<{ ok: boolean; answer: string; error?: string }> {
  const bin = soBinary();
  const child = spawn(/* turbopackIgnore: true */ bin, ["graph", "query", question], {
    cwd: repoCwd(),
    env: { ...process.env, SUPEROPEN_ROOT: repoRoot() },
    stdio: ["ignore", "pipe", "pipe"],
  });
  const chunks: Buffer[] = [];
  const errChunks: Buffer[] = [];
  child.stdout.on("data", (c) => chunks.push(c));
  child.stderr.on("data", (c) => errChunks.push(c));
  const code: number = await new Promise((resolve) => {
    const t = setTimeout(() => {
      child.kill("SIGKILL");
      resolve(-1);
    }, 60_000);
    child.on("close", (c) => {
      clearTimeout(t);
      resolve(c ?? 0);
    });
  });
  const out = Buffer.concat(chunks).toString("utf8").trim();
  const err = Buffer.concat(errChunks).toString("utf8").trim();
  if (code !== 0 && !out) {
    return {
      ok: false,
      answer: "",
      error: err || `so graph query failed (exit ${code})`,
    };
  }
  return { ok: true, answer: out || err };
}

export async function GET(req: NextRequest) {
  const q = req.nextUrl.searchParams.get("q")?.trim() || "";
  const project = projectIdFromRequest(req);
  return runWithProjectAsync(project, async () => {
    if (!q) {
      return NextResponse.json({ error: "q required" }, { status: 400 });
    }
    const res = await runGraphQuery(q);
    if (!res.ok) {
      return NextResponse.json(
        { error: res.error, tip: "Ensure `so` is on PATH; try `so graph rebuild`" },
        { status: 500 }
      );
    }
    return NextResponse.json({ query: q, answer: res.answer });
  });
}
