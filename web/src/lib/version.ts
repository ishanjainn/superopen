/**
 * Product semver fallback (MAJOR.MINOR.PATCH). Keep in sync with `/VERSION`
 * and `internal/version.Version`. The UI prefers the running `so` binary
 * via `/api/meta` (`so version`); this constant is only a last resort.
 */
export const VERSION = "0.3.0";

/** UI display form without a leading `v` (e.g. `0.3.0`). */
export function displayVersion(semver: string = VERSION): string {
  const s = String(semver || VERSION).trim();
  if (!s) return VERSION;
  return s.startsWith("v") ? s.slice(1) : s;
}

/** Parse `so version` / `so 0.3.0 (abc123)` stdout. */
export function parseSoVersion(output: string): string | null {
  const match = String(output || "").trim().match(/\bv?(\d+\.\d+\.\d+)\b/);
  return match?.[1] ?? null;
}
