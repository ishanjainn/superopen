/** Session map: in-memory repo file layout used by session replay (tree/terrain). */
import { createHash } from "crypto";
import { readdirSync, statSync } from "fs";
import { basename, dirname, extname, isAbsolute, join, relative, sep } from "path";
import { repoRoot } from "./root";

/** Directories the walk never descends into; trace seating skips them too. */
export const SKIPPED_DIRS = new Set([
  "node_modules",
  "dist",
  "build",
  "vendor",
  "target",
  "__pycache__",
  ".git",
  ".so",
  ".next",
  "Library",
]);

const MAX_FILES = 10_000;
const ROOT_SIZE = 120;
const CACHE_VERSION = 2;

export type SessionFile = {
  id: number;
  path: string;
  dir: string;
  lines: number;
  bytes: number;
  lang?: string;
  rect: Rect;
  ghost: boolean;
};

export type SessionDir = {
  path: string;
  depth: number;
  rect: Rect;
  fileCount: number;
  lines: number;
};

/**
 * A checkout or plain directory the session touched. The map is rooted at one
 * of them; the rest occupy a district named by `prefix`.
 */
export type MapRoot = {
  /** Top-level path segment on the map; "" for the repo the map is rooted at. */
  prefix: string;
  name: string;
  path: string;
  /** False for a plain directory, which is not a checkout and has no history. */
  git: boolean;
  files: number;
};

export type SessionMap = {
  version: number;
  repo: {
    root: string;
    commit?: string;
    dirty: boolean;
    generatedAt: string;
    truncated?: boolean;
  };
  roots: MapRoot[];
  files: SessionFile[];
  dirs: SessionDir[];
  layout: { algorithm: string; weight: string };
};

type Rect = { x: number; z: number; w: number; d: number };

type LayoutNode = {
  name: string;
  path: string;
  children: Map<string, LayoutNode>;
  files: number[];
  weight: number;
  fileCount: number;
  lines: number;
};

type LayoutItem = {
  name: string;
  kind: "dir" | "file";
  idx: number;
  node?: LayoutNode;
  weight: number;
  area: number;
};

function estimateLines(bytes: number): number {
  if (bytes <= 0) return 1;
  return Math.max(1, Math.round(bytes / 40));
}

function walkFiles(root: string): { path: string; bytes: number }[] {
  const out: { path: string; bytes: number }[] = [];
  const stack: string[] = [root];
  while (stack.length && out.length < MAX_FILES) {
    const dir = stack.pop()!;
    let entries;
    try {
      entries = readdirSync(dir, { withFileTypes: true });
    } catch {
      continue;
    }
    entries.sort((a, b) => a.name.localeCompare(b.name));
    for (const e of entries) {
      if (e.name.startsWith(".") && e.name !== ".github" && e.name !== ".agents") continue;
      if (SKIPPED_DIRS.has(e.name)) continue;
      const abs = join(dir, e.name);
      if (e.isDirectory()) {
        stack.push(abs);
        continue;
      }
      if (!e.isFile()) continue;
      let bytes = 0;
      try {
        bytes = statSync(abs).size;
      } catch {
        continue;
      }
      const rel = relative(root, abs).split(sep).join("/");
      out.push({ path: rel, bytes });
      if (out.length >= MAX_FILES) break;
    }
  }
  return out;
}

function fileWeight(lines: number, bytes: number): number {
  let units = lines;
  const byteUnits = bytes / 4096;
  if (byteUnits > units) units = byteUnits;
  if (units < 16) units = 16;
  return Math.sqrt(units);
}

function buildTree(files: SessionFile[]): LayoutNode {
  const root: LayoutNode = {
    name: "",
    path: "",
    children: new Map(),
    files: [],
    weight: 0,
    fileCount: 0,
    lines: 0,
  };
  for (let i = 0; i < files.length; i++) {
    const parts = files[i].path.split("/").filter(Boolean);
    let node = root;
    for (let p = 0; p < parts.length - 1; p++) {
      const name = parts[p];
      let child = node.children.get(name);
      if (!child) {
        child = {
          name,
          path: node.path ? `${node.path}/${name}` : name,
          children: new Map(),
          files: [],
          weight: 0,
          fileCount: 0,
          lines: 0,
        };
        node.children.set(name, child);
      }
      node = child;
    }
    node.files.push(i);
  }
  return root;
}

