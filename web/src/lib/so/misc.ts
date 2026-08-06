import { mkdirSync, rmSync, writeFileSync } from "fs";
import { homedir } from "os";
import { basename, dirname, join } from "path";
import { gitRemoteURL, repoSlug } from "./git";
import { fileExists, readJSONFile, readText } from "./nodeio";
import { processRepoRoot, soPath, soRoot } from "./root";

export type Project = {
  id: string;
  name: string;
  repo_root: string;
  so_root?: string;
  remote_url?: string;
  /** `owner/repo` when resolvable from git remote */
  slug?: string;
  /** True when repo_root is gone from disk but still in projects.json */
  missing?: boolean;
};

function enrich(p: Project): Project {
  const remote = p.remote_url || gitRemoteURL(p.repo_root) || undefined;
  const slug = remote
    ? repoSlug(p.repo_root)
    : p.slug || p.name || p.repo_root.split("/").pop() || p.id;
  const missing = !fileExists(p.repo_root);
  return {
    ...p,
    remote_url: remote,
    slug,
    name: p.name || slug.split("/").pop() || p.id,
    missing,
  };
}

function projectsFile(): string {
  const xdg = process.env.XDG_CONFIG_HOME;
  if (xdg) return join(xdg, "superopen", "projects.json");
  // Match Go internal/projects: Windows → %APPDATA%\superopen
  if (process.platform === "win32") {
    const appdata = process.env.APPDATA || join(homedir(), "AppData", "Roaming");
    return join(appdata, "superopen", "projects.json");
  }
  return join(homedir(), ".config", "superopen", "projects.json");
}

export function listProjects(activeRoot = processRepoRoot()): {
  projects: Project[];
  active: Project;
} {
  const active: Project = enrich({
    id: "local",
    name: activeRoot.split("/").pop() || "local",
    repo_root: activeRoot,
    so_root: join(activeRoot, ".so"),
  });
  const p = projectsFile();
  if (!fileExists(p)) {
    return { projects: [active], active };
  }
  try {
    const raw = JSON.parse(readText(p)) as {
      projects?: Project[];
      active_id?: string;
      active_project_id?: string;
    };
    const projects = (Array.isArray(raw.projects) ? raw.projects : [active]).map(
      enrich
    );
    const activeKey = raw.active_project_id || raw.active_id || "";
    const found =
      projects.find((x) => x.id === activeKey) ||
      projects.find((x) => x.repo_root === activeRoot) ||
      projects[0] ||
      active;
    if (!projects.some((x) => x.repo_root === activeRoot)) {
      projects.unshift(active);
    }
    return { projects, active: enrich(found) };
  } catch {
    return { projects: [active], active };
  }
}

export type RemoveProjectResult = {
  project: Project;
  unregistered: boolean;
  purged_so: boolean;
  so_path?: string;
  repo_missing: boolean;
};

type ProjectsFile = {
  projects: Project[];
  active_project_id?: string;
  active_id?: string;
};

function readProjectsFile(): ProjectsFile {
  const p = projectsFile();
  if (!fileExists(p)) return { projects: [] };
  try {
    const raw = JSON.parse(readText(p)) as ProjectsFile;
    return {
      projects: Array.isArray(raw.projects) ? raw.projects : [],
      active_project_id: raw.active_project_id || raw.active_id,
    };
  } catch {
    return { projects: [] };
  }
}

function writeProjectsFile(data: ProjectsFile) {
  const p = projectsFile();
  mkdirSync(dirname(p), { recursive: true });
  writeFileSync(
    p,
    JSON.stringify(
      {
        projects: data.projects,
        active_project_id: data.active_project_id || "",
      },
      null,
      2
    ) + "\n",
    "utf8"
  );
}

function matchesProject(p: Project, idOrRoot: string): boolean {
  return (
    p.id === idOrRoot ||
    p.repo_root === idOrRoot ||
    p.name === idOrRoot ||
    p.slug === idOrRoot
  );
}

