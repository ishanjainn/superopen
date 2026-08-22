"use client";

import {
  createContext,
  useCallback,
  useContext,
  useLayoutEffect,
  useMemo,
  useSyncExternalStore,
  type ReactNode,
} from "react";

export type ThemePreference = "light" | "dark" | "system";
export type ResolvedTheme = "light" | "dark";

const STORAGE_KEY = "so-theme";

type ThemeContextValue = {
  preference: ThemePreference;
  resolved: ResolvedTheme;
  /** False until client has read localStorage / prefs (avoid hydration mismatch). */
  ready: boolean;
  setPreference: (next: ThemePreference) => void;
};

const ThemeContext = createContext<ThemeContextValue | null>(null);

function isPreference(v: unknown): v is ThemePreference {
  return v === "light" || v === "dark" || v === "system";
}

function systemResolved(): ResolvedTheme {
  if (typeof window === "undefined") return "dark";
  return window.matchMedia("(prefers-color-scheme: dark)").matches
    ? "dark"
    : "light";
}

function applyDomTheme(resolved: ResolvedTheme) {
  const root = document.documentElement;
  root.classList.toggle("dark", resolved === "dark");
  root.style.colorScheme = resolved;
}

function readStoredPreference(): ThemePreference {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (isPreference(raw)) return raw;
  } catch {
    /* ignore */
  }
  return "dark";
}

const themeListeners = new Set<() => void>();

function emitTheme() {
  for (const listener of themeListeners) listener();
}

function subscribeTheme(onStoreChange: () => void) {
  themeListeners.add(onStoreChange);
  const mq = window.matchMedia("(prefers-color-scheme: dark)");
  mq.addEventListener("change", onStoreChange);
  window.addEventListener("storage", onStoreChange);
  return () => {
    themeListeners.delete(onStoreChange);
    mq.removeEventListener("change", onStoreChange);
    window.removeEventListener("storage", onStoreChange);
  };
}

const SERVER_THEME: { preference: ThemePreference; resolved: ResolvedTheme } = {
  preference: "dark",
  resolved: "dark",
};
let cachedTheme = SERVER_THEME;

function getThemeSnapshot(): { preference: ThemePreference; resolved: ResolvedTheme } {
  const preference = readStoredPreference();
  const resolved = preference === "system" ? systemResolved() : preference;
  if (
    cachedTheme.preference === preference &&
    cachedTheme.resolved === resolved
  ) {
    return cachedTheme;
  }
  cachedTheme = { preference, resolved };
  return cachedTheme;
}

function getServerThemeSnapshot() {
  return SERVER_THEME;
}

function persist(next: ThemePreference) {
  try {
    localStorage.setItem(STORAGE_KEY, next);
  } catch {
    /* ignore */
  }
}

function subscribeHydrated(onStoreChange: () => void) {
  return () => {
    void onStoreChange;
  };
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const { preference, resolved } = useSyncExternalStore(
    subscribeTheme,
    getThemeSnapshot,
    getServerThemeSnapshot,
  );
  const ready = useSyncExternalStore(subscribeHydrated, () => true, () => false);

  useLayoutEffect(() => {
    applyDomTheme(resolved);
  }, [resolved]);

  const setPreference = useCallback((next: ThemePreference) => {
    persist(next);
    const nextResolved = next === "system" ? systemResolved() : next;
    applyDomTheme(nextResolved);
    emitTheme();
  }, []);

  const value = useMemo(
    () => ({ preference, resolved, ready, setPreference }),
    [preference, resolved, ready, setPreference],
  );

  return (
    <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
  );
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) {
    throw new Error("useTheme must be used within ThemeProvider");
  }
  return ctx;
}

/** Safe for components that may render outside the provider during SSR. */
export function useThemeOptional(): ThemeContextValue | null {
  return useContext(ThemeContext);
}
