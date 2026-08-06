"use client";

import type { LucideIcon } from "lucide-react";
import { useEffect, useRef, type ReactNode } from "react";

export type PanelPresentation = "sheet" | "pop";
export type PanelSection = "scene" | "session";
export type PanelBadge = "running" | "done" | "stale" | "failed";

/**
 * One dock panel, declared as data. Tool triggers live in the title block;
 * Dock only mounts the open sheet / pop content.
 */
export interface PanelDescriptor {
  id: string;
  icon: LucideIcon;
  hint: string;
  /** Short label for the title-block tools row. */
  label?: string;
  /** scene = how the stage renders; session = depth content about the trace */
  section: PanelSection;
  /** sheet = full-height paper, one at a time; pop = compact card */
  presentation: PanelPresentation;
  badge?: PanelBadge | null;
  render: () => ReactNode;
}

interface DockProps {
  panels: PanelDescriptor[];
  openSheet: string | null;
  openPop: string | null;
  onClosePop: () => void;
  onCloseSheet: () => void;
}

/** Sheets / pops only - no floating icon strip. */
export function Dock({
  panels,
  openSheet,
  openPop,
  onClosePop,
  onCloseSheet,
}: DockProps) {
  const popRef = useRef<HTMLDivElement | null>(null);
  const sheet = panels.find(
    (panel) => panel.id === openSheet && panel.presentation === "sheet"
  );
  const pop = panels.find(
    (panel) => panel.id === openPop && panel.presentation === "pop"
  );

  useEffect(() => {
    if (!pop && !sheet) return;
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Node;
      if (popRef.current?.contains(target)) return;
      const sheetEl = document.querySelector(".map-root .dock-panel");
      if (sheetEl?.contains(target)) return;
      if ((target as Element).closest?.(".tb-tools, .title-block")) return;
      onClosePop();
      onCloseSheet();
    };
    const onEsc = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      onClosePop();
      onCloseSheet();
    };
    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("keydown", onEsc);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("keydown", onEsc);
    };
  }, [pop, sheet, onClosePop, onCloseSheet]);

  if (!pop && !sheet) return null;

  return (
    <div className="dock dock-sheets-only">
      {pop ? (
        <div className="dock-pop" ref={popRef}>
          {pop.render()}
        </div>
      ) : null}
      {sheet ? <aside className="dock-panel">{sheet.render()}</aside> : null}
    </div>
  );
}
