"use client";

import type { ReactNode } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { ArrowLeft, LayoutDashboard } from "lucide-react";
import { SIDEBAR_ITEMS } from "@/constants/sidebar";
import { cn } from "@/lib/utils";
import type { SidebarActionItem } from "@/types/sidebar";

type FeaturePageHeaderProps = {
  title: string;
  /** Override auto icon from the sidebar nav */
  icon?: ReactNode;
  tone?: string;
  leading?: ReactNode;
  actions?: ReactNode;
  className?: string;
};

/** Matches the page icon chip (`size-7`) so back controls align with the header icon. */
export function FeatureBackLink({
  href,
  label,
}: {
  href: string;
  label: string;
}) {
  return (
    <Link
      href={href}
      aria-label={label}
      className="inline-flex size-7 shrink-0 items-center justify-center rounded-md border border-neutral-200 text-neutral-600 hover:bg-neutral-50 hover:text-neutral-900"
    >
      <ArrowLeft className="size-4" />
    </Link>
  );
}

function iconForPath(pathname: string): ReactNode | null {
  const actions = SIDEBAR_ITEMS.filter(
    (item): item is SidebarActionItem => item.type === "action" && Boolean(item.link)
  );
  // Longest link prefix wins (e.g. /sessions/xyz → Sessions)
  const match = actions
    .filter((item) => {
      const link = item.link!;
      if (link === "/sessions") {
        return pathname === "/sessions" || pathname.startsWith("/sessions/");
      }
      return pathname === link || pathname.startsWith(link + "/");
    })
    .sort((a, b) => (b.link?.length || 0) - (a.link?.length || 0))[0];
  return match?.icon ?? null;
}

function iconForTitle(title: string): ReactNode | null {
  const hit = SIDEBAR_ITEMS.find(
    (item) => item.type === "action" && item.text === title
  );
  return hit && hit.type === "action" ? hit.icon : null;
}

/** Page title bar under the breadcrumb - height, icon + title. */
export default function FeaturePageHeader({
  title,
  icon,
  tone = "border-neutral-200 bg-stone-100 text-stone-700",
  leading,
  actions,
  className,
}: FeaturePageHeaderProps) {
  const pathname = usePathname() || "";
  const resolved =
    icon ?? iconForPath(pathname) ?? iconForTitle(title) ?? (
      <LayoutDashboard className="size-4" />
    );

  return (
    <section
      className={cn(
        "flex h-12 shrink-0 items-center justify-between gap-3 border-b border-neutral-200 bg-white px-5",
        className
      )}
    >
      <div className="flex min-w-0 items-center gap-2.5">
        {leading ? <div className="shrink-0">{leading}</div> : null}
        <span
          className={cn(
            "inline-flex size-7 shrink-0 items-center justify-center rounded-md border",
            tone
          )}
          aria-hidden
        >
          {resolved}
        </span>
        <h1 className="truncate text-[15px] font-semibold leading-none tracking-tight text-neutral-900">
          {title}
        </h1>
      </div>
      {actions ? (
        <div className="flex shrink-0 items-center gap-2">{actions}</div>
      ) : null}
    </section>
  );
}
