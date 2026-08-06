import type { ReactNode } from "react";

export type SidebarActionItem = {
  type: "action";
  text: string;
  link?: string;
  icon?: ReactNode;
  target?: string;
  onClick?: () => void;
};

export type SidebarSection = {
  type: "section";
  title: string;
  icon?: ReactNode;
  collapsible?: boolean;
  children?: SidebarActionItem[];
  groups?: { title: string; children: SidebarActionItem[] }[];
};

export type SidebarItemProps = SidebarActionItem | SidebarSection;
