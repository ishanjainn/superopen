"use client";

import { useEffect, useId, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { Check, ChevronDown } from "lucide-react";
import { useProject } from "@/components/shell/project-context";
import { useBreadcrumb } from "@/components/shell/breadcrumb-context";
import { cn } from "@/lib/utils";
import { playgroundTopBarClassName } from "./sidebar-layout-context";

const TITLES: Record<string, string> = {
  "/sessions": "Sessions",
  "/memory": "Memory",
  "/graph": "Graph",
  "/knowledge": "Knowledge",
  "/docs": "Knowledge",
  "/context": "Knowledge",
  "/rules": "Rules",
  "/skills": "Skills",
  "/guardrails": "Guardrails",
  "/evaluations": "Evaluations",
  "/evals": "Evaluations",
  "/recs": "Recommendations",
  "/settings": "Settings",
};

function pageTitle(pathname: string) {
  if (pathname.startsWith("/sessions/")) return "Sessions";
  for (const [prefix, title] of Object.entries(TITLES)) {
    if (pathname === prefix || pathname.startsWith(prefix + "/")) return title;
  }
  return "Superopen";
}

/** Section root path for breadcrumb up-navigation (never the detail pathname). */
function sectionRoot(pathname: string): string {
  if (pathname.startsWith("/sessions")) return "/sessions";
  if (pathname.startsWith("/recs")) return "/recs";
  if (pathname.startsWith("/guardrails")) return "/guardrails";
  if (pathname.startsWith("/evaluations") || pathname.startsWith("/evals")) {
    return "/evaluations";
  }
  if (pathname.startsWith("/knowledge") || pathname.startsWith("/docs") || pathname.startsWith("/context")) {
    return "/knowledge";
  }
  for (const prefix of Object.keys(TITLES)) {
    if (pathname === prefix || pathname.startsWith(prefix + "/")) return prefix;
  }
  return pathname;
}

type Option = { value: string; label: string };

function ProjectSelect({
  value,
  options,
  disabled,
  onChange,
}: {
  value: string;
  options: Option[];
  disabled?: boolean;
  onChange: (value: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const listId = useId();
  const selected = options.find((o) => o.value === value) || options[0];

  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", onDoc);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDoc);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  return (
    <div ref={rootRef} className="relative min-w-0 max-w-[18rem] shrink">
      <button
        type="button"
        disabled={disabled}
        aria-label="Project"
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={listId}
        onClick={() => setOpen((v) => !v)}
        className={cn(
          "inline-flex h-7 max-w-[18rem] items-center gap-1 bg-transparent py-0 pl-0 pr-0.5 text-xs font-medium text-neutral-700 outline-none transition-colors",
          "hover:text-neutral-950",
          "focus-visible:text-neutral-950",
          open && "text-neutral-950",
          disabled && "opacity-60"
        )}
      >
        <span className="min-w-0 truncate">{selected?.label}</span>
        <ChevronDown
          className={cn(
            "size-3 shrink-0 text-neutral-400 transition-transform",
            open && "rotate-180"
          )}
          aria-hidden
        />
      </button>

      {open ? (
        <ul
          id={listId}
          role="listbox"
          aria-label="Project"
          className="absolute left-0 top-[calc(100%+4px)] z-50 min-w-[14rem] max-w-[22rem] overflow-hidden rounded-md border border-neutral-200 bg-white py-1 shadow-lg shadow-neutral-200/60"
        >
          {options.map((opt) => {
            const isActive = opt.value === value;
            return (
              <li
                key={opt.value || "__current"}
                role="option"
                aria-selected={isActive}
              >
                <button
                  type="button"
                  onClick={() => {
                    onChange(opt.value);
                    setOpen(false);
                  }}
                  className={cn(
                    "flex w-full items-center gap-2 px-2.5 py-1.5 text-left text-xs transition-colors",
                    isActive
                      ? "bg-neutral-100 font-medium text-neutral-900"
                      : "text-neutral-700 hover:bg-neutral-50"
                  )}
                >
                  <Check
                    className={cn(
                      "size-3.5 shrink-0 text-neutral-800",
                      !isActive && "opacity-0"
                    )}
                    aria-hidden
                  />
                  <span className="min-w-0 truncate">{opt.label}</span>
                </button>
              </li>
            );
          })}
        </ul>
      ) : null}
    </div>
  );
}

/** breadcrumb row with global project selector. */
export function HeaderContextRow() {
  const pathname = usePathname();
  const { projects, projectId, setProjectId, currentSlug, ready } = useProject();
  const { crumb } = useBreadcrumb();
  const title = pageTitle(pathname);
  const root = sectionRoot(pathname);
  const hasObject = Boolean(crumb?.label);

  const options = useMemo<Option[]>(() => {
    const currentLabel = currentSlug || "repo";
    const others = projects
      .filter((p) => {
        // Don't duplicate the current workspace under a second label
        if (!p.slug && !p.name) return true;
        const label = p.slug || p.name;
        return label !== currentLabel;
      })
      .map((p) => ({
        value: p.id,
        label: `${p.slug || p.name}${p.missing ? " (missing)" : ""}`,
      }));
    return [
      { value: "", label: currentLabel },
      { value: "all", label: "All projects" },
      ...others,
    ];
  }, [projects, currentSlug]);

  return (
    <div
      className={playgroundTopBarClassName(
        "min-w-0 flex-1 flex-wrap items-center gap-x-2 gap-y-0.5 pl-7 pr-3"
      )}
    >
      <ProjectSelect
        value={projectId}
        options={options}
        disabled={!ready}
        onChange={setProjectId}
      />
      <span className="shrink-0 text-xs text-neutral-400">/</span>
      <Link
        href={root}
        className={cn(
          "max-w-56 truncate text-xs",
          hasObject
            ? "font-medium text-neutral-600 hover:text-neutral-900"
            : "font-semibold text-neutral-900"
        )}
      >
        {title}
      </Link>
      {hasObject ? (
        <>
          <span className="shrink-0 text-xs text-neutral-400">/</span>
          {crumb?.href ? (
            <Link
              href={crumb.href}
              className="max-w-72 truncate text-xs font-semibold text-neutral-900"
            >
              {crumb.label}
            </Link>
          ) : (
            <span className="max-w-72 truncate text-xs font-semibold text-neutral-900">
              {crumb!.label}
            </span>
          )}
        </>
      ) : null}
    </div>
  );
}
