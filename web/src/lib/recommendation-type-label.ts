/** Client-safe display helpers for recommendation types (no Node imports). */

export function recommendationTypeLabel(type?: string): string {
  const t = String(type || "").toLowerCase();
  if (t === "docs" || t === "doc" || t === "knowledge") return "Knowledge";
  if (t === "guardrail") return "Guardrail";
  if (t === "skill") return "Skill";
  if (t === "graph") return "Graph";
  if (!type) return "Rec";
  return String(type);
}
