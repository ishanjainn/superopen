"use client";

import { useEffect, useState } from "react";
import { LoaderCircle } from "lucide-react";

/**
 * Hosts Graphify in a short-lived iframe.
 *
 * Large Graphify exports run an expensive vis-network stabilization pass. The
 * iframe must be destroyed when the route unmounts so that work and memory do
 * not follow the user around the rest of the UI. Mounting one frame later also
 * gives React a chance to paint the shell and loading indicator first.
 */
export function GraphifyFrame({ src }: { src: string }) {
  const [mounted, setMounted] = useState(false);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    const timer = window.setTimeout(() => setMounted(true), 75);
    return () => {
      window.clearTimeout(timer);
      // React removes the iframe and its browsing context on unmount. In
      // particular, no module-level reference retains it between routes.
    };
  }, []);

  useEffect(() => {
    // Graphify's export has no cached layout, so the iframe stabilizes once
    // and reports the result here; /api/graph/html bakes it in next load so
    // returning to this page doesn't re-run the physics pass. Best-effort:
    // a failed POST just means the next visit stabilizes again.
    function onMessage(e: MessageEvent) {
      if (e?.data?.type !== "so-graph-positions") return;
      const positions = e.data.positions;
      if (!positions || typeof positions !== "object") return;
      const project = new URL(src, window.location.origin).searchParams.get("project");
      const url = new URL("/api/graph/positions", window.location.origin);
      if (project) url.searchParams.set("project", project);
      fetch(url.toString(), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ positions }),
      }).catch(() => {});
    }
    window.addEventListener("message", onMessage);
    return () => window.removeEventListener("message", onMessage);
  }, [src]);

  return (
    <div
      className="absolute inset-0 bg-white"
      aria-label="Graphify architecture map"
      aria-busy={!ready}
    >
      {mounted ? (
        <iframe
          title="Graphify"
          src={src}
          className="absolute inset-0 h-full w-full border-0"
          sandbox="allow-scripts allow-same-origin allow-popups"
          onLoad={() => setReady(true)}
        />
      ) : null}

      {!ready ? (
        <div
          className="absolute inset-0 z-10 grid place-items-center bg-white"
          role="status"
          aria-live="polite"
        >
          <div className="flex flex-col items-center gap-3 text-center">
            <LoaderCircle className="size-5 animate-spin text-neutral-500" />
            <div>
              <p className="text-sm font-medium text-neutral-700">
                Loading Graphify graph…
              </p>
              <p className="mt-1 text-xs text-neutral-500">
                Large repository graphs can take a moment to lay out.
              </p>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}
