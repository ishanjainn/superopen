import { basename, join } from "path";
import { listProjects, type Project } from "./misc";
import { absPath } from "./nodeio";
import {
  processRepoRoot,
  runWithWorkspace,
  runWithWorkspaceAsync,
  type WorkspaceOverride,
} from "./root";

export function projectIdFromRequest(req: Request): string {
  const header = req.headers.get("x-so-project");
  if (header != null) return header.trim();
  try {
    return new URL(req.url).searchParams.get("project")?.trim() || "";
  } catch {
    return "";
  }
}

/**
 * Resolve which workspace a UI project filter maps to.
 * - "" (This repo): current SUPEROPEN_ROOT
 * - "all": current SUPEROPEN_ROOT for single-root pages (sessions lists merge separately)
 * - id: that project's so_root / repo_root
 */
function resolveWorkspace(projectFilter: string): WorkspaceOverride & {
  projectId: string;
  name: string;
} {
  const activeRoot = processRepoRoot();
  const { projects, active } = listProjects(activeRoot);
  const filter = projectFilter.trim();

  if (!filter || filter === "all") {
    return {
      repoRoot: activeRoot,
      soRoot: join(activeRoot, ".so"),
      projectId: active.id || "local",
      name: basename(activeRoot) || "repo",
    };
  }

  const hit =
    projects.find((p) => p.id === filter) ||
    projects.find((p) => p.repo_root === filter) ||
    projects.find((p) => p.name === filter);

  if (!hit) {
    return {
      repoRoot: activeRoot,
      soRoot: join(activeRoot, ".so"),
      projectId: "local",
      name: basename(activeRoot) || "repo",
    };
  }

  const repoRootPath = String(hit.repo_root || "");
  const soRootPath = hit.so_root ? String(hit.so_root) : "";
  const repo = repoRootPath ? absPath(repoRootPath) : activeRoot;
  const so = soRootPath ? absPath(soRootPath) : join(repo, ".so");
  return {
    repoRoot: repo,
    soRoot: so,
    projectId: hit.id,
    name: hit.name || basename(repo),
  };
}

export function runWithProject<T>(projectFilter: string, fn: () => T): T {
  const ws = resolveWorkspace(projectFilter);
  return runWithWorkspace(ws, fn);
}

export async function runWithProjectAsync<T>(
  projectFilter: string,
  fn: () => Promise<T>
): Promise<T> {
  const ws = resolveWorkspace(projectFilter);
  return runWithWorkspaceAsync(ws, fn);
}

/** Projects to scan for session list filters. */
export function projectsForFilter(projectFilter: string): Project[] {
  const activeRoot = processRepoRoot();
  const { projects } = listProjects(activeRoot);
  const filter = projectFilter.trim();

  if (filter === "all") return projects.length ? projects : [{
    id: "local",
    name: basename(activeRoot) || "local",
    repo_root: activeRoot,
    so_root: join(activeRoot, ".so"),
  }];

  if (!filter) {
    const local = projects.filter((p) => p.repo_root === activeRoot);
    if (local.length) return local;
    return [
      {
        id: "local",
        name: basename(activeRoot) || "local",
        repo_root: activeRoot,
        so_root: join(activeRoot, ".so"),
      },
    ];
  }

  const hit = projects.find(
    (p) => p.id === filter || p.repo_root === filter || p.name === filter
  );
  return hit ? [hit] : [];
}
