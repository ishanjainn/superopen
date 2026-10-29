"use client";

import { useState, type ReactNode } from "react";

export type OnboardingStep = {
  /** What the command does, in a few words. */
  label: string;
  /** Omit for a step that is something to do, not something to run. */
  command?: string;
};

/**
 * First-run screen for a feature that has no data yet. The UI only runs when
 * the CLI is installed, and most features run on their own once a repository has
 * been set up, so the commands here are the manual way, not required setup.
 */
export default function Onboarding({
  title,
  description,
  steps,
  stepsLabel,
  note,
  actions,
}: {
  title: string;
  description: string;
  steps: OnboardingStep[];
  /** Heading above the steps, e.g. "Or run it yourself". */
  stepsLabel?: string;
  note?: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <div className="pointer-events-auto relative z-10 grid h-full min-h-0 place-items-center overflow-auto p-6">
      <div className="w-full max-w-lg rounded-lg border border-neutral-200 bg-white p-6">
        <h2 className="text-sm font-semibold text-neutral-900">{title}</h2>
        <p className="mt-2 text-xs leading-relaxed text-neutral-500">{description}</p>
        {stepsLabel ? (
          <h3 className="mt-5 text-[11px] font-medium uppercase tracking-wide text-neutral-400">{stepsLabel}</h3>
        ) : null}
        <ol className={stepsLabel ? "mt-2 space-y-3" : "mt-4 space-y-3"}>
          {steps.map((step, index) => (
            <li key={step.label} className="flex gap-3">
              <span className="mt-0.5 grid size-5 shrink-0 place-items-center rounded-full border border-neutral-300 text-[11px] text-neutral-500">
                {index + 1}
              </span>
              <div className="min-w-0 flex-1">
                <p className="text-xs text-neutral-700">{step.label}</p>
                {step.command ? <CommandLine command={step.command} /> : null}
              </div>
            </li>
          ))}
        </ol>
        {note ? <p className="mt-4 text-xs leading-relaxed text-neutral-500">{note}</p> : null}
        {actions ? <div className="mt-4 flex gap-2">{actions}</div> : null}
      </div>
    </div>
  );
}

function CommandLine({ command }: { command: string }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(command);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard access is optional; the command stays selectable.
    }
  };
  return (
    <div className="mt-1.5 flex items-center gap-2 rounded border border-neutral-200 bg-neutral-50 px-2.5 py-1.5">
      <code className="min-w-0 flex-1 truncate font-mono text-xs text-neutral-900">{command}</code>
      <button
        type="button"
        onClick={() => void copy()}
        className="shrink-0 rounded border border-neutral-300 px-2 py-0.5 text-[11px] text-neutral-600 hover:text-neutral-900"
      >
        {copied ? "Copied" : "Copy"}
      </button>
    </div>
  );
}