function sortedChildren(children: Map<string, LayoutNode>): LayoutNode[] {
  return [...children.values()].sort((a, b) => a.name.localeCompare(b.name));
}

function computeWeight(n: LayoutNode, files: SessionFile[]): number {
  n.weight = 0;
  n.fileCount = 0;
  n.lines = 0;
  for (const idx of n.files) {
    n.weight += fileWeight(files[idx].lines, files[idx].bytes);
    n.fileCount += 1;
    n.lines += files[idx].lines;
  }
  for (const child of sortedChildren(n.children)) {
    n.weight += computeWeight(child, files);
    n.fileCount += child.fileCount;
    n.lines += child.lines;
  }
  if (n.weight <= 0) n.weight = 1;
  return n.weight;
}

function inset(rect: Rect, pad: number): Rect {
  const out = { ...rect };
  if (out.w > pad * 2) {
    out.x += pad;
    out.w -= pad * 2;
  }
  if (out.d > pad * 2) {
    out.z += pad;
    out.d -= pad * 2;
  }
  return out;
}

function capAspect(rect: Rect, maxRatio: number): Rect {
  if (rect.w <= 0 || rect.d <= 0 || maxRatio <= 1) return rect;
  const out = { ...rect };
  if (out.w / out.d > maxRatio) {
    const newW = out.d * maxRatio;
    out.x += (out.w - newW) / 2;
    out.w = newW;
  } else if (out.d / out.w > maxRatio) {
    const newD = out.w * maxRatio;
    out.z += (out.d - newD) / 2;
    out.d = newD;
  }
  return out;
}

function worstAspect(row: LayoutItem[], side: number): number {
  if (row.length === 0 || side <= 0) return Infinity;
  let sum = 0;
  let minArea = Infinity;
  let maxArea = 0;
  for (const item of row) {
    sum += item.area;
    minArea = Math.min(minArea, item.area);
    maxArea = Math.max(maxArea, item.area);
  }
  if (sum <= 0 || minArea <= 0) return Infinity;
  const side2 = side * side;
  const sum2 = sum * sum;
  return Math.max((side2 * maxArea) / sum2, sum2 / (side2 * minArea));
}

function layoutRow(
  rect: Rect,
  row: LayoutItem[]
): { placed: { item: LayoutItem; rect: Rect }[]; remaining: Rect } {
  let sum = 0;
  for (const item of row) sum += item.area;
  if (sum <= 0) return { placed: [], remaining: rect };
  const placed: { item: LayoutItem; rect: Rect }[] = [];
  const remaining = { ...rect };
  if (remaining.w >= remaining.d) {
    const rowD = sum / remaining.w;
    let x = remaining.x;
    for (let i = 0; i < row.length; i++) {
      let w = row[i].area / rowD;
      if (i === row.length - 1) w = remaining.x + remaining.w - x;
      placed.push({
        item: row[i],
        rect: { x, z: remaining.z, w, d: rowD },
      });
      x += w;
    }
    remaining.z += rowD;
    remaining.d -= rowD;
  } else {
    const rowW = sum / remaining.d;
    let z = remaining.z;
    for (let i = 0; i < row.length; i++) {
      let d = row[i].area / rowW;
      if (i === row.length - 1) d = remaining.z + remaining.d - z;
      placed.push({
        item: row[i],
        rect: { x: remaining.x, z, w: rowW, d },
      });
      z += d;
    }
    remaining.x += rowW;
    remaining.w -= rowW;
  }
  if (remaining.w < 0) remaining.w = 0;
  if (remaining.d < 0) remaining.d = 0;
  return { placed, remaining };
}

