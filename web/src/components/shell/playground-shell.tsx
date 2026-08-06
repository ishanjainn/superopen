"use client";

import type { ReactNode } from "react";
import Sidebar from "@/components/shell/sidebar";
import SidebarBrand from "@/components/shell/sidebar-brand";
import { HeaderContextRow } from "@/components/shell/header";
import { BreadcrumbProvider } from "@/components/shell/breadcrumb-context";
import { ProjectProvider } from "@/components/shell/project-context";
import {
  SidebarLayoutProvider,
  useSidebarLayout,
} from "@/components/shell/sidebar-layout-context";
import { cn } from "@/lib/utils";

function PlaygroundShellFrame({ children }: { children: ReactNode }) {
  const { sidebarWidthClass } = useSidebarLayout();

  return (
    <div className="flex h-screen w-full flex-col overflow-hidden border border-neutral-200 bg-white">
      <div className="flex shrink-0 border-b border-neutral-200">
        <div
          className={cn(
            "relative shrink-0 border-r border-neutral-200",
            sidebarWidthClass
          )}
        >
          <SidebarBrand />
        </div>
        <HeaderContextRow />
      </div>
      <div className="flex min-h-0 flex-1">
        <div
          className={cn(
            "relative z-30 flex shrink-0 flex-col border-r border-neutral-200",
            sidebarWidthClass
          )}
        >
          <Sidebar />
        </div>
        <div className="flex min-w-0 flex-1 flex-col bg-white">
          <main className="flex min-h-0 flex-1 flex-col overflow-hidden bg-white">
            {children}
          </main>
        </div>
      </div>
    </div>
  );
}

export default function PlaygroundShell({ children }: { children: ReactNode }) {
  return (
    <SidebarLayoutProvider>
      <ProjectProvider>
        <BreadcrumbProvider>
          <PlaygroundShellFrame>{children}</PlaygroundShellFrame>
        </BreadcrumbProvider>
      </ProjectProvider>
    </SidebarLayoutProvider>
  );
}
