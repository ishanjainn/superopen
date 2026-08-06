import { execFileSync } from "child_process";
import { basename } from "path";
import { processRepoRoot, repoName, repoRoot } from "./root";
import { VERSION, displayVersion } from "@/lib/version";

/** Parse `owner/repo` from a git remote URL (HTTPS, SSH, or SSH aliases). */
function slugFromRemoteURL(url: string): string | null {
  const raw = url.trim().replace(/\.git$/i, "");
  if (!raw) return null;

  // git@host:owner/repo  or  git@host-alias:owner/repo
  const ssh = raw.match(/^git@[^:]+:(.+)$/);
  if (ssh?.[1]) {
    const path = ssh[1].replace(/^\/+/, "");
    const parts = path.split("/").filter(Boolean);
    if (parts.length >= 2) return `${parts[parts.length - 2]}/${parts[parts.length - 1]}`;
  }

  // ssh://git@host/owner/repo
  const sshURL = raw.match(/^ssh:\/\/[^/]+\/(.+)$/i);
  if (sshURL?.[1]) {
    const parts = sshURL[1].split("/").filter(Boolean);
    if (parts.length >= 2) return `${parts[parts.length - 2]}/${parts[parts.length - 1]}`;
  }

  // https://host/owner/repo
  try {
    const u = new URL(raw.includes("://") ? raw : `https://${raw}`);
    const parts = u.pathname.split("/").filter(Boolean);
    if (parts.length >= 2) return `${parts[parts.length - 2]}/${parts[parts.length - 1]}`;
  } catch {
    /* ignore */
  }

  return null;
}

export function gitRemoteURL(repo: string): string | null {
  try {
    const out = execFileSync("git", ["-C", repo, "remote", "get-url", "origin"], {
      encoding: "utf8",
      timeout: 2000,
      stdio: ["ignore", "pipe", "ignore"],
    }).trim();
    return out || null;
  } catch {
    return null;
  }
}

/** Full `owner/repo` for a workspace, falling back to directory basename. */
export function repoSlug(repo = processRepoRoot()): string {
  const remote = gitRemoteURL(repo);
  if (remote) {
    const slug = slugFromRemoteURL(remote);
    if (slug) return slug;
  }
  return basename(repo) || repoName();
}

export function currentRepoMeta() {
  const root = repoRoot();
  return {
    repo: repoName(),
    root,
    slug: repoSlug(root),
    remote: gitRemoteURL(root),
    version: VERSION,
    version_display: displayVersion(),
  };
}
