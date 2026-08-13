"use client";

import { AlertTriangle, ChevronLeft, ChevronRight, X } from "lucide-react";
import { touchWord, type SessionFile, type Touch, type TraceEvent } from "../types";

interface InspectorProps {
  /** absent when nothing is selected yet - renders the teaching empty state */
  file?: SessionFile;
  touch?: Touch;
  history: TraceEvent[];
  onClose: () => void;
  onJumpTo: (seq: number) => void;
  locked?: boolean;
  /** Spatial neighbors around the selection (closest first, including current). */
  neighbors?: SessionFile[];
  onSelectNeighbor?: (path: string) => void;
}

/** Selected file identity, touch state, and visit history. Dock owns placement. */
export function Inspector({
  file,
  touch,
  history,
  onClose,
  onJumpTo,
  locked = false,
  neighbors = [],
  onSelectNeighbor,
}: InspectorProps) {
  if (!file) {
    return (
      <div className="dock-body" aria-label="File inspector">
        <div className="inspector-head">
          <div className="inspector-path">Inspect</div>
          <button type="button" className="icon-btn" onClick={onClose} title="Close" aria-label="Close inspector">
            <X size={15} />
          </button>
        </div>
        <p className="dock-note">
          Click a file on the map to inspect it - touch state, size, and every visit the agent paid it.
        </p>
      </div>
    );
  }
  const slash = file.path.lastIndexOf("/");
  const dir = slash >= 0 ? file.path.slice(0, slash + 1) : "";
  const name = slash >= 0 ? file.path.slice(slash + 1) : file.path;

  const idx = neighbors.findIndex((n) => n.path === file.path);
  const canStep = neighbors.length > 1 && Boolean(onSelectNeighbor);
  const prev = canStep && idx >= 0 ? neighbors[(idx - 1 + neighbors.length) % neighbors.length] : undefined;
  const next = canStep && idx >= 0 ? neighbors[(idx + 1) % neighbors.length] : undefined;

  return (
    <div className="dock-body" aria-label={`File ${file.path}`}>
      <div className="inspector-head">
        <div>
          <div className="inspector-path">
            <span className="dir">{dir}</span>
            {name}
          </div>
          {file.ghost ? <span className="ghost-badge">ghost - not in this tree</span> : null}
        </div>
        <button type="button" className="icon-btn" onClick={onClose} title="Close" aria-label="Close inspector">
          <X size={15} />
        </button>
      </div>

      {canStep ? (
        <div className="inspector-nav" role="group" aria-label="Nearby files">
          <button
            type="button"
            className="inspector-nav-btn"
            disabled={locked || !prev}
            onClick={() => prev && onSelectNeighbor?.(prev.path)}
            title={prev ? `Previous nearby: ${prev.path}` : undefined}
            aria-label="Previous nearby file"
          >
            <ChevronLeft size={15} />
            Prev
          </button>
          <span className="inspector-nav-count">
            {idx >= 0 ? idx + 1 : "-"} / {neighbors.length} nearby
          </span>
          <button
            type="button"
            className="inspector-nav-btn"
            disabled={locked || !next}
            onClick={() => next && onSelectNeighbor?.(next.path)}
            title={next ? `Next nearby: ${next.path}` : undefined}
            aria-label="Next nearby file"
          >
            Next
            <ChevronRight size={15} />
          </button>
        </div>
      ) : null}

      <dl className="inspector-facts">
        <div>
          <dt>Touch</dt>
          <dd className={touch ? `touch-${touch}` : undefined}>{touchWord(touch)}</dd>
        </div>
        <div>
          <dt>Lang</dt>
          <dd>{file.lang || "text"}</dd>
        </div>
        <div>
          <dt>Lines</dt>
          <dd>{file.lines.toLocaleString()}</dd>
        </div>
        <div>
          <dt>Bytes</dt>
          <dd>{file.bytes.toLocaleString()}</dd>
        </div>
      </dl>
      <section className="inspector-history">
        <p className="eyebrow">Visits · {history.length}</p>
        <div className="history-list">
          {history
            .slice(-14)
            .reverse()
            .map((event) => (
              <button
                key={event.seq}
                type="button"
                className="history-row"
                onClick={() => onJumpTo(event.seq)}
                disabled={locked}
                title={`Jump to step ${event.seq + 1} - ${event.summary}`}
              >
                <span className={`action-dot ${event.action}`} />
                <strong>#{event.seq + 1}</strong>
                <span>{event.tool}</span>
                <span className="history-time">{event.ts ? clock(event.ts) : ""}</span>
                {event.isError ? <AlertTriangle className="history-err" size={13} /> : <span />}
              </button>
            ))}
          {history.length === 0 ? (
            <p className="muted">Not visited yet at this point of the walk. Scrub the timeline forward.</p>
          ) : null}
        </div>
      </section>
    </div>
  );
}

function clock(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return [d.getHours(), d.getMinutes(), d.getSeconds()].map((n) => String(n).padStart(2, "0")).join(":");
}
