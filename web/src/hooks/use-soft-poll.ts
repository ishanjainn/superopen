"use client";

import { useCallback, useEffect, useRef } from "react";

/**
 * Soft-poll while the tab is visible so Sessions / Memory / Recs / Evals
 * pick up disk updates from hooks without a full remount.
 */
export function useSoftPoll(
  refresh: () => void | Promise<void>,
  intervalMs = 8000,
  enabled = true
) {
  const fn = useRef(refresh);
  fn.current = refresh;

  const tick = useCallback(() => {
    if (typeof document !== "undefined" && document.visibilityState === "hidden") {
      return;
    }
    void fn.current();
  }, []);

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
