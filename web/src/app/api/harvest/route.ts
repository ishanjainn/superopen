import { NextRequest, NextResponse } from "next/server";
import { projectIdFromRequest, runWithProjectAsync } from "@/lib/so/workspace";
import { soJSON, soJSONRows } from "@/lib/so/exec";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

type ListRow = {
  id?: number | string;
  session?: string;
  session_id?: string;
  status?: string;
  kind?: string;
  target?: string;
  title?: string;
  reason?: string;
  source?: string;
  created_at?: string;
};

export async function GET(req: NextRequest) {
  const project = projectIdFromRequest(req);
  const session = req.nextUrl.searchParams.get("session") || "";
  const history = ["1", "true", "yes"].includes(
    (req.nextUrl.searchParams.get("history") || "").toLowerCase(),
  );
  return runWithProjectAsync(project, async () => {
    if (history) {
      const list = await soJSON<ListRow>(["harvest", "list", "--history"]);
      if (!list.ok) {
        return NextResponse.json({ error: list.error, items: [] }, { status: 200 });
      }
      let rows = soJSONRows<ListRow>(list);
      if (session) {
        rows = rows.filter(
          (row) => row.session === session || row.session_id === session,
        );
      }
      return NextResponse.json({ items: rows });
    }

    const [list, hist] = await Promise.all([
      soJSON<ListRow>(["harvest", "list"]),
      soJSON<ListRow>(["harvest", "list", "--history"]),
    ]);
    if (!list.ok) {
      return NextResponse.json({ error: list.error, items: [] }, { status: 200 });
    }
    let rows: ListRow[] = soJSONRows<ListRow>(list);
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
    const historyRows = hist.ok ? soJSONRows<ListRow>(hist) : [];
    return NextResponse.json({
      items,
      latest: historyRows[0] ?? null,
    });
  });
}
