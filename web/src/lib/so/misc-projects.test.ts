import {
  existsSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "fs";
import { tmpdir } from "os";
import { join } from "path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

describe("removeProject / pruneMissingProjects", () => {
  let dir: string;
  let projectsPath: string;

  beforeEach(() => {
    dir = mkdtempSync(join(tmpdir(), "so-projects-"));
    projectsPath = join(dir, "superopen", "projects.json");
    mkdirSync(join(dir, "superopen"), { recursive: true });
    vi.stubEnv("XDG_CONFIG_HOME", dir);
    vi.resetModules();
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.resetModules();
    rmSync(dir, { recursive: true, force: true });
  });

  it("unregisters and optionally purges .so", async () => {
    const repo = join(dir, "repo-a");
    const so = join(repo, ".so");
    mkdirSync(so, { recursive: true });
    writeFileSync(join(so, "config.yaml"), "memory:\n  enabled: true\n");
    writeFileSync(
      projectsPath,
      JSON.stringify(
        {
          projects: [
            {
              id: "aaaaaaaaaaaaaaaa",
              name: "repo-a",
              repo_root: repo,
              so_root: so,
            },
          ],
          active_project_id: "aaaaaaaaaaaaaaaa",
        },
        null,
        2
      )
    );

    const { removeProject } = await import("./misc");
    const res = removeProject("aaaaaaaaaaaaaaaa", true);
    expect(res.unregistered).toBe(true);
    expect(res.purged_so).toBe(true);
    expect(existsSync(so)).toBe(false);
    expect(existsSync(repo)).toBe(true);

    const raw = JSON.parse(readFileSync(projectsPath, "utf8")) as {
      projects: unknown[];
    };
    expect(raw.projects).toHaveLength(0);
  });

  it("prunes missing repo paths", async () => {
    const gone = join(dir, "gone-repo");
    const orphanSo = join(dir, "orphan", ".so");
    mkdirSync(orphanSo, { recursive: true });
    writeFileSync(
      projectsPath,
      JSON.stringify(
        {
          projects: [
            {
              id: "bbbbbbbbbbbbbbbb",
              name: "gone",
              repo_root: gone,
              so_root: orphanSo,
            },
          ],
        },
        null,
        2
      )
    );

    const { pruneMissingProjects } = await import("./misc");
    const out = pruneMissingProjects(true);
    expect(out).toHaveLength(1);
    expect(out[0].repo_missing).toBe(true);
    expect(existsSync(orphanSo)).toBe(false);
  });
});
