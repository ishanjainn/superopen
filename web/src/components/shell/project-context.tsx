"use client";

import {
  Fragment,
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { getUIPref, setUIPref } from "@/lib/ui-prefs";
import { useLatestRef } from "@/hooks/use-latest-ref";

export type Project = {
  id: string;
  name: string;
  repo_root: string;
  so_root?: string;
  remote_url?: string;
  slug?: string;
  missing?: boolean;
};

type ProjectContextValue = {
  projects: Project[];
  /** "" = the UI server's repository; otherwise one managed project id. */
  projectId: string;
  setProjectId: (id: string) => void;
  refreshProjects: () => Promise<void>;
  /** `owner/repo` for the process workspace (SUPEROPEN_ROOT) */
  currentSlug: string;
  ready: boolean;
};

const ProjectContext = createContext<ProjectContextValue | null>(null);

const PREF_KEY = "selected_project";
const HEADER = "x-so-project";

async function loadPref(): Promise<string> {
	return getUIPref(PREF_KEY) ?? "";
}

async function savePref(value: string) {
	setUIPref(PREF_KEY, value);
}

export function ProjectProvider({ children }: { children: ReactNode }) {
  const [projects, setProjects] = useState<Project[]>([]);
  const [projectId, setProjectIdState] = useState("");
  const [currentSlug, setCurrentSlug] = useState("");
  const [ready, setReady] = useState(false);
  const projectIdRef = useLatestRef(projectId);

  const refreshProjects = useCallback(async () => {
    try {
      const projRes = await fetch("/api/projects").then((r) =>
        r.ok ? r.json() : null
      );
      const list: Project[] = Array.isArray(projRes?.projects)
        ? projRes.projects
        : [];
      setProjects(list);
    } catch {
      /* ignore */
    }
  }, []);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const [pref, projRes, meta] = await Promise.all([
        loadPref(),
        fetch("/api/projects")
          .then((r) => (r.ok ? r.json() : null))
          .catch(() => null),
        fetch("/api/meta")
          .then((r) => (r.ok ? r.json() : null))
          .catch(() => null),
      ]);
      if (cancelled) return;
      const list: Project[] = Array.isArray(projRes?.projects)
        ? projRes.projects
        : [];
      setProjects(list);
      const slug =
        (typeof meta?.slug === "string" && meta.slug) ||
        list.find((p) => p.repo_root === meta?.root)?.slug ||
        list[0]?.slug ||
        meta?.repo ||
        "repo";
      setCurrentSlug(slug);
      // "all" was a legacy aggregate scope. Data pages are now always bound
      // to one repository, while Settings still lists the global registry.
      const preferredProject = list.find((project) => project.id === pref);
      const selected =
        pref !== "all" &&
        preferredProject &&
        preferredProject.repo_root !== meta?.root
          ? pref
          : "";
      setProjectIdState(selected);
      if (selected !== pref) void savePref(selected);
      setReady(true);
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (typeof window === "undefined") return;
    const original = window.fetch.bind(window);
    window.fetch = (input: RequestInfo | URL, init?: RequestInit) => {
      const url =
        typeof input === "string"
          ? input
          : input instanceof URL
            ? input.href
            : input.url;
      const isApi =
        url.startsWith("/api/") ||
        url.startsWith(`${window.location.origin}/api/`);
      if (!isApi) return original(input, init);

      const headers = new Headers(
        init?.headers ||
          (typeof input !== "string" && !(input instanceof URL)
            ? input.headers
            : undefined)
      );
      if (!headers.has(HEADER)) {
        headers.set(HEADER, projectIdRef.current);
      }
      return original(input, { ...init, headers });
    };
    return () => {
      window.fetch = original;
    };
  }, [projectIdRef]);

  const setProjectId = useCallback((id: string) => {
    const selected = id === "all" ? "" : id;
    setProjectIdState(selected);
    void savePref(selected);
  }, []);

  const value = useMemo(
    () => ({
      projects,
      projectId,
      setProjectId,
      refreshProjects,
      currentSlug,
      ready,
    }),
    [projects, projectId, setProjectId, refreshProjects, currentSlug, ready]
  );

  return (
    <ProjectContext.Provider value={value}>
      <Fragment key={projectId}>{children}</Fragment>
    </ProjectContext.Provider>
  );
}

export function useProject() {
  const ctx = useContext(ProjectContext);
  if (!ctx) {
    throw new Error("useProject must be used within ProjectProvider");
  }
  return ctx;
}
