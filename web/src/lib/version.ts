/**
 * Product semver (MAJOR.MINOR.PATCH) - keep in sync with `/VERSION` and
 * `internal/version.Version`. Release builds override the Go value via ldflags.
 */
export const VERSION = "0.1.0";

/** UI display form without a leading `v` (e.g. `0.1.0`). */
export function displayVersion(semver: string = VERSION): string {
  const s = String(semver || VERSION).trim();
  if (!s) return VERSION;
  return s.startsWith("v") ? s.slice(1) : s;
}
