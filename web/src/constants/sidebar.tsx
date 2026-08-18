import {
  LayoutDashboard,
  Network,
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
    icon: <Network className={ICON_CLASSES} />,
    text: "Graph",
    link: "/graph",
    type: "action",
  },
];
