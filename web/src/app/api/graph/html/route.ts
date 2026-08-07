import { existsSync, readFileSync } from "fs";
import { NextRequest, NextResponse } from "next/server";
import { applyDarkTheme, applyLightTheme } from "@/lib/so/graphify-theme";
import { inspectGraphHtml, type GraphHtmlStatus } from "@/lib/so/graph-html";
import {
  applyCachedPositions,
  POSITION_REPORT_SCRIPT,
  readCachedPositions,
} from "@/lib/so/graph-positions";
import { soPath } from "@/lib/so/root";
import { projectIdFromRequest, runWithProject } from "@/lib/so/workspace";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

function loadGraphHtml(theme: "light" | "dark"): {
  body: Buffer | null;
  status: GraphHtmlStatus;
} {
  const path = soPath("graph", "graph.html");
  if (!existsSync(path)) {
    return {
      body: null,
      status: {
        ok: false,
        reason: "graph.html not found",
        tip: "so graph rebuild   or   graphify cluster-only . && graphify export html --graph .so/graph/graph.json",
      },
    };
  }
  const raw = readFileSync(path, "utf8");
  const status = inspectGraphHtml(raw);
  if (!status.ok) {
    return { body: null, status };
  }
  let html = theme === "dark" ? applyDarkTheme(raw) : applyLightTheme(raw);

  // Skip vis-network's ~200-iteration stabilization on every visit: bake in
  // positions from the last real stabilization when they still match this
  // graph.html, otherwise let it stabilize once and report the result back.
  const cached = readCachedPositions();
  html = cached
    ? applyCachedPositions(html, cached)
    : html.replace("</body>", `${POSITION_REPORT_SCRIPT}</body>`);

  return { body: Buffer.from(html, "utf8"), status };
}

function themeFromRequest(req: NextRequest): "light" | "dark" {
  const t = (req.nextUrl.searchParams.get("theme") || "light").toLowerCase();
  return t === "dark" ? "dark" : "light";
}

export async function HEAD(req: NextRequest) {
  const project = projectIdFromRequest(req);
  const theme = themeFromRequest(req);
  const { body, status } = runWithProject(project, () => loadGraphHtml(theme));
  if (!body) {
    return new NextResponse(null, {
      status: 404,
      headers: {
        "X-So-Graph-Reason": status.reason || "unavailable",
        "X-So-Graph-Tip": status.tip || "so graph rebuild",
      },
    });
  }
  return new NextResponse(null, {
    status: 200,
    headers: {
      "Content-Type": "text/html; charset=utf-8",
      "Content-Length": String(body.length),
    },
  });
}

export async function GET(req: NextRequest) {
  const project = projectIdFromRequest(req);
  const theme = themeFromRequest(req);
  const url = new URL(req.url);
  if (url.searchParams.get("meta") === "1") {
    const { status } = runWithProject(project, () => loadGraphHtml(theme));
    return NextResponse.json(status);
  }
  const { body, status } = runWithProject(project, () => loadGraphHtml(theme));
  if (!body) {
    return NextResponse.json(
      {
        ok: false,
        reason: status.reason,
        tip: status.tip,
        commands: [
          "so graph rebuild",
          "graphify cluster-only .",
          "graphify export html --graph .so/graph/graph.json",
        ],
      },
      { status: 404 }
    );
  }
  return new NextResponse(body, {
    status: 200,
    headers: {
      "Content-Type": "text/html; charset=utf-8",
      "Cache-Control": "public, max-age=120",
      "X-Content-Type-Options": "nosniff",
    },
  });
}
