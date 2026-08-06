"use client";

import { HarnessFilesPage } from "@/components/harness-files-page";

export default function KnowledgePage() {
  return (
    <HarnessFilesPage
      title="Knowledge"
      dir="knowledge"
      emptyHint="No knowledge yet. Create one with New, or run so init / so sync."
    />
  );
}
