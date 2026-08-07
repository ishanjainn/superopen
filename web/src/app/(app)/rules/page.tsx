"use client";

import { HarnessFilesPage } from "@/components/harness-files-page";

export default function RulesPage() {
  return (
    <HarnessFilesPage
      title="Rules"
      dir="rules"
      emptyHint="No rules yet across Cursor/Claude/agents/… trees. New files land in the preferred vendor rules dir."
    />
  );
}
