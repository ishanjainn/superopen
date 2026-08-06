import { AsyncLocalStorage } from "async_hooks";
import { basename, join } from "path";
import { absPath, fileExists } from "./nodeio";

export type WorkspaceOverride = {
  repoRoot: string;
  soRoot: string;
};

const workspaceStorage = new AsyncLocalStorage<WorkspaceOverride>();

export function runWithWorkspace<T>(ws: WorkspaceOverride, fn: () => T): T {
  return workspaceStorage.run(ws, fn);
}

export async function runWithWorkspaceAsync<T>(
  ws: WorkspaceOverride,
  fn: () => Promise<T>
): Promise<T> {
  return workspaceStorage.run(ws, fn);
}

function defaultRepoRoot(): string {
  const fromEnv = process.env.SUPEROPEN_ROOT;
  if (typeof fromEnv === "string") {
    const trimmed = fromEnv.trim();
    if (trimmed) return absPath(trimmed);
  }
  // Fallback: walk up from cwd looking for .so/
  let dir = process.cwd();
  for (let i = 0; i < 8; i++) {
    if (fileExists(join(dir, ".so"))) return dir;
    const parent = absPath(join(dir, ".."));
    if (parent === dir) break;
    dir = parent;
  }
  return process.cwd();
}

/** Repo root for the local Superopen workspace (set by `so dev`), or request override. */
export function repoRoot(): string {
  return workspaceStorage.getStore()?.repoRoot ?? defaultRepoRoot();
}

export function soRoot(): string {
  return workspaceStorage.getStore()?.soRoot ?? join(defaultRepoRoot(), ".so");
}

export function soPath(...parts: string[]): string {
  return join(soRoot(), ...parts);
}

export function repoName(): string {
  return basename(repoRoot()) || "repo";
}

/** Unscoped SUPEROPEN_ROOT (ignores request project override). */
export function processRepoRoot(): string {
  return defaultRepoRoot();
}
