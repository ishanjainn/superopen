"use client";

import {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
} from "react";
import { Search, X } from "lucide-react";
import {
  displayUser,
  emptySessionQuery,
  incompleteSessionToken,
  parseSessionQuery,
  serializeSessionQuery,
  type SessionQuery,
  type SessionQueryFacets,
} from "@/lib/so/session-query";
import { cn } from "@/lib/utils";

const KNOWN_AGENTS = [
  { value: "cursor", label: "Cursor" },
  { value: "claude", label: "Claude Code" },
  { value: "codex", label: "Codex" },
  { value: "opencode", label: "OpenCode" },
  { value: "pi", label: "Pi" },
];

const KNOWN_TOOLS = [
  "Bash",
  "Read",
  "Write",
  "Edit",
  "Grep",
  "Glob",
  "Shell",
  "WebSearch",
  "Task",
  "TodoWrite",
];

type Props = {
  value: string;
  onChange: (value: string) => void;
  facets?: SessionQueryFacets;
  className?: string;
};

/** Split committed filters (chips) from the editable tail of the query. */
function splitQuery(value: string): { chips: SessionQuery; draft: string } {
  const incomplete = incompleteSessionToken(value);
  if (incomplete) {
    const before = value.slice(0, incomplete.start);
    return {
      chips: parseSessionQuery(before),
      draft: value.slice(incomplete.start),
    };
  }
  const parsed = parseSessionQuery(value);
  return {
    chips: { ...parsed, text: "" },
    draft: parsed.text,
  };
}

function joinQuery(chips: SessionQuery, draft: string): string {
  const d = draft.trimStart();
  // Draft may itself contain new operators (from:@… / agent:… / file:… / tool:…).
  if (/^(from|user|agent|vendor|model|file|tool):/i.test(d)) {
    const head = serializeSessionQuery({ ...chips, text: "" });
    return `${head}${head && d ? " " : ""}${draft}`.replace(/\s+/g, " ").trimStart();
  }
  return serializeSessionQuery({ ...chips, text: draft });
}

