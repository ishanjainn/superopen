"use client";

import { HarnessFilesPage } from "@/components/harness-files-page";

export default function SkillsPage() {
  return (
    <HarnessFilesPage
      title="Skills"
      dir="skills"
      emptyHint="No skills yet. Create one with New, or run so init / so sync."
    />
  );
}
