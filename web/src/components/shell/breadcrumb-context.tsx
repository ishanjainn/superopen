"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";

export type BreadcrumbCrumb = {
  label: string;
  href?: string;
};

type BreadcrumbContextValue = {
  crumb: BreadcrumbCrumb | null;
  setCrumb: (crumb: BreadcrumbCrumb | null) => void;
};

const BreadcrumbContext = createContext<BreadcrumbContextValue | null>(null);

export function BreadcrumbProvider({ children }: { children: ReactNode }) {
  const [crumb, setCrumbState] = useState<BreadcrumbCrumb | null>(null);
  const setCrumb = useCallback((next: BreadcrumbCrumb | null) => {
    setCrumbState(next);
  }, []);
  const value = useMemo(() => ({ crumb, setCrumb }), [crumb, setCrumb]);
  return (
    <BreadcrumbContext.Provider value={value}>{children}</BreadcrumbContext.Provider>
  );
}

export function useBreadcrumb() {
  const ctx = useContext(BreadcrumbContext);
  if (!ctx) {
    throw new Error("useBreadcrumb must be used within BreadcrumbProvider");
  }
  return ctx;
}

/** Publish the current object crumb while this component is mounted. */
export function useBreadcrumbCrumb(label: string | null | undefined) {
  const ctx = useContext(BreadcrumbContext);
  const setCrumb = ctx?.setCrumb;
  useEffect(() => {
    if (!setCrumb) return;
    if (!label) {
      setCrumb(null);
      return;
    }
    setCrumb({ label });
    return () => setCrumb(null);
  }, [setCrumb, label]);
}
