"use client";

import { cn } from "@/lib/utils";

/**
 * Product paper (fill + grid). Used via StageViewport. Keep behind WebGL (z-0).
 */
export function StageBackdrop({ className }: { className?: string }) {
  return <div className={cn("stage-backdrop", className)} aria-hidden />;
}
