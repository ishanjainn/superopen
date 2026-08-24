import { NextRequest, NextResponse } from "next/server";
import { projectIdFromRequest, runWithProjectAsync } from "@/lib/so/workspace";
import { soJSON } from "@/lib/so/exec";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

type ListRow = {
  id?: number | string;
  session?: string;
  session_id?: string;
};

export async function GET(req: NextRequest) {
  const project = projectIdFromRequest(req);
  const session = req.nextUrl.searchParams.get("session") || "";
  return runWithProjectAsync(project, async () => {
    const list = await soJSON<ListRow[]>(["harvest", "list"]);
    if (!list.ok) {
      return NextResponse.json({ error: list.error, items: [] }, { status: 200 });
    }
    let rows: ListRow[] = Array.isArray(list.data)
      ? list.data
      : Array.isArray(list.items)
        ? (list.items as ListRow[])
        : [];
    if (session) {
      rows = rows.filter(
        (row) => row.session === session || row.session_id === session,
      );
    }
    const items: unknown[] = [];
    for (const row of rows) {
      const id = String(row.id ?? "");
      if (!id) continue;
      const show = await soJSON<unknown>(["harvest", "show", id]);
      if (show.ok && show.data) items.push(show.data);
    }
    return NextResponse.json({ items });
  });
}
