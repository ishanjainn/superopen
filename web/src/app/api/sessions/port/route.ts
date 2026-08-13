import { NextResponse } from "next/server";
import { soJSON, repoCwd } from "@/lib/so/exec";
import { projectIdFromRequest, runWithProjectAsync } from "@/lib/so/workspace";

type PortBody = {
  op?: "detect" | "preview" | "port" | "verify";
  from?: string;
  to?: string;
  ids?: string[];
  all?: boolean;
  force?: boolean;
  sample?: number;
};

export async function GET(req: Request) {
  const project = projectIdFromRequest(req);
  return runWithProjectAsync(project, async () => {
    const res = await soJSON(["sessions", "detect"], { cwd: repoCwd() });
    return NextResponse.json(res);
  });
}

export async function POST(req: Request) {
  const body = (await req.json().catch(() => ({}))) as PortBody;
  const op = body.op || "preview";
  const project = projectIdFromRequest(req);

  return runWithProjectAsync(project, async () => {
    const cwd = repoCwd();

    if (op === "detect") {
      const res = await soJSON(["sessions", "detect"], { cwd });
      return NextResponse.json(res);
    }
    if (op === "verify") {
      const args = ["sessions", "verify", "--from", String(body.from || "claude")];
      if (body.to) args.push("--to", String(body.to));
      if (body.sample) args.push("--sample", String(body.sample));
      const res = await soJSON(args, { cwd, timeoutMs: 120_000 });
      return NextResponse.json(res);
    }
    if (op === "preview" || op === "port") {
      const from = String(body.from || "");
      const to = String(body.to || "");
      if (!from || !to) {
        return NextResponse.json(
          { schema: 1, ok: false, error: "from and to required" },
          { status: 400 }
        );
      }
      const args = ["sessions", "port", "--from", from, "--to", to];
      if (op === "preview") args.push("--preview");
      if (body.force) args.push("--force");
      if (body.all) args.push("--all");
      for (const id of body.ids || []) {
        if (id) args.push("--id", id);
      }
      if (op === "port" && !body.all && !(body.ids && body.ids.length)) {
        return NextResponse.json(
          { schema: 1, ok: false, error: "select ids or all" },
          { status: 400 }
        );
      }
      const res = await soJSON(args, { cwd, timeoutMs: 180_000 });
      return NextResponse.json(res);
    }
    return NextResponse.json({ schema: 1, ok: false, error: "unknown op" }, { status: 400 });
  });
}