export default function SessionSearchBar({
  value,
  onChange,
  facets,
  className,
}: Props) {
  const [focused, setFocused] = useState(false);
  const [highlight, setHighlight] = useState(0);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const rootRef = useRef<HTMLDivElement | null>(null);
  const listId = useId();

  const { chips, draft } = useMemo(() => splitQuery(value), [value]);
  const incomplete = useMemo(() => incompleteSessionToken(value), [value]);

  const suggestions = useMemo(() => {
    const out: { value: string; label: string; detail?: string }[] = [];
    if (!incomplete) return out;
    const prefix = incomplete.prefix;
    if (incomplete.key === "from") {
      for (const u of facets?.users || []) {
        if (
          prefix &&
          !u.toLowerCase().includes(prefix) &&
          !displayUser(u).toLowerCase().includes(prefix)
        ) {
          continue;
        }
        out.push({
          value: u,
          label: displayUser(u),
          detail: u.includes("@") ? u : undefined,
        });
        if (out.length >= 8) break;
      }
      return out;
    }
    if (incomplete.key === "agent") {
      const merged = new Map<string, { value: string; label: string }>();
      for (const a of KNOWN_AGENTS) merged.set(a.value, a);
      for (const a of facets?.agents || []) {
        const key = a.toLowerCase();
        if (![...merged.keys()].some((k) => key.includes(k) || k.includes(key))) {
          merged.set(a, { value: a, label: a });
        }
      }
      for (const a of merged.values()) {
        if (
          prefix &&
          !a.value.toLowerCase().includes(prefix) &&
          !a.label.toLowerCase().includes(prefix)
        ) {
          continue;
        }
        out.push(a);
      }
      return out;
    }
    if (incomplete.key === "file") {
      for (const f of facets?.files || []) {
        if (prefix && !f.toLowerCase().includes(prefix)) continue;
        out.push({ value: f, label: f });
        if (out.length >= 8) break;
      }
      return out;
    }
    if (incomplete.key === "tool") {
      for (const t of KNOWN_TOOLS) {
        if (prefix && !t.toLowerCase().includes(prefix)) continue;
        out.push({ value: t, label: t });
      }
      return out;
    }
    return out;
  }, [incomplete, facets]);

  const openSuggest = focused && incomplete !== null && suggestions.length > 0;
  const openHints = focused && !value.trim();

  useEffect(() => {
    setHighlight(0);
  }, [incomplete?.key, incomplete?.prefix, suggestions.length]);

  useEffect(() => {
    if (!openSuggest && !openHints) return;
    const onPointerDown = (event: PointerEvent) => {
      if (rootRef.current?.contains(event.target as Node)) return;
      setFocused(false);
    };
    document.addEventListener("pointerdown", onPointerDown);
    return () => document.removeEventListener("pointerdown", onPointerDown);
  }, [openSuggest, openHints]);

  function clearFilter(kind: "user" | "agent" | "model" | "file" | "tool") {
    const next = { ...chips, text: draft };
    if (kind === "user") next.user = "";
    else if (kind === "agent") next.agent = "";
    else if (kind === "model") next.model = "";
    else if (kind === "file") next.file = "";
    else next.tool = "";
    const dropDraft =
      (kind === "user" && /^from:/i.test(draft)) ||
      (kind === "agent" && /^(agent|vendor):/i.test(draft)) ||
      (kind === "model" && /^model:/i.test(draft)) ||
      (kind === "file" && /^file:/i.test(draft)) ||
      (kind === "tool" && /^tool:/i.test(draft));
    onChange(serializeSessionQuery({ ...next, text: dropDraft ? "" : next.text }));
    inputRef.current?.focus();
  }

  function applySuggestion(s: { value: string }) {
    if (!incomplete) return;
    const token =
      incomplete.key === "from"
        ? `from:@${s.value}`
        : incomplete.key === "agent"
          ? `agent:${s.value}`
          : incomplete.key === "file"
            ? `file:${s.value}`
            : incomplete.key === "tool"
              ? `tool:${s.value}`
              : `model:${s.value}`;
    const before = value.slice(0, incomplete.start).trimEnd();
    onChange(`${before}${before ? " " : ""}${token} `);
    setFocused(false);
    requestAnimationFrame(() => inputRef.current?.focus());
  }

  function insertToken(prefix: string) {
    onChange(joinQuery(chips, prefix));
    setFocused(true);
    requestAnimationFrame(() => inputRef.current?.focus());
  }

  function onKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if (!openSuggest) return;
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setHighlight((h) => (h + 1) % suggestions.length);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setHighlight((h) => (h - 1 + suggestions.length) % suggestions.length);
    } else if (e.key === "Enter" && suggestions[highlight]) {
      e.preventDefault();
      applySuggestion(suggestions[highlight]);
    } else if (e.key === "Escape") {
      setFocused(false);
    }
  }

  return (
    <div ref={rootRef} className={cn("relative min-w-0 flex-1", className)}>
      <div
        className={cn(
          "flex min-h-8 w-full flex-wrap items-center gap-1.5 rounded-md border border-neutral-200 bg-neutral-50 px-2 py-1",
          focused && "border-neutral-400 bg-white"
        )}
        onClick={() => inputRef.current?.focus()}
      >
        <Search className="size-3.5 shrink-0 text-neutral-400" />
        {chips.user && (
          <Chip
            label={`from:@${displayUser(chips.user)}`}
            title={chips.user}
            onRemove={() => clearFilter("user")}
          />
        )}
        {chips.agent && (
          <Chip
            label={`agent:${chips.agent}`}
            onRemove={() => clearFilter("agent")}
          />
        )}
        {chips.model && (
          <Chip
            label={`model:${chips.model}`}
            onRemove={() => clearFilter("model")}
          />
        )}
        {chips.file && (
          <Chip
            label={`file:${chips.file}`}
            onRemove={() => clearFilter("file")}
          />
        )}
        {chips.tool && (
          <Chip
            label={`tool:${chips.tool}`}
            onRemove={() => clearFilter("tool")}
          />
        )}
        <input
          ref={inputRef}
          value={draft}
          onChange={(e) => onChange(joinQuery(chips, e.target.value))}
          onFocus={() => setFocused(true)}
          onKeyDown={onKeyDown}
          placeholder={
            chips.user || chips.agent || chips.model || chips.file || chips.tool
              ? "Add text or another filter…"
              : "Search sessions - try file:trace.ts tool:Bash"
          }
          className="min-w-[10rem] flex-1 bg-transparent py-0.5 text-sm text-neutral-900 outline-none placeholder:text-neutral-400"
          aria-autocomplete="list"
          aria-controls={openSuggest ? listId : undefined}
          aria-expanded={openSuggest}
          role="combobox"
        />
        {value && (
          <button
            type="button"
            aria-label="Clear search"
            className="rounded p-0.5 text-neutral-400 hover:bg-neutral-200 hover:text-neutral-700"
            onClick={(e) => {
              e.stopPropagation();
              onChange("");
              inputRef.current?.focus();
            }}
          >
            <X className="size-3.5" />
          </button>
        )}
      </div>

      {openSuggest && (
        <ul
          id={listId}
          role="listbox"
          className="absolute left-0 right-0 top-[calc(100%+4px)] z-30 max-h-56 overflow-auto rounded-md border border-neutral-200 bg-white py-1 shadow-lg"
        >
          <li className="px-2.5 py-1 text-[10px] font-medium uppercase tracking-wide text-neutral-400">
            {incomplete?.key === "from" ? "People" : "Agents"}
          </li>
          {suggestions.map((s, i) => (
            <li key={`${s.value}-${i}`} role="option" aria-selected={i === highlight}>
              <button
                type="button"
                className={cn(
                  "flex w-full items-center justify-between gap-2 px-2.5 py-1.5 text-left text-sm",
                  i === highlight
                    ? "bg-neutral-100 text-neutral-900"
                    : "text-neutral-700 hover:bg-neutral-50"
                )}
                onMouseEnter={() => setHighlight(i)}
                onClick={() => applySuggestion(s)}
              >
                <span className="truncate">{s.label}</span>
                {s.detail && s.detail !== s.label ? (
                  <span className="truncate text-xs text-neutral-400">{s.detail}</span>
                ) : null}
              </button>
            </li>
          ))}
        </ul>
      )}

      {openHints && (
        <div className="absolute left-0 right-0 top-[calc(100%+4px)] z-20 rounded-md border border-neutral-200 bg-white p-2 shadow-lg">
          <p className="px-1 pb-1.5 text-[11px] text-neutral-500">
            Type a filter, then pick a value - or search free text for files and tools:
          </p>
          <div className="flex flex-wrap gap-1.5">
            <HintButton label="from:@" onClick={() => insertToken("from:@")} />
            <HintButton label="agent:" onClick={() => insertToken("agent:")} />
            <HintButton label="file:" onClick={() => insertToken("file:")} />
            <HintButton label="tool:" onClick={() => insertToken("tool:")} />
            {(facets?.users || []).slice(0, 3).map((u) => (
              <HintButton
                key={u}
                label={`from:@${displayUser(u)}`}
                onClick={() =>
                  onChange(
                    serializeSessionQuery({
                      ...emptySessionQuery(),
                      ...chips,
                      user: u,
                      text: chips.text || "",
                    })
                  )
                }
              />
            ))}
            {KNOWN_AGENTS.slice(0, 3).map((a) => (
              <HintButton
                key={a.value}
                label={`agent:${a.value}`}
                onClick={() =>
                  onChange(
                    serializeSessionQuery({
                      ...emptySessionQuery(),
                      ...chips,
                      agent: a.value,
                      text: "",
                    })
                  )
                }
              />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function Chip({
  label,
  title,
  onRemove,
}: {
  label: string;
  title?: string;
  onRemove: () => void;
}) {
  return (
    <span
      title={title}
      className="inline-flex max-w-[12rem] items-center gap-0.5 rounded bg-sky-50 px-1.5 py-0.5 text-[11px] text-sky-900 ring-1 ring-sky-100"
    >
      <span className="truncate">{label}</span>
      <button
        type="button"
        aria-label={`Remove ${label}`}
        className="rounded p-0.5 text-sky-700/70 hover:bg-sky-100 hover:text-sky-950"
        onClick={(e) => {
          e.stopPropagation();
          onRemove();
        }}
      >
        <X className="size-3" />
      </button>
    </span>
  );
}

function HintButton({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="rounded border border-neutral-200 bg-neutral-50 px-1.5 py-0.5 font-mono text-[11px] text-neutral-700 hover:border-neutral-300 hover:bg-white"
    >
      {label}
    </button>
  );
}
