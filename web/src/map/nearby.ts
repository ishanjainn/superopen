import type { CityFile } from "./types";
import type { TreeLayout } from "./scene/treeLayout";

const NEIGHBOR_LIMIT = 48;

function centerOf(file: CityFile, layout?: TreeLayout | null): { x: number; z: number } {
  if (layout) {
    const leaf = layout.leaf.get(file.id);
    if (leaf) return leaf;
  }
  return {
    x: file.rect.x + file.rect.w / 2,
    z: file.rect.z + file.rect.d / 2,
  };
}

/**
 * Files near `selected`, closest first (selected first). Used by Inspect Prev/Next
 * so dense clusters are easy to step through.
 */
export function nearbyFiles(
  files: CityFile[],
  selected: CityFile,
  layout?: TreeLayout | null
): CityFile[] {
  const origin = centerOf(selected, layout);
  const ranked = files
    .map((file) => {
      const c = centerOf(file, layout);
      const dx = c.x - origin.x;
      const dz = c.z - origin.z;
      return { file, dist: dx * dx + dz * dz };
    })
    .sort((a, b) => a.dist - b.dist || a.file.path.localeCompare(b.file.path));
  return ranked.slice(0, NEIGHBOR_LIMIT).map((r) => r.file);
}
