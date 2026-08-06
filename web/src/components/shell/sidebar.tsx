"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import { ChevronRight, Search, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { SIDEBAR_ITEMS } from "@/constants/sidebar";
import { cn } from "@/lib/utils";
import type {
  SidebarActionItem,
  SidebarItemProps,
  SidebarSection,
} from "@/types/sidebar";
import { useSidebarLayout } from "./sidebar-layout-context";

const SECONDARY_PANEL_WIDTH = "min(20rem, calc(100vw - 4rem))";

const isActive = (pathname: string, item: SidebarActionItem) => {
  if (!item.link) return false;
  if (item.link === "/sessions") {
    return pathname === "/sessions" || pathname.startsWith("/sessions/");
  }
  return pathname === item.link || pathname.startsWith(item.link + "/");
};

const flatItems = (items: SidebarItemProps[]) =>
  items.flatMap((item) => {
    if (item.type !== "section") return [item];
    const direct = item.children || [];
    const grouped = (item.groups || []).flatMap((group) => group.children);
    return [...direct, ...grouped];
  });

function NavigationLink({
  item,
  active,
  onNavigate,
  compact = false,
}: {
  item: SidebarActionItem;
  active: boolean;
  onNavigate?: () => void;
  compact?: boolean;
}) {
  const content = (
    <>
      {item.icon}
      <span className={cn("min-w-0 truncate", compact && "sr-only")}>{item.text}</span>
    </>
  );
  const className = cn(
    "flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left text-[13px] font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-stone-950 focus-visible:ring-offset-2 focus-visible:ring-offset-white",
    compact && "justify-center px-2",
    active
      ? "bg-stone-200 text-stone-950"
      : "text-stone-600 hover:bg-stone-200/70 hover:text-stone-950"
  );

  if (item.link) {
    return (
      <Link
        href={item.link}
        onClick={onNavigate}
        aria-current={active ? "page" : undefined}
        className={className}
      >
        {content}
      </Link>
    );
  }
  return (
    <button type="button" onClick={onNavigate} className={className}>
      {content}
    </button>
  );
}

function PrimaryItem({
  item,
  pathname,
  compact,
  openSection,
  setOpenSection,
}: {
  item: SidebarItemProps;
  pathname: string;
  compact: boolean;
  openSection: string | null;
  setOpenSection: (section: SidebarSection | null) => void;
}) {
  if (item.type === "action") {
    const active = openSection ? false : isActive(pathname, item);
    const link = (
      <NavigationLink
        item={item}
        active={active}
        compact={compact}
        onNavigate={() => setOpenSection(null)}
      />
    );
    return compact ? (
      <Tooltip delayDuration={100}>
        <TooltipTrigger asChild>{link}</TooltipTrigger>
        <TooltipContent side="right" sideOffset={8}>
          {item.text}
        </TooltipContent>
      </Tooltip>
    ) : (
      link
    );
  }

  const selected = openSection === item.title;
  return (
    <Tooltip delayDuration={100}>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          aria-expanded={openSection === item.title}
          onClick={() => setOpenSection(openSection === item.title ? null : item)}
          className={cn(
            "h-9 w-full justify-start gap-2 rounded-lg px-2.5 text-[13px] font-medium",
            compact && "justify-center px-2",
            selected
              ? "bg-stone-200 text-stone-950 hover:bg-stone-200"
              : "text-stone-600 hover:bg-stone-200/70 hover:text-stone-950"
          )}
        >
          {compact ? (
            item.icon ?? <span className="text-[13px] font-bold">{item.title.slice(0, 1)}</span>
          ) : (
            <>
              {item.icon}
              <span>{item.title}</span>
              <ChevronRight className="ml-auto size-4" />
            </>
          )}
        </Button>
      </TooltipTrigger>
      {compact && (
        <TooltipContent side="right" sideOffset={8}>
          {item.title}
        </TooltipContent>
      )}
    </Tooltip>
  );
}

