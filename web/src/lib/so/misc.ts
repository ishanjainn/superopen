import { mkdirSync, rmSync, writeFileSync } from "fs";
import { homedir } from "os";
import { basename, dirname, join } from "path";
import { gitRemoteURL, repoSlug } from "./git";
import { fileExists, readText } from "./nodeio";
import { processRepoRoot } from "./root";

export type Project = {
  id: string;
  name: string;
  repo_root: string;
  so_root?: string;
  remote_url?: string;
  slug?: string;
  missing?: boolean;
};

function enrich(project: Project): Project {
  const remote = project.remote_url || gitRemoteURL(project.repo_root) || undefined;
  const slug = remote ? repoSlug(project.repo_root) : project.slug || project.name || basename(project.repo_root) || project.id;
  return {
    ...project,
    remote_url: remote,
    slug,
    name: project.name || slug.split("/").pop() || basename(project.repo_root) || project.id,
    missing: !fileExists(project.repo_root),
  };
}

function projectsFile(): string {
  if (process.env.XDG_CONFIG_HOME) return join(process.env.XDG_CONFIG_HOME, "superopen", "projects.json");
  if (process.platform === "win32") {
    return join(process.env.APPDATA || join(homedir(), "AppData", "Roaming"), "superopen", "projects.json");
  }
  return join(homedir(), ".config", "superopen", "projects.json");
}

type ProjectsFile = {
  projects: Project[];
  active_project_id?: string;
  active_id?: string;
};

function readProjectsFile(): ProjectsFile {
  const path = projectsFile();
  if (!fileExists(path)) return { projects: [] };
  try {
    const value = JSON.parse(readText(path)) as ProjectsFile;
    return {
      projects: Array.isArray(value.projects) ? value.projects : [],
      active_project_id: value.active_project_id || value.active_id,
    };
  } catch {
    return { projects: [] };
  }
}

function writeProjectsFile(value: ProjectsFile) {
  const path = projectsFile();
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(path, `${JSON.stringify({ projects: value.projects, active_project_id: value.active_project_id || "" }, null, 2)}\n`, "utf8");
}

export function listProjects(activeRoot = processRepoRoot()): { projects: Project[]; active: Project } {
  const local = enrich({ id: "local", name: basename(activeRoot) || "local", repo_root: activeRoot, so_root: join(activeRoot, ".so") });
  const value = readProjectsFile();
  const projects = value.projects.map(enrich);
  if (!projects.some((project) => project.repo_root === activeRoot)) projects.unshift(local);
  const active = projects.find((project) => project.id === value.active_project_id) ||
    projects.find((project) => project.repo_root === activeRoot) ||
    local;
  return { projects, active: enrich(active) };
}

export type RemoveProjectResult = {
  project: Project;
  unregistered: boolean;
  purged_so: boolean;
  so_path?: string;
  repo_missing: boolean;
};

function matchesProject(project: Project, selector: string): boolean {
  return project.id === selector || project.repo_root === selector || project.name === selector || project.slug === selector;
}

export function removeProject(selector: string, purge = false): RemoveProjectResult {
  const value = readProjectsFile();
  const index = value.projects.findIndex((project) => matchesProject(project, selector));
  if (index < 0) throw new Error(`project not found: ${selector}`);
  const project = enrich(value.projects[index]);
  const soPath = project.so_root || join(project.repo_root, ".so");
  value.projects.splice(index, 1);
  if (value.active_project_id === project.id || value.active_project_id === selector) value.active_project_id = value.projects[0]?.id || "";
  writeProjectsFile(value);
  if (purge) {
    if (basename(soPath.replace(/\/+$/, "")) !== ".so") throw new Error(`refusing to delete non-.so path: ${soPath}`);
    rmSync(soPath, { recursive: true, force: true });
  }
  return { project, unregistered: true, purged_so: purge, so_path: soPath, repo_missing: Boolean(project.missing) };
}

export function pruneMissingProjects(purge = false): RemoveProjectResult[] {
  return readProjectsFile().projects.filter((project) => !fileExists(project.repo_root)).map((project) => removeProject(project.id, purge));
}
