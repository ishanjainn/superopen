"use client";

import { useEffect, useState } from "react";
import { ChevronsLeft, ChevronsRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { displayVersion } from "@/lib/version";
import {
  playgroundTopBarClassName,
  useSidebarLayout,
} from "./sidebar-layout-context";

export default function SidebarBrand() {
  const { isExpanded, toggleSidebar } = useSidebarLayout();
  const [version, setVersion] = useState("");

  useEffect(() => {
    let cancelled = false;
    void fetch("/api/meta")
      .then((res) => (res.ok ? res.json() : null))
      .then((body) => {
        if (cancelled) return;
        const raw =
          (typeof body?.version_display === "string" && body.version_display) ||
          (typeof body?.version === "string" && body.version) ||
          "";
        setVersion(displayVersion(raw || undefined));
      })
      .catch(() => {
        if (!cancelled) setVersion(displayVersion());
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <div className={playgroundTopBarClassName("relative gap-2 px-3")}>
      <Tooltip delayDuration={100}>
        <TooltipTrigger asChild>
          <Button
            variant="outline"
            size="icon"
            onClick={toggleSidebar}
            aria-label={isExpanded ? "Collapse sidebar" : "Expand sidebar"}
            aria-expanded={isExpanded}
            className="absolute -right-3 top-1/2 z-50 size-6 -translate-y-1/2 rounded-full border-neutral-300 bg-white p-0 text-neutral-600 shadow-sm hover:bg-neutral-50 hover:text-neutral-900 focus-visible:ring-2 focus-visible:ring-neutral-400"
          >
            {isExpanded ? (
              <ChevronsLeft className="size-3.5" />
            ) : (
              <ChevronsRight className="size-3.5" />
            )}
          </Button>
        </TooltipTrigger>
        <TooltipContent side="right" sideOffset={10}>
          {isExpanded ? "Collapse sidebar" : "Expand sidebar"}
        </TooltipContent>
      </Tooltip>
      {/* The mark keeps its slot in both states; expanding only reveals the
          wordmark and version beside it. */}
      <div className="flex min-w-0 flex-1 items-center gap-2">
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img
          src="/brand-mark.png"
          alt="Superopen"
          className="size-8 shrink-0 object-contain"
        />
        {isExpanded ? (
          <div className="flex min-w-0 items-end gap-1.5">
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img
              src="/brand-wordmark.png"
              alt="superopen"
              className="h-5 w-auto min-w-0 max-w-[9rem] object-contain object-left"
            />
            {version ? (
              <span
                className="mb-[3px] shrink-0 text-[10px] leading-none text-neutral-400"
                title={`Superopen ${version}`}
              >
                {version}
              </span>
            ) : null}
          </div>
        ) : null}
      </div>
    </div>
  );
}
