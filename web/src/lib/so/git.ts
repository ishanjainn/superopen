import { execFileSync } from "child_process";
import { basename } from "path";
import { fileExists } from "./nodeio";
import { processRepoRoot, repoName, repoRoot } from "./root";
import { soBinary } from "./exec";
import { displayVersion, parseSoVersion } from "@/lib/version";

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

/**
 * Remotes are read with a blocking `git` spawn, and callers ask for every
 * registered project on every request - so memoize per repo and never spawn
 * for a path that no longer exists.
 */
const remoteCache = new Map<string, { url: string | null; at: number }>();
const REMOTE_TTL_MS = 60_000;

export function gitRemoteURL(repo: string): string | null {
  if (!repo) return null;
  const cached = remoteCache.get(repo);
  const now = Date.now();
  if (cached && now - cached.at < REMOTE_TTL_MS) return cached.url;

  let url: string | null = null;
  if (fileExists(repo)) {
    try {
      url =
        execFileSync("git", ["-C", repo, "remote", "get-url", "origin"], {
          encoding: "utf8",
          timeout: 2000,
          stdio: ["ignore", "pipe", "ignore"],
        }).trim() || null;
    } catch {
      url = null;
    }
  }
  remoteCache.set(repo, { url, at: now });
  return url;
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

function gitConfig(repo: string, key: string): string {
  if (!fileExists(repo)) return "";
  try {
    return (
      execFileSync("git", ["-C", repo, "config", "--get", key], {
        encoding: "utf8",
        timeout: 2000,
        stdio: ["ignore", "pipe", "ignore"],
      }).trim() || ""
    );
  } catch {
    return "";
  }
}

function initialsFrom(label: string): string {
  const parts = label
    .replace(/@.*$/, "")
    .replace(/[._-]+/g, " ")
    .split(/\s+/)
    .filter(Boolean);
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase();
  return label.slice(0, 2).toUpperCase() || "U";
}

/**
 * Best-effort GitHub avatar URL for the local git author. The image is always
 * hotlinked from GitHub at render time and never downloaded or stored by
 * Superopen. Only a GitHub account (noreply email or `github.user` config)
 * yields a URL; otherwise the UI shows a generic user icon. We deliberately do
 * not fall back to Gravatar, which keys on the raw email and would surface a
 * personal photo the user did not link to GitHub. The email never leaves the
 * server.
 */
export function gitAuthorAvatar(repo = repoRoot()): {
  initials: string;
  avatar_url: string;
} {
  const name = gitConfig(repo, "user.name");
  const email = gitConfig(repo, "user.email").toLowerCase();
  const login = gitConfig(repo, "github.user");
  const initials = initialsFrom(name || email || "You");

  const safeLogin = (value: string) =>
    /^[a-z\d](?:[a-z\d]|-(?=[a-z\d])){0,38}$/i.test(value) ? value : "";

  let avatar_url = "";
  // GitHub noreply emails carry the account: "<id>+<login>@users.noreply..."
  const noreply = email.match(
    /^(?:(\d+)\+)?([a-z\d-]+)@users\.noreply\.github\.com$/,
  );
  if (noreply?.[1]) {
    avatar_url = `https://avatars.githubusercontent.com/u/${noreply[1]}?v=4&s=64`;
  } else if (noreply?.[2] && safeLogin(noreply[2])) {
    avatar_url = `https://github.com/${encodeURIComponent(noreply[2])}.png?size=64`;
  } else if (login && safeLogin(login)) {
    avatar_url = `https://github.com/${encodeURIComponent(login)}.png?size=64`;
  }

  return { initials, avatar_url };
}

/** Running `so` semver. Falls back to the UI constant if the CLI is missing. */
let cachedCliVersion: string | null = null;

function cliVersion(): string {
  if (cachedCliVersion) return cachedCliVersion;
  try {
    const out = execFileSync(soBinary(), ["version"], {
      encoding: "utf8",
      timeout: 2000,
      stdio: ["ignore", "pipe", "ignore"],
    });
    const parsed = parseSoVersion(out);
    if (parsed) {
      cachedCliVersion = parsed;
      return parsed;
    }
  } catch {
    // optional
  }
  cachedCliVersion = displayVersion();
  return cachedCliVersion;
}

export function currentRepoMeta() {
  const root = repoRoot();
  const version = cliVersion();
  return {
    repo: repoName(),
    root,
    slug: repoSlug(root),
    remote: gitRemoteURL(root),
    author: gitAuthorAvatar(root),
    version,
    version_display: displayVersion(version),
  };
}
