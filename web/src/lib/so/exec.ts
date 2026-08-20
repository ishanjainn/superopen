import { spawn } from "child_process";
import { homedir } from "os";
import { join } from "path";
import { fileExists } from "./nodeio";
import { repoRoot, soRoot } from "./root";

export type SoJSON<T> = {
  schema: number;
  ok: boolean;
  error?: string;
  data?: T;
  items?: unknown[];
  hint?: string;
};

/**
 * Run `so <args> --json` and parse the envelope.
 * Native graph and session materialization algorithms live in Go.
 * SUPEROPEN_ROOT follows the active workspace override (x-so-project) when set.
 */
export async function soJSON<T = unknown>(
  args: string[],
  opts?: { cwd?: string; timeoutMs?: number },
): Promise<SoJSON<T>> {
  const cwd = opts?.cwd ?? repoCwd();
  const bin = soBinary();
  const child = spawn(bin, [...args, "--json"], {
    cwd,
    env: {
      ...process.env,
      SUPEROPEN_JSON: "1",
      SUPEROPEN_ROOT: repoRoot(),
    },
    stdio: ["ignore", "pipe", "pipe"],
  });
  const chunks: Buffer[] = [];
  const errChunks: Buffer[] = [];
  child.stdout.on("data", (c) => chunks.push(c));
  child.stderr.on("data", (c) => errChunks.push(c));
  const timeout = opts?.timeoutMs ?? 60_000;
  const code: number = await new Promise((resolve) => {
    const t = setTimeout(() => {
      child.kill("SIGKILL");
      resolve(-1);
    }, timeout);
    child.on("close", (c) => {
      clearTimeout(t);
      resolve(c ?? 0);
    });
  });
  const raw = Buffer.concat(chunks).toString("utf8").trim();
  const stderr = Buffer.concat(errChunks).toString("utf8").trim();
  if (!raw) {
    // A failing `so` command writes its envelope to stderr and leaves stdout
    // empty. Parse it so callers see the engine's error code rather than an
    // opaque wrapper string with JSON embedded in it.
    const envelope = parseEnvelope<T>(stderr);
    if (envelope) return envelope;
    return {
      schema: 1,
      ok: false,
      error: `so ${args.join(" ")} failed (exit ${code}): ${stderr}`,
    };
  }
  try {
    return JSON.parse(raw) as SoJSON<T>;
  } catch {
    return {
      schema: 1,
      ok: false,
      error: `invalid JSON from so: ${raw.slice(0, 200)}`,
    };
  }
}

function parseEnvelope<T>(raw: string): SoJSON<T> | null {
  if (!raw.startsWith("{")) return null;
  try {
    const parsed = JSON.parse(raw) as SoJSON<T>;
    return typeof parsed?.ok === "boolean" ? parsed : null;
  } catch {
    return null;
  }
}

export function soBinary(): string {
  const configured = process.env.SUPEROPEN_SO_BIN?.trim();
  if (configured) return configured;
  const candidates = [
    process.env.GOBIN
      ? join(process.env.GOBIN, process.platform === "win32" ? "so.exe" : "so")
      : "",
    join(
      homedir(),
      "go",
      "bin",
      process.platform === "win32" ? "so.exe" : "so",
    ),
    join(
      homedir(),
      ".superopen",
      "bin",
      process.platform === "win32" ? "so.exe" : "so",
    ),
    "/opt/homebrew/bin/so",
    "/usr/local/bin/so",
  ].filter(Boolean);
  return candidates.find(fileExists) || "so";
}

export function repoCwd(): string {
  // Parent of .so/ - prefer workspace override via repoRoot()
  return repoRoot() || soRoot().replace(/[/\\]\.so$/, "") || process.cwd();
}
