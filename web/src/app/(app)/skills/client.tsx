"use client";

import { HarnessFilesPage } from "@/components/harness-files-page";

export default function SkillsPage() {
  return (
    <HarnessFilesPage
      title="Skills"
      dir="skills"
      emptyHint="No skills yet across vendor trees. New skills land in the preferred Claude/Cursor/… skills dir as <name>/SKILL.md."
    />
  );
}
