"use client";

import { useCallback } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";

function formatSearch(params: URLSearchParams): string {
  // Prefer `?map` over `?map=` for empty flag values
  return params.toString().replace(/=(?=&|$)/g, "");
}

function hrefFor(pathname: string, params: URLSearchParams): string {
  const qs = formatSearch(params);
  return qs ? `${pathname}?${qs}` : pathname;
}

/**
 * Sync a boolean UI mode to a flag query param (e.g. `?map`).
 * When on=true the key is present; when false it is removed.
 * Other params (e.g. project) are preserved.
 */
export function useFlagQueryParam(flag: string): [boolean, (on: boolean) => void] {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const on = searchParams.has(flag);

  const setFlag = useCallback(
    (next: boolean) => {
      const params = new URLSearchParams(searchParams.toString());
      if (next) params.set(flag, "");
      else params.delete(flag);
      router.replace(hrefFor(pathname, params), { scroll: false });
    },
    [flag, pathname, router, searchParams]
  );

  return [on, setFlag];
}

/**
 * Sync a string query param (e.g. `?file=create-api.md`).
 * Pass null/empty to remove the key. Other params are preserved.
 */
export function useStringQueryParam(
  key: string,
  defaultValue = ""
): [string, (value: string | null) => void] {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const value = searchParams.get(key) ?? defaultValue;

  const setParam = useCallback(
    (next: string | null) => {
      const trimmed = (next ?? "").trim();
      const params = new URLSearchParams(searchParams.toString());
      if (trimmed) params.set(key, trimmed);
      else params.delete(key);
      router.replace(hrefFor(pathname, params), { scroll: false });
    },
    [key, pathname, router, searchParams]
  );

  return [value, setParam];
}

/**
 * Exclusive flag tabs: default has no flag; other values set `?value` and clear siblings.
 * Example: default "open", alternatives ["resolved","dismissed"] → `?resolved` / `?dismissed`.
 */
export function useExclusiveFlagTab<T extends string>(
  defaultTab: T,
  alternatives: readonly T[]
): [T, (tab: T) => void] {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();

  let tab = defaultTab;
  for (const alt of alternatives) {
    if (searchParams.has(alt)) {
      tab = alt;
      break;
    }
  }

  const setTab = useCallback(
    (next: T) => {
      const params = new URLSearchParams(searchParams.toString());
      for (const alt of alternatives) params.delete(alt);
      if (next !== defaultTab) params.set(next, "");
      router.replace(hrefFor(pathname, params), { scroll: false });
    },
    [alternatives, defaultTab, pathname, router, searchParams]
  );

  return [tab, setTab];
}
