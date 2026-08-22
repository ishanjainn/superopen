"use client";

import type { CSSProperties, ReactNode } from "react";
import { cn } from "@/lib/utils";
import { StageBackdrop } from "./StageBackdrop";

/**
 * Shared paper for Graph, Memory, and session map. The backdrop is the only
 * stage fill + grid; WebGL children must clear with alpha 0 so this shows
 * through.
 */
export function StageViewport({
  children,
  className,
  style,
}: {
  children: ReactNode;
  className?: string;
  style?: CSSProperties;
}) {
  return (
    <div className={cn("stage-viewport", className)} style={style}>
      <StageBackdrop />
      {children}
    </div>
  );
}
