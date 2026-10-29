"use client";

import { cn } from "@/lib/utils";

export type TabItem<T extends string> = { value: T; label: string };

/**
 * Page-level tabs: plain text, the selected one underlined. Not pills, which
 * read as filters or toggles rather than as views of the same page.
 */
export default function Tabs<T extends string>({
  items,
  value,
  onChange,
  label,
  className,
}: {
  items: TabItem<T>[];
  value: T;
  onChange: (value: T) => void;
  label: string;
  className?: string;
}) {
  return (
    <div
      role="tablist"
      aria-label={label}
      className={cn("flex shrink-0 gap-5 border-b border-neutral-200 px-5", className)}
    >
      {items.map((item) => {
        const selected = item.value === value;
        return (
          <button
            key={item.value}
            type="button"
            role="tab"
            aria-selected={selected}
            onClick={() => onChange(item.value)}
            className={cn(
              "-mb-px border-b-2 py-2.5 text-sm transition-colors",
              selected
                ? "border-neutral-900 font-medium text-neutral-900"
                : "border-transparent text-neutral-500 hover:text-neutral-800",
            )}
          >
            {item.label}
          </button>
        );
      })}
    </div>
  );
}
