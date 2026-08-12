import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "fs";
import { join } from "path";
import { tmpdir } from "os";
import { afterEach, describe, expect, it } from "vitest";
import {
  createHarnessFile,
  deleteHarnessFile,
  listOrRead,
  writeHarnessFile,
} from "./files";

const roots: string[] = [];

afterEach(() => {
  delete process.env.SUPEROPEN_ROOT;
  for (const root of roots.splice(0)) rmSync(root, { recursive: true, force: true });
});

function repository(): string {
  const root = mkdtempSync(join(tmpdir(), "superopen-files-skills-"));
  roots.push(root);
  process.env.SUPEROPEN_ROOT = root;
  mkdirSync(join(root, ".so"), { recursive: true });
  writeFileSync(
    join(root, ".so", "config.yaml"),
    "vendors:\n  enabled:\n    - codex\n"
  );
  return root;
}

describe("native skill paths", () => {
  it("lists and edits the real repository-relative SKILL.md path", () => {
    const root = repository();
    const nested = join(root, ".codex", "skills", "review", "SKILL.md");
    mkdirSync(join(root, ".codex", "skills", "review"), { recursive: true });
    writeFileSync(nested, "# Review\n");

    const listing = listOrRead("skills");
    expect(listing?.entries).toEqual([
      {
        name: "codex/review",
        path: ".codex/skills/review/SKILL.md",
        isDir: false,
      },
    ]);
    expect(listOrRead(".codex/skills/review/SKILL.md")?.body).toBe("# Review\n");

    writeHarnessFile(".codex/skills/review/SKILL.md", "# Updated\n");
    expect(readFileSync(nested, "utf8")).toBe("# Updated\n");
    deleteHarnessFile(".codex/skills/review/SKILL.md");
    expect(listOrRead(".codex/skills/review/SKILL.md")).toBeNull();
  });

  it("creates the standard folder layout and ignores loose skill files", () => {
    const root = repository();
    const skills = join(root, ".codex", "skills");
    mkdirSync(skills, { recursive: true });
    writeFileSync(join(skills, "SKILL.md"), "# Loose\n");
    writeFileSync(join(skills, "loose.md"), "# Loose\n");

    expect(listOrRead("skills")?.entries).toEqual([]);
    const created = createHarnessFile("skills/codex", "testing");
    expect(created.path).toBe(".codex/skills/testing/SKILL.md");
    expect(readFileSync(join(skills, "testing", "SKILL.md"), "utf8")).toContain(
      "# SKILL"
    );
  });
});
