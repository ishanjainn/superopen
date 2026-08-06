import {
  BookOpen,
  Brain,
  ClipboardCheck,
  FileText,
  LayoutDashboard,
  Lightbulb,
  Network,
  SettingsIcon,
  Shield,
  Sparkles,
} from "lucide-react";
import type { SidebarItemProps } from "@/types/sidebar";

const ICON_CLASSES = "flex-shrink-0 size-4";

export const SIDEBAR_ITEMS: SidebarItemProps[] = [
  {
    icon: <LayoutDashboard className={ICON_CLASSES} />,
    text: "Sessions",
    link: "/sessions",
    type: "action",
  },
  {
    icon: <Brain className={ICON_CLASSES} />,
    text: "Memory",
    link: "/memory",
    type: "action",
  },
  {
    icon: <Network className={ICON_CLASSES} />,
    text: "Graph",
    link: "/graph",
    type: "action",
  },
  {
    icon: <BookOpen className={ICON_CLASSES} />,
    text: "Knowledge",
    link: "/knowledge",
    type: "action",
  },
  {
    icon: <FileText className={ICON_CLASSES} />,
    text: "Rules",
    link: "/rules",
    type: "action",
  },
  {
    icon: <Sparkles className={ICON_CLASSES} />,
    text: "Skills",
    link: "/skills",
    type: "action",
  },
  {
    icon: <Shield className={ICON_CLASSES} />,
    text: "Guardrails",
    link: "/guardrails",
    type: "action",
  },
  {
    icon: <ClipboardCheck className={ICON_CLASSES} />,
    text: "Evaluations",
    link: "/evaluations",
    type: "action",
  },
  {
    icon: <Lightbulb className={ICON_CLASSES} />,
    text: "Recommendations",
    link: "/recs",
    type: "action",
  },
  {
    icon: <SettingsIcon className={ICON_CLASSES} />,
    text: "Settings",
    link: "/settings",
    type: "action",
  },
];