function squarify(
  rect: Rect,
  items: LayoutItem[]
): { item: LayoutItem; rect: Rect }[] {
  let remaining = { ...rect };
  const placed: { item: LayoutItem; rect: Rect }[] = [];
  let row: LayoutItem[] = [];
  for (let idx = 0; idx < items.length; idx++) {
    const item = items[idx];
    if (item.area <= 0) continue;
    const side = Math.min(remaining.w, remaining.d);
    if (
      row.length < 2 ||
      idx === items.length - 1 ||
      worstAspect([...row, item], side) <= worstAspect(row, side)
    ) {
      row.push(item);
      continue;
    }
    const laid = layoutRow(remaining, row);
    placed.push(...laid.placed);
    remaining = laid.remaining;
    row = [item];
  }
  if (row.length > 0) {
    placed.push(...layoutRow(remaining, row).placed);
  }
  return placed;
}

function layoutNode(
  n: LayoutNode,
  rect: Rect,
  files: SessionFile[],
  dirs: SessionDir[]
) {
  if (n.path) {
    dirs.push({
      path: n.path,
      depth: n.path.split("/").length,
      rect: { ...rect },
      fileCount: n.fileCount,
      lines: n.lines,
    });
  }

  const items: LayoutItem[] = [];
  for (const child of sortedChildren(n.children)) {
    items.push({
      name: child.name,
      kind: "dir",
      idx: -1,
      node: child,
      weight: child.weight,
      area: 0,
    });
  }
  for (const idx of n.files) {
    items.push({
      name: files[idx].path,
      kind: "file",
      idx,
      weight: fileWeight(files[idx].lines, files[idx].bytes),
      area: 0,
    });
  }
  items.sort((a, b) => {
    if (a.weight === b.weight) return a.name.localeCompare(b.name);
    return b.weight - a.weight;
  });

  let total = 0;
  for (const item of items) total += item.weight;
  if (total <= 0) return;

  const scale = (rect.w * rect.d) / total;
  for (const item of items) item.area = item.weight * scale;

  for (const placed of squarify(rect, items)) {
    const childRect = capAspect(inset(placed.rect, 0.08), 40);
    if (placed.item.kind === "dir" && placed.item.node) {
      layoutNode(placed.item.node, childRect, files, dirs);
    } else if (placed.item.kind === "file") {
      files[placed.item.idx].rect = childRect;
    }
  }
}

function applyTreemap(files: SessionFile[]): SessionDir[] {
  const root = buildTree(files);
  computeWeight(root, files);
  const dirs: SessionDir[] = [];
  layoutNode(root, { x: 0, z: 0, w: ROOT_SIZE, d: ROOT_SIZE }, files, dirs);
  return dirs;
}

/** Upper bound on trace-seated files so a noisy session cannot swamp the map. */
const MAX_SEATED = 400;

/**
 * A district path ("superopen/web/src/x.ts") is repo-relative to another
 * checkout, so it has to be resolved through that repo's root before the
 * filesystem can be asked whether the file is really there.
 */
function absoluteFor(
  path: string,
  root: string,
  districts: ReadonlyMap<string, string>,
): string {
  if (isAbsolute(path)) return path;
  const cut = path.indexOf("/");
  if (cut > 0) {
    const owner = districts.get(path.slice(0, cut));
    if (owner) return join(owner, path.slice(cut + 1));
  }
  return join(root, path);
}

/**
 * Put trace-touched files on the map before playback runs.
 *
 * A session frequently edits a repo other than the one being mapped (chat
 * opened in one workspace, tool calls in another), and the walk also skips
 * dotted or deleted paths. Those touches still earned a landing: without a
 * file on the map they carry no id, so terrain grows no column and the trail
 * has no point to arc between. Paths that resolve under the mapped root are
 * seated as real tiles; everything else becomes a ghost.
 */