function safePurgeSO(soDir: string): void {
  const cleaned = soDir.replace(/\/+$/, "");
  if (basename(cleaned) !== ".so") {
    throw new Error(`refusing to delete non-.so path: ${soDir}`);
  }
  if (!fileExists(cleaned)) return;
  rmSync(cleaned, { recursive: true, force: true });
}

/** Unregister a project; with purgeSO also delete its .so directory. */
export function removeProject(
  idOrRoot: string,
  purgeSO = false
): RemoveProjectResult {
  const data = readProjectsFile();
  const idx = data.projects.findIndex((p) => matchesProject(p, idOrRoot));
  if (idx < 0) {
    throw new Error(`project not found: ${idOrRoot}`);
  }
  const found = enrich(data.projects[idx]);
  const soDir = found.so_root || join(found.repo_root, ".so");
  data.projects.splice(idx, 1);
  if (
    data.active_project_id === found.id ||
    data.active_project_id === idOrRoot
  ) {
    data.active_project_id = data.projects[0]?.id || "";
  }
  writeProjectsFile(data);

  const result: RemoveProjectResult = {
    project: found,
    unregistered: true,
    purged_so: false,
    so_path: soDir,
    repo_missing: Boolean(found.missing),
  };
  if (purgeSO) {
    safePurgeSO(soDir);
    result.purged_so = true;
  }
  return result;
}

/** Drop registry entries whose repo_root no longer exists. */
export function pruneMissingProjects(purgeSO = false): RemoveProjectResult[] {
  const data = readProjectsFile();
  const missing = data.projects.filter((p) => !fileExists(p.repo_root));
  const out: RemoveProjectResult[] = [];
  for (const p of missing) {
    out.push(removeProject(p.id, purgeSO));
  }
  return out;
}

