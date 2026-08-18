"use client";

import { FormEvent, useState } from "react";
import FeaturePageHeader from "@/components/shell/feature-page-header";
import { useProject } from "@/components/shell/project-context";

export default function GraphPage() {
  const { projectId } = useProject();
  const [question, setQuestion] = useState("");
  const [answer, setAnswer] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function queryGraph(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const value = question.trim();
    if (!value) return;
    setLoading(true);
    setError("");
    try {
      const url = new URL("/api/graph/query", window.location.origin);
      url.searchParams.set("q", value);
      if (projectId) url.searchParams.set("project", projectId);
      const response = await fetch(url.toString());
      const body = (await response.json()) as { answer?: string; error?: string };
      if (!response.ok) throw new Error(body.error || "Native graph query failed");
      setAnswer(body.answer || "");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Native graph query failed");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col bg-white">
      <FeaturePageHeader
        title="Native graph"
        actions={<span className="text-xs text-neutral-500">native graph</span>}
      />
      <div className="mx-auto flex w-full max-w-4xl flex-1 flex-col gap-5 overflow-auto p-6">
        <form onSubmit={queryGraph} className="flex gap-2">
          <input
            value={question}
            onChange={(event) => setQuestion(event.target.value)}
            placeholder="How does this subsystem work?"
            className="min-w-0 flex-1 rounded-lg border border-neutral-300 px-3 py-2 text-sm outline-none focus:border-neutral-500"
          />
          <button
            type="submit"
            disabled={loading || !question.trim()}
            className="rounded-lg bg-neutral-900 px-4 py-2 text-sm text-white disabled:opacity-50"
          >
            {loading ? "Querying…" : "Query"}
          </button>
        </form>
        <p className="text-xs text-neutral-500">
          Build or refresh with <code>so graph build</code>. Coding agents can connect with{" "}
          <code>so graph mcp serve</code>.
        </p>
        {error && <p className="rounded-lg bg-red-50 p-3 text-sm text-red-700">{error}</p>}
        {answer && (
          <pre className="whitespace-pre-wrap rounded-lg border border-neutral-200 bg-neutral-50 p-4 text-xs leading-relaxed text-neutral-800">
            {answer}
          </pre>
        )}
      </div>
    </div>
  );
}
