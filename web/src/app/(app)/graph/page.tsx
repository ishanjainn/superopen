"use client";

import { useEffect, useMemo, useState } from "react";
import FeaturePageHeader from "@/components/shell/feature-page-header";
import { GraphifyFrame } from "@/components/graphify-frame";
import { useProject } from "@/components/shell/project-context";
import { useTheme } from "@/components/shell/theme-provider";

/** Graphify community map from `.so/graph/graph.html` (themed to match the UI). */
export default function GraphPage() {
  const { projectId } = useProject();
  const { resolved } = useTheme();
  const graphSrc = useMemo(() => {
    // bump v when theme/serve rules change so iframes refresh
    const params = new URLSearchParams({ theme: resolved, v: "7" });
    if (projectId && projectId !== "all") params.set("project", projectId);
    return `/api/graph/html?${params.toString()}`;
  }, [projectId, resolved]);

  const [htmlOk, setHtmlOk] = useState<boolean | null>(null);
  const [htmlError, setHtmlError] = useState("");
  const [cliTip, setCliTip] = useState("");

  useEffect(() => {
    let cancelled = false;
    setHtmlOk(null);
    setCliTip("");
    (async () => {
      try {
        const metaUrl = new URL(graphSrc, window.location.origin);
        metaUrl.searchParams.set("meta", "1");
        const metaRes = await fetch(metaUrl.toString());
        const meta = (await metaRes.json().catch(() => null)) as {
          ok?: boolean;
          reason?: string;
          tip?: string;
        } | null;
        if (cancelled) return;
        if (meta?.ok) {
          setHtmlOk(true);
          setHtmlError("");
          return;
        }
        setHtmlOk(false);
        setHtmlError(
          meta?.reason ||
            "Graphify community map unavailable - communities must come from Graphify"
        );
        setCliTip(
          meta?.tip ||
            "so graph rebuild   or   graphify cluster-only . && graphify export html --graph .so/graph/graph.json"
        );
      } catch (e) {
        if (!cancelled) {
          setHtmlOk(false);
          setHtmlError(
            e instanceof Error ? e.message : "Failed to load Graphify HTML"
          );
          setCliTip("so graph rebuild");
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [graphSrc]);

  return (
    <div className="flex h-full min-h-0 flex-col bg-white">
      <FeaturePageHeader
        title="Graph"
        actions={
          <span className="text-xs text-neutral-500">provider: Graphify</span>
        }
      />

      {htmlOk === null ? (
        <div className="grid min-h-0 flex-1 place-items-center text-sm text-neutral-500">
          Loading Graphify…
        </div>
      ) : htmlOk ? (
        <div className="relative min-h-0 flex-1 bg-white">
          <GraphifyFrame src={graphSrc} />
        </div>
      ) : (
        <div className="flex min-h-0 flex-1 flex-col items-center justify-center gap-4 px-6 text-center">
          <p className="max-w-lg text-sm text-neutral-700">
            Graphify communities aren’t ready yet. Superopen no longer invents the
            community list - Graphify must write{" "}
            <code className="rounded bg-neutral-100 px-1">LEGEND</code> into{" "}
            <code className="rounded bg-neutral-100 px-1">.so/graph/graph.html</code>.
          </p>
          {htmlError && (
            <p className="max-w-lg text-xs text-red-600">{htmlError}</p>
          )}
          <div className="max-w-xl rounded-lg border border-neutral-200 bg-neutral-50 px-4 py-3 text-left text-xs text-neutral-700">
            <p className="mb-2 font-medium text-neutral-900">
              Run one of these in the repo root (after retries in{" "}
              <code>so graph rebuild</code> already failed):
            </p>
            <ol className="list-decimal space-y-2 pl-4 font-mono text-[11px] leading-relaxed">
              <li>
                <code>so graph rebuild</code>
                <span className="mt-0.5 block font-sans text-neutral-500">
                  preferred - retries Graphify twice, then writes{" "}
                  <code>.so/graph/</code>
                </span>
              </li>
              <li>
                <code>graphify cluster-only .</code>
                <span className="mt-0.5 block font-sans text-neutral-500">
                  then{" "}
                  <code>
                    graphify export html --graph .so/graph/graph.json
                  </code>
                </span>
              </li>
            </ol>
            {cliTip && (
              <p className="mt-3 font-sans text-neutral-500">
                Hint: <code className="font-mono text-neutral-700">{cliTip}</code>
              </p>
            )}
          </div>
          <p className="max-w-md text-xs text-neutral-500">
            Install Graphify if needed:{" "}
            <code className="rounded bg-neutral-100 px-1">uv tool install graphifyy</code>
          </p>
        </div>
      )}
    </div>
  );
}
