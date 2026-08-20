import { NextRequest, NextResponse } from "next/server";
import { writeFile, mkdir } from "fs/promises";
import { join } from "path";
import { tmpdir } from "os";
import { projectIdFromRequest, runWithProjectAsync } from "@/lib/so/workspace";
import { soJSON } from "@/lib/so/exec";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function POST(req: NextRequest) {
  const project = projectIdFromRequest(req);
  const contentType = req.headers.get("content-type") || "";
  return runWithProjectAsync(project, async () => {
    if (contentType.includes("multipart/form-data")) {
      const form = await req.formData();
      const file = form.get("file");
      const title = String(form.get("title") || "");
      if (!(file instanceof File)) {
        return NextResponse.json({ error: "file required" }, { status: 400 });
      }
      const dir = join(tmpdir(), "so-memory-teach");
      await mkdir(dir, { recursive: true });
      const dest = join(dir, file.name.replace(/[^A-Za-z0-9._-]/g, "_") || "teach.txt");
      const buf = Buffer.from(await file.arrayBuffer());
      await writeFile(dest, buf);
      const args = ["memory", "teach", dest];
      if (title) args.push("--title", title);
      const res = await soJSON<unknown>(args);
      if (!res.ok) {
        return NextResponse.json({ error: res.error }, { status: 500 });
      }
      return NextResponse.json(res.data ?? {});
    }
    const body = (await req.json().catch(() => ({}))) as {
      title?: string;
      text?: string;
    };
    const args = ["memory", "teach"];
    if (body.title) args.push("--title", body.title);
    if (body.text) args.push("--text", body.text);
    const res = await soJSON<unknown>(args);
    if (!res.ok) {
      return NextResponse.json({ error: res.error }, { status: 500 });
    }
    return NextResponse.json(res.data ?? {});
  });
}
