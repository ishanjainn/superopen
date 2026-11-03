import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

type PanelPresentation = "sheet" | "pop";
type PanelSection = "scene" | "session";
type PanelBadge = "running" | "done" | "stale" | "failed";

/** Panel metadata used by the map HUD. */
export interface PanelDescriptor {
  id: string;
  icon: LucideIcon;
  hint: string;
  label?: string;
  section: PanelSection;
  presentation: PanelPresentation;
  badge?: PanelBadge | null;
  render: () => ReactNode;
}
