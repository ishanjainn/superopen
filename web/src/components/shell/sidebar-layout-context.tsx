"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { cn } from "@/lib/utils";
import { getUIPref, setUIPref } from "@/lib/ui-prefs";

type SidebarLayoutContextValue = {
  isExpanded: boolean;
  isPinned: boolean;
  toggleSidebar: () => void;
  togglePin: () => void;
  expandSidebar: () => void;
  setSidebarHover: (hover: boolean) => void;
  sidebarWidthClass: string;
};

const SidebarLayoutContext = createContext<SidebarLayoutContextValue | null>(null);

function loadPinned(): boolean {
	const pinned = getUIPref("sidebar_pinned");
	if (pinned === "1" || pinned === "true") return true;
	if (pinned === "0" || pinned === "false") return false;
	// Older builds stored an explicit expand toggle. Honor that once.
	const legacy = getUIPref("sidebar_expanded");
	return legacy === "1" || legacy === "true";
}

function savePinned(pinned: boolean) {
	setUIPref("sidebar_pinned", pinned ? "1" : "0");
}

export function SidebarLayoutProvider({ children }: { children: ReactNode }) {
  const [isPinned, setIsPinned] = useState(false);
  const [hovered, setHovered] = useState(false);
  const [ready, setReady] = useState(false);
  const hoverTimer = useRef<number | null>(null);
  const isExpanded = isPinned || hovered;

  useEffect(() => {
    setIsPinned(loadPinned());
    setReady(true);
  }, []);

  useEffect(() => {
    return () => {
      if (hoverTimer.current != null) window.clearTimeout(hoverTimer.current);
    };
  }, []);

  const togglePin = useCallback(() => {
    setIsPinned((value) => {
      const next = !value;
      savePinned(next);
      return next;
    });
  }, []);

  const expandSidebar = useCallback(() => {
    setIsPinned(true);
    savePinned(true);
  }, []);

  const setSidebarHover = useCallback((hover: boolean) => {
    if (hoverTimer.current != null) {
      window.clearTimeout(hoverTimer.current);
      hoverTimer.current = null;
    }
    if (hover) {
      setHovered(true);
      return;
    }
    hoverTimer.current = window.setTimeout(() => {
      setHovered(false);
      hoverTimer.current = null;
    }, 120);
  }, []);

  const value = useMemo(
    () => ({
      isExpanded,
      isPinned,
      toggleSidebar: togglePin,
      togglePin,
      expandSidebar,
      setSidebarHover,
      sidebarWidthClass: isExpanded ? "w-64" : "w-16",
    }),
    [isExpanded, isPinned, togglePin, expandSidebar, setSidebarHover]
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