export function seatTracePaths(
  sessionMap: SessionMap,
  paths: Iterable<string>,
  districts: ReadonlyMap<string, string> = new Map(),
): void {
  const root = sessionMap.repo.root;
  const known = new Set(sessionMap.files.map((f) => f.path));
  let seated = 0;
  for (const path of paths) {
    if (!path || known.has(path)) continue;
    if (seated >= MAX_SEATED || sessionMap.files.length >= MAX_FILES) {
      sessionMap.repo.truncated = true;
      break;
    }
    known.add(path);
    seated++;

    let bytes = 0;
    let ghost = true;
    try {
      const st = statSync(absoluteFor(path, root, districts));
      if (st.isFile()) {
        bytes = st.size;
        ghost = false;
      }
    } catch {
      /* nothing on disk to measure: ghost */
    }
    const dir = dirname(path);
    sessionMap.files.push({
      id: 0,
      path,
      dir: dir === "." ? "" : dir,
      lines: ghost ? 0 : estimateLines(bytes),
      bytes,
      lang: extname(path).replace(/^\./, "") || undefined,
      rect: { x: 0, z: 0, w: 1, d: 1 },
      ghost,
    });
  }
  if (seated > 0) {
    // Scene meshes index files by array position, so ids must stay dense.
    sessionMap.files.sort((a, b) => a.path.localeCompare(b.path));
    sessionMap.files.forEach((f, i) => {
      f.id = i;
    });
    sessionMap.dirs = applyTreemap(sessionMap.files);
  }
  sessionMap.roots = computeRoots(sessionMap, districts);
}

/** Build the session map in memory; v2 never writes UI caches under .so. */
export function getSessionMap(): SessionMap {
  const root = repoRoot();
  const listed = walkFiles(root);
  const truncated = listed.length >= MAX_FILES;
  const files: SessionFile[] = listed.map((f, id) => {
    const dir = dirname(f.path);
    return {
      id,
      path: f.path,
      dir: dir === "." ? "" : dir,
      lines: estimateLines(f.bytes),
      bytes: f.bytes,
      lang: extname(f.path).replace(/^\./, "") || undefined,
      rect: { x: 0, z: 0, w: 1, d: 1 },
      ghost: false,
    };
  });
  files.sort((a, b) => a.path.localeCompare(b.path));
  files.forEach((f, i) => {
    f.id = i;
  });

  const dirs = applyTreemap(files);

  const sessionMap: SessionMap = {
    version: CACHE_VERSION,
    repo: {
      root,
      dirty: true,
      generatedAt: new Date().toISOString(),
      truncated,
    },
    roots: [],
    files,
    dirs,
    layout: {
      algorithm: "squarified-treemap-v1",
      weight: "sqrt(max(lines, bytes/4096, 16))",
    },
  };
  sessionMap.roots = computeRoots(sessionMap, new Map());

  return sessionMap;
}

function computeRoots(
  sessionMap: SessionMap,
  districts: ReadonlyMap<string, string>,
): MapRoot[] {
  const counts = new Map<string, number>();
  for (const f of sessionMap.files) {
    const cut = f.path.indexOf("/");
    const head = cut > 0 ? f.path.slice(0, cut) : "";
    const prefix = districts.has(head) ? head : "";
    counts.set(prefix, (counts.get(prefix) ?? 0) + 1);
  }
  const isGit = (path: string) => {
    try {
      return statSync(join(path, ".git")) != null;
    } catch {
      return false;
    }
  };
  const roots: MapRoot[] = [
    {
      prefix: "",
      name: basename(sessionMap.repo.root) || "repo",
      path: sessionMap.repo.root,
      git: isGit(sessionMap.repo.root),
      files: counts.get("") ?? 0,
    },
  ];
  for (const [prefix, path] of districts) {
    const files = counts.get(prefix) ?? 0;
    if (files === 0) continue;
    roots.push({ prefix, name: prefix, path, git: isGit(path), files });
  }
  return roots;
}

export function sessionKey(harness: string, sessionDir: string): string {
  const h = createHash("sha256");
  h.update(harness);
  h.update("\0");
  h.update(sessionDir);
  return `${harness}-${h.digest("hex").slice(0, 24)}`;
}
