/**
 * Server-side path/fs helpers for the `.so` workspace.
 *
 * Next 16 Turbopack is default; annotate Node fs/path calls so tracing can
 * skip intentional dynamic `.so/` IO (also covered by turbopack.ignoreIssue).
 */
import {
  existsSync,
  mkdirSync,
  readFileSync,
  writeFileSync,
  type PathLike,
} from "fs";
import { dirname, resolve } from "path";

/** Absolute path. Empty 2nd arg is a TP1006-friendly resolve form. */
export function absPath(input: string): string {
  const p = String(input || "");
  if (!p) return resolve(/* turbopackIgnore: true */ ".", "");
  return resolve(/* turbopackIgnore: true */ p, "");
}

export function fileExists(filePath: string): boolean {
  const p = filePath;
  return existsSync(/* turbopackIgnore: true */ p);
}

export function readText(filePath: string): string {
  const p = filePath;
  return readFileSync(/* turbopackIgnore: true */ p, "utf8");
}

export function writeText(filePath: string, body: string): void {
  const p = filePath;
  mkdirSync(/* turbopackIgnore: true */ dirname(p), { recursive: true });
  writeFileSync(/* turbopackIgnore: true */ p, body);
}

export function readJSONFile<T>(filePath: string): T | null {
  try {
    return JSON.parse(readText(filePath)) as T;
  } catch {
    return null;
  }
}

export function pathExists(filePath: PathLike): boolean {
  const p = String(filePath);
  return existsSync(/* turbopackIgnore: true */ p);
}
