const PREFIX = "superopen.ui.";

export function getUIPref(key: string): string | null {
  if (typeof window === "undefined") return null;
  try {
    return window.localStorage.getItem(PREFIX + key);
  } catch {
    return null;
  }
}

export function setUIPref(key: string, value: string): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(PREFIX + key, value);
  } catch {
    // Preferences are optional and must never break the UI.
  }
}
