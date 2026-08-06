"use client";

import { useEffect, useRef } from "react";

/**
 * Keeps a single Graphify iframe instance alive across Graph route mounts so
 * revisiting /graph does not re-download / re-simulate the network.
 */
let retained: HTMLDivElement | null = null;
let retainedSrc = "";

export function GraphifyFrame({ src }: { src: string }) {
  const slotRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const slot = slotRef.current;
    if (!slot) return;

    if (!retained || retainedSrc !== src) {
      retained?.remove();
      retained = document.createElement("div");
      retained.style.cssText = "position:absolute;inset:0;width:100%;height:100%;";
      const iframe = document.createElement("iframe");
      iframe.title = "Graphify";
      iframe.src = src;
      iframe.className = "absolute inset-0 h-full w-full border-0";
      iframe.setAttribute("sandbox", "allow-scripts allow-same-origin allow-popups");
      retained.appendChild(iframe);
      retainedSrc = src;
    }

    slot.appendChild(retained);
    return () => {
      // Detach but keep the iframe in memory for the next visit.
      if (retained && retained.parentNode === slot) {
        slot.removeChild(retained);
      }
    };
  }, [src]);

  return (
    <div
      ref={slotRef}
      className="absolute inset-0 bg-white"
      aria-label="Graphify architecture map"
    />
  );
}
