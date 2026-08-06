"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

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
  /** "" = this repo, "all" = all projects, else project id */
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
  try {
    const res = await fetch(`/api/ui/prefs?key=${encodeURIComponent(PREF_KEY)}`);
    if (!res.ok) return "";
    const data = (await res.json()) as { value?: string | null };
    return typeof data.value === "string" ? data.value : "";
  } catch {
    return "";
  }
}

async function savePref(value: string) {
  try {
    await fetch("/api/ui/prefs", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ key: PREF_KEY, value }),
    });
  } catch {
    /* ignore */
  }
}

export function ProjectProvider({ children }: { children: ReactNode }) {
  const [projects, setProjects] = useState<Project[]>([]);
  const [projectId, setProjectIdState] = useState("");
  const [currentSlug, setCurrentSlug] = useState("");
  const [ready, setReady] = useState(false);
  const projectIdRef = useRef(projectId);
  projectIdRef.current = projectId;

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
      setProjectIdState(pref);
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
  }, []);

  const setProjectId = useCallback((id: string) => {
    setProjectIdState(id);
    void savePref(id);
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
    <ProjectContext.Provider value={value}>{children}</ProjectContext.Provider>
  );
}

export function useProject() {
  const ctx = useContext(ProjectContext);
  if (!ctx) {
    throw new Error("useProject must be used within ProjectProvider");
  }
  return ctx;
}