export function loadConfig(): Record<string, unknown> {
  const p = soPath("config.yaml");
  if (!fileExists(p)) {
    return {
      memory: { enabled: true, idle_harvest_hours: 6, backend: "auto" },
      guardrails: { enabled: true },
      retention: { days: 7 },
    };
  }
  const raw = readText(p);
  // Lightweight structured view for Settings UI (no yaml dependency).
  const pick = (key: string, fallback = ""): string => {
    const re = new RegExp(`^\\s*${key}:\\s*(.+)$`, "m");
    const m = raw.match(re);
    return m ? m[1].replace(/['"]/g, "").trim() : fallback;
  };
  const pickInt = (key: string, fallback: number): number => {
    const n = parseInt(pick(key, ""), 10);
    return Number.isFinite(n) ? n : fallback;
  };
  return {
    memory: {
      enabled: !/memory:[\s\S]*?enabled:\s*(false|0|off)/i.test(raw),
      idle_harvest_hours: pickInt("idle_harvest_hours", 6),
      backend: (() => {
        const m = raw.match(/memory:[\s\S]*?backend:\s*(\S+)/);
        return m?.[1]?.replace(/['"]/g, "") || "auto";
      })(),
    },
    guardrails: {
      enabled: !/(?:guardrails|governance):[\s\S]*?enabled:\s*(false|0|off)/i.test(raw),
    },
    graph: {
      code: !/graph:[\s\S]*?code:\s*false/i.test(raw),
      semantic: !/graph:[\s\S]*?semantic:\s*false/i.test(raw),
    },
    recommendations: {
      auto: !/recommendations:[\s\S]*?auto:\s*false/i.test(raw),
      require_approval: !/require_approval:\s*false/i.test(raw),
      soft_auto:
        /require_approval:\s*false/i.test(raw) ||
        (!/auto_apply_tiers:\s*\[\s*\]/.test(raw) &&
          (!/auto_apply_tiers:/.test(raw) ||
            /auto_apply_tiers:[\s\S]*?\bsoft\b/.test(raw) ||
            /auto_apply_tiers:[\s\S]*?\ball\b/.test(raw))),
    },
    retention: {
      days: (() => {
        const m = raw.match(/retention:[\s\S]*?days:\s*(\d+)/);
        if (!m) return 7;
        const n = parseInt(m[1], 10);
        return Number.isFinite(n) && n > 0 ? n : 7;
      })(),
    },
    evals: {
      on_session_end: !/on_session_end:\s*false/i.test(raw),
      auto: !/evals:[\s\S]*?auto:\s*false/i.test(raw),
      backend: (() => {
        const m = raw.match(/evals:[\s\S]*?backend:\s*(\S+)/);
        return m?.[1]?.replace(/['"]/g, "") || pick("backend", "auto");
      })(),
      model_claude: (() => {
        const m = raw.match(/evals:[\s\S]*?models:[\s\S]*?claude:\s*(\S+)/);
        return m?.[1]?.replace(/['"]/g, "") || "claude-sonnet-5";
      })(),
      model_codex: (() => {
        const m = raw.match(/evals:[\s\S]*?models:[\s\S]*?codex:\s*(\S+)/);
        return m?.[1]?.replace(/['"]/g, "") || "gpt-5.6-luna";
      })(),
    },
    advanced_llm: {
      provider: (() => {
        const m = raw.match(/llm:[\s\S]*?provider:\s*(\S+)/);
        return m?.[1]?.replace(/['"]/g, "") || "";
      })(),
      model: (() => {
        const m = raw.match(/llm:[\s\S]*?model:\s*(\S+)/);
        return m?.[1]?.replace(/['"]/g, "") || "";
      })(),
      api_key_env: (() => {
        const m = raw.match(/api_key_env:\s*(\S+)/);
        return m?.[1]?.replace(/['"]/g, "") || "";
      })(),
      base_url: (() => {
        const m = raw.match(/base_url:\s*(\S+)/);
        return m?.[1]?.replace(/['"]/g, "") || "";
      })(),
    },
  };
}

export type RecommendationStatus = "pending" | "applied" | "dismissed" | string;

export type Recommendation = {
  id: string;
  fingerprint?: string;
  title?: string;
  type?: string;
  rationale?: string;
  why?: string;
  status?: RecommendationStatus;
  proposed_path?: string;
  proposed_body?: string;
  session_id?: string;
  related_sessions?: string[];
  evidence?: string[];
  created_at?: string;
  [key: string]: unknown;
};

export type RecommendationsDashboard = {
  summary: {
    open: number;
    resolved: number;
    dismissed: number;
  };
  items: Recommendation[];
};

function pendingPath() {
  return soPath("recommendations", "pending.json");
}

function historyPath() {
  return soPath("recommendations", "history.json");
}

function readRecFile(path: string): Recommendation[] {
  if (!fileExists(path)) return [];
  try {
    const raw = readJSONFile<unknown>(path);
    return Array.isArray(raw) ? raw : [];
  } catch {
    return [];
  }
}

/** Stable cross-session key - mirrors Go recommend.FingerprintKey. */
export function fingerprintKey(
  recType: string,
  proposedPath: string,
  kind = ""
): string {
  let p = String(proposedPath || "").replace(/\\/g, "/");
  const soIdx = p.lastIndexOf("/.so/");
  if (soIdx >= 0) p = p.slice(soIdx + 5);
  else if (p.includes("/skills/")) p = `skills/${p.split("/").pop()}`;
  else if (p.includes("/knowledge/")) p = `knowledge/${p.split("/").pop()}`;
  else if (p.endsWith("guardrails.yaml")) p = "guardrails/guardrails.yaml";
  const t = String(recType || "").toLowerCase().trim();
  const k = String(kind || "").toLowerCase().trim();
  return k ? `${t}|${p}|${k}` : `${t}|${p}`;
}

function mergeSessions(...lists: Array<string[] | undefined>): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const list of lists) {
    for (const s of list || []) {
      if (!s || seen.has(s)) continue;
      seen.add(s);
      out.push(s);
    }
  }
  return out;
}

function ensureFingerprint(r: Recommendation): string {
  if (r.fingerprint) return String(r.fingerprint);
  const path = String(r.proposed_path || "");
  const kind =
    r.type === "skill" && path.includes("prefer-harness")
      ? "prefer-harness"
      : r.type === "docs" && path.includes("architecture")
        ? "architecture"
        : r.type === "guardrail"
          ? "tool-thrash"
          : "";
  r.fingerprint = fingerprintKey(String(r.type || ""), path, kind);
  return String(r.fingerprint);
}

/**
 * Collapse duplicate open cards that share a fingerprint (same skill/docs/
 * guardrail proposal from different sessions) into one row.
 */
function compactPendingRecommendations(): Recommendation[] {
  const pending = readRecFile(pendingPath()).map((r) => ({
    ...r,
    status: r.status || "pending",
  }));
  const byFp = new Map<string, Recommendation>();
  let changed = false;

  for (const r of pending) {
    const fp = ensureFingerprint(r);
    const status = String(r.status || "pending");
    const prev = byFp.get(fp);
    if (!prev) {
      byFp.set(fp, {
        ...r,
        fingerprint: fp,
        status: status === "stale" ? "pending" : status,
        related_sessions: mergeSessions(
          r.related_sessions,
          r.session_id ? [r.session_id] : []
        ),
      });
      if (status === "stale") changed = true;
      continue;
    }
    changed = true;
    byFp.set(fp, {
      ...prev,
      ...r,
      id: prev.id,
      fingerprint: fp,
      status: "pending",
      created_at: prev.created_at || r.created_at,
      related_sessions: mergeSessions(
        prev.related_sessions,
        r.related_sessions,
        prev.session_id ? [prev.session_id] : [],
        r.session_id ? [r.session_id] : []
      ),
      evidence: Array.from(
        new Set([...(prev.evidence || []), ...(r.evidence || [])])
      ).slice(0, 10),
      title: r.title || prev.title,
      rationale: r.rationale || prev.rationale,
      why: r.why || prev.why,
      proposed_body: r.proposed_body || prev.proposed_body,
    });
  }

  const compacted = Array.from(byFp.values());
  if (changed || compacted.length !== pending.length) {
    mkdirSync(dirname(pendingPath()), { recursive: true });
    writeFileSync(pendingPath(), JSON.stringify(compacted, null, 2));
  }
  return compacted;
}

export function listRecommendations(): Recommendation[] {
  return compactPendingRecommendations().map((r) => ({
    ...r,
    status: r.status || "pending",
  }));
}

function listRecommendationHistory(): Recommendation[] {
  return readRecFile(historyPath());
}

/** Pending + history (newest first). Pending wins if the same id appears twice. */
function listAllRecommendations(): Recommendation[] {
  const pending = listRecommendations();
  const history = listRecommendationHistory();
  const byId = new Map<string, Recommendation>();
  for (const r of history) {
    if (!r?.id) continue;
    byId.set(r.id, {
      ...r,
      status: r.status || "applied",
    });
  }
  for (const r of pending) {
    if (!r?.id) continue;
    byId.set(r.id, { ...r, status: r.status || "pending" });
  }
  return Array.from(byId.values()).sort((a, b) =>
    String(b.created_at || "").localeCompare(String(a.created_at || ""))
  );
}

export function getRecommendation(id: string): Recommendation | null {
  return listAllRecommendations().find((r) => r.id === id) || null;
}

export function listRecommendationsDashboard(): RecommendationsDashboard {
  const items = listAllRecommendations();
  const summary = { open: 0, resolved: 0, dismissed: 0 };
  for (const r of items) {
    const s = String(r.status || "pending");
    if (s === "applied") summary.resolved++;
    else if (s === "dismissed") summary.dismissed++;
    else if (s === "pending") summary.open++;
    // stale / invalid / reverted are not "Open"
  }
  return { summary, items };
}
