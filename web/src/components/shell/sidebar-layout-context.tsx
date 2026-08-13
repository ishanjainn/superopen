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
import { cn } from "@/lib/utils";
import { getUIPref, setUIPref } from "@/lib/ui-prefs";

type SidebarLayoutContextValue = {
  isExpanded: boolean;
  toggleSidebar: () => void;
  expandSidebar: () => void;
  sidebarWidthClass: string;
};

const SidebarLayoutContext = createContext<SidebarLayoutContextValue | null>(null);

async function loadExpanded(): Promise<boolean> {
	const value = getUIPref("sidebar_expanded");
	return value !== "0" && value !== "false";
}

async function saveExpanded(expanded: boolean) {
	setUIPref("sidebar_expanded", expanded ? "1" : "0");
}

export function SidebarLayoutProvider({ children }: { children: ReactNode }) {
  const [isExpanded, setIsExpanded] = useState(true);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    loadExpanded().then((v) => {
      setIsExpanded(v);
      setReady(true);
    });
  }, []);

  const toggleSidebar = useCallback(() => {
    setIsExpanded((value) => {
      const next = !value;
      void saveExpanded(next);
      return next;
    });
  }, []);

  const expandSidebar = useCallback(() => {
    setIsExpanded(true);
    void saveExpanded(true);
  }, []);

  const value = useMemo(
    () => ({
      isExpanded,
      toggleSidebar,
      expandSidebar,
      sidebarWidthClass: isExpanded ? "w-64" : "w-16",
    }),
    [isExpanded, toggleSidebar, expandSidebar]
  );

  if (!ready) {
    return (
      <SidebarLayoutContext.Provider value={value}>
        {children}
      </SidebarLayoutContext.Provider>
    );
  }

  return (
    <SidebarLayoutContext.Provider value={value}>{children}</SidebarLayoutContext.Provider>
  );
}

export function useSidebarLayout() {
  const context = useContext(SidebarLayoutContext);
  if (!context) {
    throw new Error("useSidebarLayout must be used within SidebarLayoutProvider");
  }
  return context;
}

const PLAYGROUND_TOP_BAR_CLASS = "flex h-11 shrink-0 items-center";

export function playgroundTopBarClassName(className?: string) {
  return cn(PLAYGROUND_TOP_BAR_CLASS, className);
}
