"use client";

import { useEffect, useRef, type ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import "./session-rail.css";

export interface SessionRailTool {
  id: string;
  label: string;
  hint: string;
  icon: LucideIcon;
  active?: boolean;
  badge?: "running" | "done" | "stale" | "failed" | null;
  onClick: () => void;
  disabled?: boolean;
}

export interface SessionRailProps {
  /** Optional Chat/Map-style segmented control (e.g. Tree / Terrain). */
  viewToggle?: ReactNode;
  tools?: SessionRailTool[];
  /** Compact tool body rendered inside the rail. */
  panel?: ReactNode;
  onClosePanel?: () => void;
  /** Body shown when no panel is open (stats, filters summary, etc.). */
  children?: ReactNode;
  locked?: boolean;
  /** Skip the outer host; parent provides positioning (Map `.hud`). */
  bare?: boolean;
  /** Horizontal chrome (shared session top bar). */
  layout?: "vertical" | "horizontal";
  hostClassName?: string;
  "aria-label"?: string;
}

/**
 * Shared right-rail chrome for Session Chat and Map.
 * Tools open a compact drawer inside the box; body shows when closed.
 */
export function SessionRail({
  viewToggle,
  tools = [],
  panel,
  onClosePanel,
  children,
  locked = false,
  bare = false,
  layout = "vertical",
  hostClassName,
  "aria-label": ariaLabel = "Session rail",
}: SessionRailProps) {
  const panelOpen = Boolean(panel);
  const blockRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (!panelOpen || !onClosePanel) return;
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Node;
      if (blockRef.current?.contains(target)) return;
      onClosePanel();
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClosePanel();
    };
    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [panelOpen, onClosePanel]);

  const rail = (
    <aside
      ref={blockRef}
      className={
        [
          panelOpen ? "title-block title-block-open" : "title-block",
          layout === "horizontal" ? "title-block-horizontal" : "",
        ]
          .filter(Boolean)
          .join(" ")
      }
      aria-label={ariaLabel}
    >
      {(viewToggle || tools.length > 0) && (
        <div className="tb-band tb-chrome">
          {viewToggle ? (
            <div className="tb-cell tb-shrink">{viewToggle}</div>
          ) : null}
          {tools.length > 0 ? (
            <div className="tb-cell tb-shrink tb-tools-cell">
              <nav className="tb-tools" aria-label="Session panels">
                {tools.map((tool) => {
                  const Icon = tool.icon;
                  return (
                    <button
                      key={tool.id}
                      type="button"
                      className={tool.active ? "active" : undefined}
                      aria-pressed={!!tool.active}
                      title={tool.hint}
                      aria-label={tool.hint}
                      onClick={tool.onClick}
                      disabled={locked || tool.disabled}
                    >
                      <Icon size={16} strokeWidth={1.75} />
                      <span className="tb-tool-label">{tool.label}</span>
                      {tool.badge ? (
                        <span
                          className={`tb-tool-dot tb-tool-dot-${tool.badge}`}
                        />
                      ) : null}
                    </button>
                  );
                })}
              </nav>
            </div>
          ) : null}
        </div>
      )}

      {panelOpen ? <div className="tb-drawer">{panel}</div> : children}
    </aside>
  );

  if (bare) return rail;

  return (
    <div
      className={["session-rail-host", "session-rail-host-fill", hostClassName]
        .filter(Boolean)
        .join(" ")}
    >
      {rail}
    </div>
  );
}

export function SessionRailDrawer({
  title,
  onClose,
  children,
}: {
  title: string;
  onClose: () => void;
  children: ReactNode;
}) {
  return (
    <div className="tb-drawer-scroll" aria-label={title}>
      <div className="tb-drawer-head">
        <div className="tb-drawer-title">{title}</div>
        <button
          type="button"
          className="tb-drawer-close"
          onClick={onClose}
          aria-label={`Close ${title}`}
        >
          ×
        </button>
      </div>
      {children}
    </div>
  );
}
