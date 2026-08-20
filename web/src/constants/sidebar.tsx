import { Settings } from "lucide-react";
import {
  GraphIcon,
  MemoryIcon,
  SessionsIcon,
} from "@/components/shell/nav-icons";
import type { SidebarItemProps } from "@/types/sidebar";

const ICON_CLASSES = "flex-shrink-0 size-4";

export const SIDEBAR_ITEMS: SidebarItemProps[] = [
  {
    icon: <GraphIcon className={ICON_CLASSES} />,
    text: "Graph",
    link: "/graph",
    type: "action",
  },
  {
    icon: <SessionsIcon className={ICON_CLASSES} />,
    text: "Sessions",
    link: "/sessions",
    type: "action",
  },
  {
    icon: <MemoryIcon className={ICON_CLASSES} />,
    text: "Memory",
    link: "/memory",
    type: "action",
  },
  {
    icon: <Settings className={ICON_CLASSES} />,
    text: "Settings",
    link: "/settings",
    type: "action",
  },
];
