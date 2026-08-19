export const EDGE_REFERENCE_COUNT = 2500;
export const NODE_REFERENCE_COUNT = 25_000;

export function edgeIntensityScale(edgeCount: number): number {
  if (edgeCount <= EDGE_REFERENCE_COUNT) return 1;
  return Math.max(0.05, Math.sqrt(EDGE_REFERENCE_COUNT / edgeCount));
}

function nodeFade(nodeCount: number): number {
  if (nodeCount <= NODE_REFERENCE_COUNT) return 0;
  return Math.min(1, (nodeCount - NODE_REFERENCE_COUNT) / 225_000);
}

export function bloomIntensityScale(nodeCount: number): number {
  return 1 - nodeFade(nodeCount) * 0.3;
}

export function nodeBoostScale(nodeCount: number): number {
  return 1 - nodeFade(nodeCount) * 0.2;
}

export function nodeGlowBoost(r: number, g: number, b: number): number {
  const blue = Math.max(0, b - Math.max(r, g));
  const red = Math.max(0, r - Math.max(g, b));
  return 1.35 + blue * 2.4 + red * 0.9;
}
