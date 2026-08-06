"use client";

import { Fragment, type ReactNode } from "react";
import { cn } from "@/lib/utils";

/** Lightweight markdown renderer - headings, lists, code, paragraphs. No deps. */
export function MarkdownView({ source, className }: { source: string; className?: string }) {
  const blocks = parseBlocks(source);
  return (
    <div className={cn("space-y-3 text-sm leading-relaxed text-neutral-800", className)}>
      {blocks.map((b, i) => (
        <Fragment key={i}>{renderBlock(b)}</Fragment>
      ))}
    </div>
  );
}

type Block =
  | { type: "h"; level: number; text: string }
  | { type: "p"; text: string }
  | { type: "ul"; items: string[] }
  | { type: "ol"; items: string[] }
  | { type: "code"; lang: string; text: string }
  | { type: "hr" };

function parseBlocks(src: string): Block[] {
  const lines = src.replace(/\r\n/g, "\n").split("\n");
  const out: Block[] = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (!line.trim()) {
      i++;
      continue;
    }
    if (/^---+$/.test(line.trim())) {
      out.push({ type: "hr" });
      i++;
      continue;
    }
    const fence = line.match(/^```(\w*)\s*$/);
    if (fence) {
      const lang = fence[1] || "";
      i++;
      const body: string[] = [];
      while (i < lines.length && !lines[i].startsWith("```")) {
        body.push(lines[i]);
        i++;
      }
      i++; // closing fence
      out.push({ type: "code", lang, text: body.join("\n") });
      continue;
    }
    const h = line.match(/^(#{1,4})\s+(.+)$/);
    if (h) {
      out.push({ type: "h", level: h[1].length, text: h[2].trim() });
      i++;
      continue;
    }
    if (/^[-*]\s+/.test(line)) {
      const items: string[] = [];
      while (i < lines.length && /^[-*]\s+/.test(lines[i])) {
        items.push(lines[i].replace(/^[-*]\s+/, ""));
        i++;
      }
      out.push({ type: "ul", items });
      continue;
    }
    if (/^\d+\.\s+/.test(line)) {
      const items: string[] = [];
      while (i < lines.length && /^\d+\.\s+/.test(lines[i])) {
        items.push(lines[i].replace(/^\d+\.\s+/, ""));
        i++;
      }
      out.push({ type: "ol", items });
      continue;
    }
    const para: string[] = [line];
    i++;
    while (
      i < lines.length &&
      lines[i].trim() &&
      !/^#{1,4}\s+/.test(lines[i]) &&
      !/^[-*]\s+/.test(lines[i]) &&
      !/^\d+\.\s+/.test(lines[i]) &&
      !lines[i].startsWith("```") &&
      !/^---+$/.test(lines[i].trim())
    ) {
      para.push(lines[i]);
      i++;
    }
    out.push({ type: "p", text: para.join(" ") });
  }
  return out;
}

function renderInline(text: string): ReactNode[] {
  const parts: ReactNode[] = [];
  const re = /(`[^`]+`|\*\*[^*]+\*\*)/g;
  let last = 0;
  let m: RegExpExecArray | null;
  let key = 0;
  while ((m = re.exec(text))) {
    if (m.index > last) parts.push(text.slice(last, m.index));
    const tok = m[0];
    if (tok.startsWith("`")) {
      parts.push(
        <code
          key={key++}
          className="rounded bg-neutral-100 px-1 py-0.5 font-mono text-[12px] text-neutral-800"
        >
          {tok.slice(1, -1)}
        </code>
      );
    } else {
      parts.push(
        <strong key={key++} className="font-semibold text-neutral-900">
          {tok.slice(2, -2)}
        </strong>
      );
    }
    last = m.index + tok.length;
  }
  if (last < text.length) parts.push(text.slice(last));
  return parts;
}

function renderBlock(b: Block): ReactNode {
  switch (b.type) {
    case "h": {
      const cls =
        b.level === 1
          ? "text-lg font-semibold text-neutral-900"
          : b.level === 2
            ? "text-base font-semibold text-neutral-900"
            : "text-sm font-semibold text-neutral-900";
      return <h2 className={cls}>{renderInline(b.text)}</h2>;
    }
    case "p":
      return <p>{renderInline(b.text)}</p>;
    case "ul":
      return (
        <ul className="list-disc space-y-1 pl-5">
          {b.items.map((it, i) => (
            <li key={i}>{renderInline(it)}</li>
          ))}
        </ul>
      );
    case "ol":
      return (
        <ol className="list-decimal space-y-1 pl-5">
          {b.items.map((it, i) => (
            <li key={i}>{renderInline(it)}</li>
          ))}
        </ol>
      );
    case "code":
      return (
        <pre className="overflow-auto rounded-lg border border-neutral-200 bg-neutral-50 p-3 font-mono text-[12px] text-neutral-700">
          {b.text}
        </pre>
      );
    case "hr":
      return <hr className="border-neutral-200" />;
  }
}