function SectionPanel({
  section,
  pathname,
  onClose,
}: {
  section: SidebarSection;
  pathname: string;
  onClose: () => void;
}) {
  const children = section.children || [];
  const groups = section.groups;

  return (
    <div
      className="absolute inset-y-0 left-full z-40 flex flex-col border-y border-r border-neutral-200 bg-white shadow-xl"
      style={{ width: SECONDARY_PANEL_WIDTH }}
    >
      <div className="flex items-center justify-between border-b border-stone-200 px-3 py-2.5">
        <div className="flex items-center gap-2 text-stone-900">
          {section.icon}
          <p className="text-[15px] font-semibold">{section.title}</p>
        </div>
        <Button
          variant="ghost"
          size="icon"
          onClick={onClose}
          aria-label="Close navigation panel"
          className="size-8 text-stone-700 hover:bg-stone-200"
        >
          <X className="size-4" />
        </Button>
      </div>
      <div className="scrollbar-hidden overflow-y-auto p-2">
        {groups ? (
          <div className="space-y-3">
            {groups.map((group) => (
              <div key={group.title} className="space-y-0.5">
                <p className="px-2.5 pb-0.5 pt-1 text-xs font-semibold tracking-wide text-stone-500">
                  {group.title}
                </p>
                {group.children.map((item) => (
                  <NavigationLink
                    key={item.text}
                    item={item}
                    active={isActive(pathname, item)}
                    onNavigate={onClose}
                  />
                ))}
              </div>
            ))}
          </div>
        ) : (
          <div className="space-y-1">
            {children.map((item) => (
              <NavigationLink
                key={item.text}
                item={item}
                active={isActive(pathname, item)}
                onNavigate={onClose}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

export default function Sidebar() {
  const pathname = usePathname();
  const router = useRouter();
  const { isExpanded } = useSidebarLayout();
  const [openSection, setOpenSection] = useState<SidebarSection | null>(null);
  const [commandOpen, setCommandOpen] = useState(false);
  const allItems = useMemo(() => flatItems(SIDEBAR_ITEMS), []);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setCommandOpen(true);
      }
      if (event.key === "Escape") setOpenSection(null);
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  useEffect(() => {
    fetch("/api/ui/prefs", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ key: "recent_route", value: pathname }),
    }).catch(() => undefined);
  }, [pathname]);

  return (
    <aside
      aria-label="Main navigation"
      className="relative flex h-full min-h-0 w-full flex-col bg-white"
    >
      {openSection && (
        <button
          aria-label="Close navigation panel"
          className={cn(
            "fixed inset-y-0 right-0 z-30 cursor-default bg-black/20",
            isExpanded ? "left-64" : "left-16"
          )}
          onClick={() => setOpenSection(null)}
        />
      )}
      <div
        data-state={isExpanded ? "open" : "closed"}
        className="relative z-40 flex h-full min-h-0 flex-col"
      >
        <div className="px-2 pb-3 pt-3">
          <Button
            variant="outline"
            onClick={() => setCommandOpen(true)}
            className={cn(
              "h-9 w-full justify-start gap-2 border-stone-200 bg-transparent px-2.5 text-[13px] text-stone-500 shadow-none hover:bg-stone-200/60",
              !isExpanded && "justify-center px-2"
            )}
            aria-label="Search navigation"
          >
            <Search className="size-4 shrink-0" />
            <span className={cn("flex-1 text-left", !isExpanded && "hidden")}>
              Search
            </span>
            <kbd
              className={cn(
                "rounded border border-stone-200 px-1 py-0.5 text-[10px]",
                !isExpanded && "hidden"
              )}
            >
              ⌘K
            </kbd>
          </Button>
        </div>

        <nav
          className="scrollbar-hidden flex grow flex-col gap-1 overflow-y-auto px-2 py-2"
          aria-label="Product navigation"
        >
          <div className="flex w-full shrink-0 flex-col">
            {SIDEBAR_ITEMS.map((item) => (
              <PrimaryItem
                key={item.type === "section" ? item.title : item.text}
                item={item}
                pathname={pathname}
                compact={!isExpanded}
                openSection={openSection?.title || null}
                setOpenSection={setOpenSection}
              />
            ))}
          </div>
        </nav>
      </div>

      {openSection && (
        <SectionPanel
          section={openSection}
          pathname={pathname}
          onClose={() => setOpenSection(null)}
        />
      )}

      <CommandDialog open={commandOpen} onOpenChange={setCommandOpen}>
        <CommandInput placeholder="Search Superopen navigation..." />
        <CommandList>
          <CommandEmpty>No navigation items found.</CommandEmpty>
          <CommandGroup heading="Navigation">
            {allItems
              .filter((item) => item.link)
              .map((item) => (
                <CommandItem
                  key={item.text}
                  value={item.text}
                  onSelect={() => {
                    setCommandOpen(false);
                    if (item.link) router.push(item.link);
                  }}
                >
                  {item.icon}
                  <span>{item.text}</span>
                </CommandItem>
              ))}
          </CommandGroup>
        </CommandList>
      </CommandDialog>
    </aside>
  );
}
