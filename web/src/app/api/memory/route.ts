import { NextResponse } from "next/server";
import {
  composeLessonText,
  deleteLessonLocal,
  deletePreferenceItem,
  deleteProjectItem,
  listLessons,
  listPatterns,
  listPreferenceItems,
  listProjectSections,
  readActivePack,
  readMarkdown,
  upsertPreferenceItem,
  upsertProjectItem,
  writeMarkdown,
  isStubMarkdown,
} from "@/lib/so/memory";
import type { MemoryVerb } from "@/lib/so/memory-items";
import { soJSON, repoCwd } from "@/lib/so/exec";
import { projectIdFromRequest, runWithProjectAsync } from "@/lib/so/workspace";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

async function refreshActive(query = "") {
  const args = ["memory", "refresh"];
  if (query) args.push("--query", query);
  try {
    await soJSON(args, { cwd: repoCwd() });
  } catch {
    /* best-effort - pack rebuilds on next SessionStart */
  }
}

export async function GET(req: Request) {
  const url = new URL(req.url);
  const op = url.searchParams.get("op") || "list";
  const project = projectIdFromRequest(req);
  return runWithProjectAsync(project, async () => {
    if (op === "lessons") {
      return NextResponse.json(listLessons());
    }
    if (op === "patterns") {
      return NextResponse.json(listPatterns());
    }
    if (op === "active") {
      return NextResponse.json({ text: readActivePack() });
    }
    if (op === "preferences") {
      const text = readMarkdown("preferences.md");
      return NextResponse.json({
        text,
        stub: isStubMarkdown(text),
        items: listPreferenceItems(),
      });
    }
    if (op === "projects") {
      const text = readMarkdown("projects.md");
      return NextResponse.json({
        text,
        stub: isStubMarkdown(text),
        sections: listProjectSections(),
      });
    }
    if (op === "context") {
      const q = url.searchParams.get("query") || "";
      const mode = url.searchParams.get("mode") || "persistent";
      const args = ["memory", "active-context", "--mode", mode];
      if (q) args.push("--query", q);
      const res = await soJSON(args, { cwd: repoCwd() });
      return NextResponse.json(res);
    }
    if (op === "search") {
      const q = url.searchParams.get("q") || "";
      const res = await soJSON(["memory", "search", q], { cwd: repoCwd() });
      return NextResponse.json(res);
    }
    return NextResponse.json({
      lessons: listLessons().length,
      active: Boolean(readActivePack()),
    });
  });
}

