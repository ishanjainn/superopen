"use client";

import {
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import { Check, ChevronDown } from "lucide-react";
import { cn } from "@/lib/utils";

type DropdownOption = {
  value: string;
  label: ReactNode;
  disabled?: boolean;
};

type DropdownProps = {
  value: string;
  options: DropdownOption[];
  onChange: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
  className?: string;
  triggerClassName?: string;
  menuClassName?: string;
  "aria-label"?: string;
  title?: string;
  size?: "sm" | "md";
};

type MenuPos = { top: number; left: number; width: number };

/**
 * Product dropdown - never a native `<select>` (OS menus).
 * Menu portals to document.body and is fixed to the trigger rect so it
 * stays aligned inside modals / overflow containers.
 */
export function Dropdown({
  value,
  options,
  onChange,
  placeholder = "Select…",
  disabled = false,
  className,
  triggerClassName,
  menuClassName,
  "aria-label": ariaLabel,
  title,
  size = "md",
}: DropdownProps) {
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState<MenuPos | null>(null);
  const rootRef = useRef<HTMLDivElement | null>(null);
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const menuRef = useRef<HTMLUListElement | null>(null);
  const listId = useId();
  const selected = options.find((o) => o.value === value);

  const updatePos = useCallback(() => {
    const trigger = triggerRef.current;
    if (!trigger) return;
    const rect = trigger.getBoundingClientRect();
    const gap = 4;
    const menuH = menuRef.current?.offsetHeight ?? Math.min(240, options.length * 36 + 8);
    const spaceBelow = window.innerHeight - rect.bottom - gap;
    const openUp = spaceBelow < menuH && rect.top > spaceBelow;
    setPos({
      top: openUp ? Math.max(8, rect.top - gap - menuH) : rect.bottom + gap,
      left: rect.left,
      width: Math.max(rect.width, 8),
    });
  }, [options.length]);

  useLayoutEffect(() => {
    if (!open) return;
    updatePos();
    const id = requestAnimationFrame(() => updatePos());
    return () => cancelAnimationFrame(id);
  }, [open, updatePos, options.length]);

  useEffect(() => {
    if (!open) return;
    const onScrollOrResize = () => updatePos();
    window.addEventListener("resize", onScrollOrResize);
    window.addEventListener("scroll", onScrollOrResize, true);
    return () => {
      window.removeEventListener("resize", onScrollOrResize);
      window.removeEventListener("scroll", onScrollOrResize, true);
    };
  }, [open, updatePos]);

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: PointerEvent) => {
      const t = event.target as Node;
      if (rootRef.current?.contains(t)) return;
      if (menuRef.current?.contains(t)) return;
      setOpen(false);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };
    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  const menu =
    open && pos && typeof document !== "undefined"
      ? createPortal(
          <ul
            ref={menuRef}
            id={listId}
            role="listbox"
            aria-label={ariaLabel}
            style={{
              position: "fixed",
              top: pos.top,
              left: pos.left,
              width: pos.width,
              maxHeight: 240,
            }}
            className={cn(
              "z-[100] overflow-auto rounded-lg border border-neutral-200 bg-white p-1 shadow-lg",
              menuClassName
            )}
          >
            {options.map((option) => {
              const isActive = option.value === value;
              return (
                <li key={option.value} role="presentation">
                  <button
                    type="button"
                    role="option"
                    aria-selected={isActive}
                    disabled={option.disabled}
                    onClick={() => {
                      if (option.disabled) return;
                      onChange(option.value);
                      setOpen(false);
                    }}
                    className={cn(
                      "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm text-neutral-800 transition-colors",
                      size === "sm" && "py-1 text-xs",
                      option.disabled
                        ? "cursor-not-allowed opacity-40"
                        : "hover:bg-neutral-100",
                      isActive && "bg-neutral-100 font-medium text-neutral-950"
                    )}
                  >
                    <Check
                      className={cn(
                        "size-3.5 shrink-0 text-neutral-900",
                        !isActive && "opacity-0"
                      )}
                      aria-hidden
                    />
                    <span className="min-w-0 flex-1 truncate">{option.label}</span>
                  </button>
                </li>
              );
            })}
          </ul>,
          document.body
        )
      : null;

  return (
    <div ref={rootRef} className={cn("relative w-full", className)}>
      <button
        ref={triggerRef}
        type="button"
        disabled={disabled}
        title={title}
        aria-label={ariaLabel}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={listId}
        onClick={() => setOpen((v) => !v)}
        className={cn(
          "flex w-full items-center gap-2 rounded-md border border-neutral-200 bg-white text-left text-neutral-900 transition-colors hover:bg-neutral-50 disabled:cursor-not-allowed disabled:opacity-40",
          size === "sm" ? "h-8 px-2 text-xs" : "h-9 px-2.5 text-sm",
          open && "border-neutral-300 bg-neutral-50",
          triggerClassName
        )}
      >
        <span
          className={cn(
            "min-w-0 flex-1 truncate",
            !selected && "text-neutral-400"
          )}
        >
          {selected?.label ?? placeholder}
        </span>
        <ChevronDown
          className={cn(
            "shrink-0 text-neutral-400",
            size === "sm" ? "size-3.5" : "size-4"
          )}
        />
      </button>
      {menu}
    </div>
  );
}
