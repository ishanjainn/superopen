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
  const root = mkdtempSync(join(tmpdir(), "superopen-files-rules-"));
  roots.push(root);
  process.env.SUPEROPEN_ROOT = root;
  mkdirSync(join(root, ".so"), { recursive: true });
  writeFileSync(
    join(root, ".so", "config.yaml"),
    "vendors:\n  enabled:\n    - cursor\n"
  );
  return root;
}

describe("native rule paths", () => {
  it("lists and edits the real repository-relative rule path", () => {
    const root = repository();
    const rule = join(root, ".cursor", "rules", "review.mdc");
    mkdirSync(join(root, ".cursor", "rules"), { recursive: true });
    writeFileSync(rule, "# Review\n");

    expect(listOrRead("rules")?.entries).toEqual([
      {
        name: "cursor/review.mdc",
        path: ".cursor/rules/review.mdc",
        isDir: false,
      },
    ]);
    expect(listOrRead(".cursor/rules/review.mdc")?.body).toBe("# Review\n");

    writeHarnessFile(".cursor/rules/review.mdc", "# Updated\n");
    expect(readFileSync(rule, "utf8")).toBe("# Updated\n");
    deleteHarnessFile(".cursor/rules/review.mdc");
    expect(listOrRead(".cursor/rules/review.mdc")).toBeNull();
  });

  it("returns the physical path when creating a rule", () => {
    const root = repository();
    const created = createHarnessFile("rules/cursor", "testing");

    expect(created.path).toBe(".cursor/rules/testing.mdc");
    expect(
      readFileSync(join(root, ".cursor", "rules", "testing.mdc"), "utf8")
    ).toContain("# Testing");
  });
});
