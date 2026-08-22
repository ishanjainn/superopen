"use client";

import { useCallback, useEffect } from "react";
import { useLatestRef } from "./use-latest-ref";

/**
 * Soft-poll while the tab is visible so session views
 * pick up disk updates from hooks without a full remount.
 */
export function useSoftPoll(
  refresh: () => void | Promise<void>,
  intervalMs = 8000,
  enabled = true
) {
  const fn = useLatestRef(refresh);

  const tick = useCallback(() => {
    if (typeof document !== "undefined" && document.visibilityState === "hidden") {
      return;
    }
    void fn.current();
  }, [fn]);

  useEffect(() => {
    if (!enabled) return;
    const id = window.setInterval(tick, intervalMs);
    const onVis = () => {
      if (document.visibilityState === "visible") tick();
    };
    document.addEventListener("visibilitychange", onVis);
    return () => {
      window.clearInterval(id);
      document.removeEventListener("visibilitychange", onVis);
    };
  }, [enabled, intervalMs, tick]);
}
