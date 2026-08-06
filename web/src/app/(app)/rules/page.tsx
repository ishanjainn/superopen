"use client";

import { HarnessFilesPage } from "@/components/harness-files-page";

export default function RulesPage() {
  return (
    <HarnessFilesPage
      title="Rules"
      dir="rules"
      emptyHint="No rules yet. Create one with New, or add markdown under .so/rules/."
    />
  );
}
