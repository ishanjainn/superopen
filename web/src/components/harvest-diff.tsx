"use client";

import { useMemo, useState } from "react";
import { cn } from "@/lib/utils";
import {
  parseUnifiedDiff,
  type DiffHunk,
  type DiffLine,
} from "@/lib/harvest-diff";

export default function HarvestDiff({ diff }: { diff: string }) {
  const [mode, setMode] = useState<"unified" | "split">("unified");
  const hunks = useMemo(() => parseUnifiedDiff(diff || ""), [diff]);
  if (!diff.trim()) {
    return (
      <p className="px-3 py-2 font-mono text-xs text-neutral-500">No diff.</p>
    );
  }
  return (
    <div className="overflow-hidden rounded-md border border-neutral-200">
      <div className="flex justify-end gap-1 border-b border-neutral-200 bg-neutral-50 px-2 py-1">
        <button
          type="button"
          onClick={() => setMode("unified")}
          className={cn(
            "rounded px-2 py-0.5 text-[11px]",
            mode === "unified"
              ? "bg-white text-neutral-900 shadow-sm"
              : "text-neutral-500",
          )}
        >
          Unified
        </button>
        <button
          type="button"
          onClick={() => setMode("split")}
          className={cn(
            "rounded px-2 py-0.5 text-[11px]",
            mode === "split"
              ? "bg-white text-neutral-900 shadow-sm"
              : "text-neutral-500",
          )}
        >
          Split
        </button>
      </div>
      {hunks.map((hunk) =>
        mode === "unified" ? (
          <UnifiedHunk key={hunk.header} hunk={hunk} />
        ) : (
          <SplitHunk key={hunk.header} hunk={hunk} />
        ),
      )}
    </div>
  );
}

function UnifiedHunk({ hunk }: { hunk: DiffHunk }) {
  return (
    <div className="font-mono text-[12px] leading-5">
      <div className="bg-neutral-100 px-2 py-1 text-[11px] text-neutral-500">
        {hunk.header}
      </div>
      {hunk.lines.map((line, i) => (
        <div
          key={i}
          className={cn(
            "grid grid-cols-[3rem_3rem_1rem_1fr] px-2",
            lineClass(line.kind),
          )}
        >
          <span className="text-right text-neutral-400">{line.oldNo ?? ""}</span>
          <span className="text-right text-neutral-400">{line.newNo ?? ""}</span>
          <span>{gutter(line.kind)}</span>
          <span className="whitespace-pre-wrap break-all">
            {renderText(line)}
          </span>
        </div>
      ))}
    </div>
  );
}

function SplitHunk({ hunk }: { hunk: DiffHunk }) {
  const rows = splitRows(hunk.lines);
  return (
    <div className="font-mono text-[12px] leading-5">
      <div className="bg-neutral-100 px-2 py-1 text-[11px] text-neutral-500">
        {hunk.header}
      </div>
      {rows.map((row, i) => (
        <div key={i} className="grid grid-cols-2 border-t border-neutral-100">
          <SplitCell side="old" line={row.left} />
          <SplitCell side="new" line={row.right} />
        </div>
      ))}
    </div>
  );
}

function SplitCell({
  side,
  line,
}: {
  side: "old" | "new";
  line?: DiffLine;
}) {
  const no = side === "old" ? line?.oldNo : line?.newNo;
  return (
    <div
      className={cn(
        "grid grid-cols-[3rem_1rem_1fr] border-neutral-100 px-2",
        side === "new" && "border-l",
        line ? lineClass(line.kind) : "bg-neutral-50/80",
      )}
    >
      <span className="text-right text-neutral-400">{no ?? ""}</span>
      <span>{line ? gutter(line.kind) : ""}</span>
      <span className="whitespace-pre-wrap break-all">
        {line ? renderText(line) : ""}
      </span>
    </div>
  );
}

function splitRows(lines: DiffLine[]): { left?: DiffLine; right?: DiffLine }[] {
  const rows: { left?: DiffLine; right?: DiffLine }[] = [];
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (line.kind === "ctx") {
      rows.push({ left: line, right: line });
    } else if (line.kind === "chg-del" && lines[i + 1]?.kind === "chg-add") {
      rows.push({ left: line, right: lines[i + 1] });
      i += 1;
    } else if (line.kind === "del") {
      rows.push({ left: line });
    } else if (line.kind === "add") {
      rows.push({ right: line });
    }
  }
  return rows;
}

function gutter(kind: DiffLine["kind"]): string {
  if (kind === "add" || kind === "chg-add") return "+";
  if (kind === "del" || kind === "chg-del") return "-";
  return " ";
}

function lineClass(kind: DiffLine["kind"]): string {
  if (kind === "add" || kind === "chg-add") return "bg-emerald-50 text-emerald-950";
  if (kind === "del" || kind === "chg-del") return "bg-red-50 text-red-950";
  return "bg-white text-neutral-800";
}

function renderText(line: DiffLine) {
  if (!line.spans?.length) return line.text;
  return line.spans.map((span, i) => (
    <span
      key={i}
      className={
        span.changed
          ? line.kind === "chg-add"
            ? "rounded-sm bg-emerald-200/80"
            : "rounded-sm bg-red-200/80"
          : undefined
      }
    >
      {span.text}
    </span>
  ));
}