export async function POST(req: Request) {
  const body = await req.json().catch(() => ({}));
  const op = body.op as string;
  const project = projectIdFromRequest(req);

  return runWithProjectAsync(project, async () => {
    if (op === "add_lesson") {
      const verb = String(body.verb || "").trim() as MemoryVerb | "";
      const text = String(body.text || "").trim();
      if (!text) {
        return NextResponse.json({ error: "empty lesson" }, { status: 400 });
      }
      if (!verb) {
        return NextResponse.json(
          { error: "pick a verb (prefer / always / never / ask / avoid)" },
          { status: 400 }
        );
      }
      const full = composeLessonText(verb, text);
      const res = await soJSON(["memory", "add", full], { cwd: repoCwd() });
      return NextResponse.json({ ...res, lessons: listLessons() });
    }

    if (op === "update_lesson") {
      const id = String(body.id || "");
      const verb = String(body.verb || "").trim() as MemoryVerb | "";
      const text = String(body.text || "").trim();
      if (!id || !text) {
        return NextResponse.json({ error: "id and text required" }, { status: 400 });
      }
      if (!verb) {
        return NextResponse.json({ error: "pick a verb" }, { status: 400 });
      }
      const full = composeLessonText(verb, text);
      const res = await soJSON(["memory", "update", id, full], { cwd: repoCwd() });
      if (res.ok === false) {
        return NextResponse.json(
          { ok: false, error: String(res.error || "update failed") },
          { status: 400 }
        );
      }
      return NextResponse.json({ ...res, ok: true, lessons: listLessons() });
    }

    if (op === "delete_lesson") {
      const id = String(body.id || "");
      if (!id) {
        return NextResponse.json({ error: "id required" }, { status: 400 });
      }
      const res = await soJSON(["memory", "rm", id], { cwd: repoCwd() });
      if (res.ok === false) {
        try {
          deleteLessonLocal(id);
          await refreshActive();
          return NextResponse.json({ ok: true, lessons: listLessons() });
        } catch (e) {
          return NextResponse.json(
            { ok: false, error: String(e instanceof Error ? e.message : e) },
            { status: 400 }
          );
        }
      }
      return NextResponse.json({ ...res, lessons: listLessons() });
    }

    if (op === "upsert_preference") {
      const verb = String(body.verb || "").trim();
      if (!verb) {
        return NextResponse.json({ error: "pick a verb" }, { status: 400 });
      }
      try {
        const items = upsertPreferenceItem({
          id: body.id ? String(body.id) : undefined,
          verb: verb as MemoryVerb,
          text: String(body.text || ""),
        });
        await refreshActive();
        return NextResponse.json({ ok: true, items });
      } catch (e) {
        return NextResponse.json(
          { ok: false, error: String(e instanceof Error ? e.message : e) },
          { status: 400 }
        );
      }
    }

    if (op === "delete_preference") {
      try {
        const items = deletePreferenceItem(String(body.id || ""));
        await refreshActive();
        return NextResponse.json({ ok: true, items });
      } catch (e) {
        return NextResponse.json(
          { ok: false, error: String(e instanceof Error ? e.message : e) },
          { status: 400 }
        );
      }
    }

    if (op === "upsert_project_item") {
      try {
        const sections = upsertProjectItem({
          sectionId: String(body.sectionId || ""),
          id: body.id ? String(body.id) : undefined,
          verb: String(body.verb || "").trim() as MemoryVerb | "",
          text: String(body.text || ""),
        });
        await refreshActive();
        return NextResponse.json({ ok: true, sections });
      } catch (e) {
        return NextResponse.json(
          { ok: false, error: String(e instanceof Error ? e.message : e) },
          { status: 400 }
        );
      }
    }

    if (op === "delete_project_item") {
      try {
        const sections = deleteProjectItem(
          String(body.sectionId || ""),
          String(body.id || "")
        );
        await refreshActive();
        return NextResponse.json({ ok: true, sections });
      } catch (e) {
        return NextResponse.json(
          { ok: false, error: String(e instanceof Error ? e.message : e) },
          { status: 400 }
        );
      }
    }

    if (op === "write" && body.name && typeof body.text === "string") {
      writeMarkdown(String(body.name), body.text);
      await refreshActive();
      return NextResponse.json({ ok: true });
    }
    if (op === "refresh") {
      const q = String(body.query || "");
      const args = ["memory", "refresh"];
      if (q) args.push("--query", q);
      const res = await soJSON(args, { cwd: repoCwd() });
      return NextResponse.json(res);
    }
    if (op === "consolidate") {
      const summary = String(body.summary || "ui consolidate");
      const res = await soJSON(["memory", "consolidate", summary], { cwd: repoCwd() });
      return NextResponse.json(res);
    }
    if (op === "reset_template") {
      const name = String(body.name || "");
      if (name !== "preferences.md" && name !== "projects.md") {
        return NextResponse.json({ error: "invalid name" }, { status: 400 });
      }
      writeMarkdown(name, `# ${name.replace(".md", "")}\n`);
      await refreshActive();
      const key = name.replace(".md", "");
      const text = readMarkdown(name);
      return NextResponse.json({
        ok: true,
        name,
        text,
        stub: isStubMarkdown(text),
        key,
        items: key === "preferences" ? listPreferenceItems() : undefined,
        sections: key === "projects" ? listProjectSections() : undefined,
      });
    }
    return NextResponse.json({ error: "unknown op" }, { status: 400 });
  });
}
