import * as THREE from "three";
import type { Touch } from "../types";

// Shared scene vocabulary - white stage; yellow / orange / pink data marks
// stay readable on both light and dark backdrops.
/** Jump arcs + walker glow - warm orange (middle of yellow→pink ramp). */
export const EMBER = new THREE.Color("#f97316");
export const TRAIL_YELLOW = new THREE.Color("#eab308");
export const TRAIL_ORANGE = new THREE.Color("#f97316");
export const TRAIL_PINK = new THREE.Color("#ec4899");

export const touchColors: Record<Touch | "selected", THREE.Color> = {
  hit: new THREE.Color("#eab308"), // yellow - seen/search
  read: new THREE.Color("#f97316"), // orange - read
  edit: new THREE.Color("#ec4899"), // pink - edit
  selected: new THREE.Color("#171717"), // near-black
};

/** Recency 0..1 → yellow → orange → pink for jump arcs. */
export function trailColorForRecency(recency: number, out = new THREE.Color()): THREE.Color {
  const t = Math.min(1, Math.max(0, recency));
  if (t < 0.5) {
    return out.copy(TRAIL_YELLOW).lerp(TRAIL_ORANGE, t * 2);
  }
  return out.copy(TRAIL_ORANGE).lerp(TRAIL_PINK, (t - 0.5) * 2);
}

// Distance along `dir` that fits every point inside the camera frustum.
// Returns null while the viewport has no usable aspect (hidden pane, tab in
// the background, mid-layout) - fitting then would park the camera at NaN
// forever; callers must retry once a real resize lands.
export function fitDistance(
  camera: THREE.PerspectiveCamera,
  dir: THREE.Vector3,
  points: Iterable<THREE.Vector3>
): number | null {
  if (!Number.isFinite(camera.aspect) || camera.aspect <= 0) return null;
  const forward = dir.clone().negate();
  const right = new THREE.Vector3().crossVectors(forward, new THREE.Vector3(0, 1, 0)).normalize();
  const up = new THREE.Vector3().crossVectors(right, forward);
  const tanV = Math.tan(THREE.MathUtils.degToRad(camera.fov) / 2);
  const tanH = tanV * camera.aspect;
  let distance = 0;
  for (const point of points) {
    const depth = point.dot(forward);
    distance = Math.max(
      distance,
      Math.abs(point.dot(right)) / tanH - depth,
      Math.abs(point.dot(up)) / tanV - depth
    );
  }
  return distance;
}

// Pan the camera (position + orbit target together, so the view direction
// holds) until `world` projects inside the viewport's safe area. The right
// margin reserves room for the inspector panel, which would otherwise sit
// exactly on top of the leaf the user just selected.
export function ensureVisible(
  camera: THREE.PerspectiveCamera,
  controls: { target: THREE.Vector3; update: () => void },
  world: THREE.Vector3,
  viewW: number,
  viewH: number,
  reservedRight: number
) {
  if (viewW === 0 || viewH === 0) return;
  const forward = camera.getWorldDirection(new THREE.Vector3());
  const depth = world.clone().sub(camera.position).dot(forward);
  if (depth <= 0) return; // behind the camera: panning math breaks down
  const projected = world.clone().project(camera);
  const sx = ((projected.x + 1) / 2) * viewW;
  const sy = ((1 - projected.y) / 2) * viewH;
  const safeL = 48;
  const safeR = Math.max(safeL + 60, viewW - reservedRight - 48);
  const safeT = 120;
  const safeB = viewH - 100;
  const targetX = Math.min(Math.max(sx, safeL), safeR);
  const targetY = Math.min(Math.max(sy, safeT), safeB);
  if (targetX === sx && targetY === sy) return;
  const tanV = Math.tan(THREE.MathUtils.degToRad(camera.fov) / 2);
  const tanH = tanV * camera.aspect;
  const right = new THREE.Vector3().crossVectors(forward, new THREE.Vector3(0, 1, 0)).normalize();
  const up = new THREE.Vector3().crossVectors(right, forward);
  // moving the camera right shifts the point left on screen, and vice versa
  const pan = right
    .multiplyScalar(((sx - targetX) * 2 * depth * tanH) / viewW)
    .addScaledVector(up, ((targetY - sy) * 2 * depth * tanV) / viewH);
  camera.position.add(pan);
  controls.target.add(pan);
  controls.update();
}

// One-line hover readout, driven imperatively from the scenes' pointermove
// handlers so hovering never re-renders the React tree.
export class SceneTip {
  private readonly el: HTMLDivElement;
  private readonly pathEl: HTMLSpanElement;
  private readonly metaEl: HTMLSpanElement;

  constructor(private readonly host: HTMLElement) {
    this.el = document.createElement("div");
    this.el.className = "scene-tip";
    this.pathEl = document.createElement("span");
    this.metaEl = document.createElement("span");
    this.metaEl.className = "dim";
    this.el.append(this.pathEl, this.metaEl);
    host.appendChild(this.el);
  }

  show(path: string, meta: string, clientX: number, clientY: number) {
    this.pathEl.textContent = path;
    this.metaEl.textContent = ` · ${meta}`;
    this.el.style.display = "block";
    const bounds = this.host.getBoundingClientRect();
    const x = clientX - bounds.left;
    const y = clientY - bounds.top;
    const left = Math.min(x + 14, Math.max(0, bounds.width - this.el.offsetWidth - 8));
    const top = Math.min(y + 16, Math.max(0, bounds.height - this.el.offsetHeight - 8));
    this.el.style.left = `${left}px`;
    this.el.style.top = `${top}px`;
  }

  hide() {
    this.el.style.display = "none";
  }

  dispose() {
    this.el.remove();
  }
}

export const prefersReducedMotion = () =>
  typeof window !== "undefined" && window.matchMedia("(prefers-reduced-motion: reduce)").matches;

/** Map-style orbit: drag pans, right-drag orbits, wheel zooms to cursor. */
export function configureMapControls(controls: {
  enableDamping: boolean;
  dampingFactor: number;
  minPolarAngle: number;
  maxPolarAngle: number;
  enableZoom: boolean;
  enablePan: boolean;
  enableRotate: boolean;
  zoomSpeed: number;
  panSpeed: number;
  rotateSpeed: number;
  screenSpacePanning: boolean;
  zoomToCursor: boolean;
  mouseButtons: { LEFT?: number | null; MIDDLE?: number | null; RIGHT?: number | null };
  touches: { ONE?: number | null; TWO?: number | null };
}) {
  controls.enableDamping = true;
  controls.dampingFactor = 0.08;
  controls.minPolarAngle = 0.05;
  controls.maxPolarAngle = Math.PI * 0.49;
  controls.enableZoom = true;
  controls.enablePan = true;
  controls.enableRotate = true;
  controls.zoomSpeed = 1.35;
  controls.panSpeed = 1.1;
  controls.rotateSpeed = 0.9;
  controls.screenSpacePanning = true;
  controls.zoomToCursor = true;
  // Default OrbitControls left-drags to rotate (orbits the semicircle).
  // Maps expect drag-to-pan; keep rotate on right-drag / ctrl+left.
  controls.mouseButtons = {
    LEFT: THREE.MOUSE.PAN,
    MIDDLE: THREE.MOUSE.DOLLY,
    RIGHT: THREE.MOUSE.ROTATE,
  };
  controls.touches = {
    ONE: THREE.TOUCH.PAN,
    TWO: THREE.TOUCH.DOLLY_ROTATE,
  };
}

/** Orbit zoom limits that still allow drilling into a local cluster on huge maps. */
export function applyZoomLimits(
  controls: { minDistance: number; maxDistance: number },
  mapSize: number,
  fittedDistance: number
) {
  // Keep min low so trackpad/wheel can dive into a fork on 10k-file maps.
  controls.minDistance = Math.max(0.8, mapSize * 0.004);
  controls.maxDistance = Math.max(mapSize * 4.5, fittedDistance * 2.2);
}

/**
 * Dolly + pan so `world` becomes the orbit target at a readable distance.
 * Used when the user clicks a file - pan alone was not enough to "zoom in".
 */
export function focusOnPoint(
  camera: THREE.PerspectiveCamera,
  controls: { target: THREE.Vector3; update: () => void; minDistance: number },
  world: THREE.Vector3,
  mapSize: number
) {
  const desired = Math.max(controls.minDistance * 1.4, Math.min(mapSize * 0.09, 42));
  const offset = camera.position.clone().sub(controls.target);
  if (offset.lengthSq() < 1e-6) {
    offset.set(0.3, 0.66, 0.47);
  }
  offset.setLength(desired);
  controls.target.copy(world);
  camera.position.copy(world).add(offset);
  controls.update();
}

export function disposeGroup(group: THREE.Group) {
  group.traverse((obj) => {
    if (obj instanceof THREE.Mesh || obj instanceof THREE.Line || obj instanceof THREE.Sprite) {
      obj.geometry?.dispose();
      const mat = obj.material as THREE.Material | THREE.Material[];
      if (Array.isArray(mat)) mat.forEach(disposeMaterial);
      else if (mat) disposeMaterial(mat);
    }
  });
}

function disposeMaterial(mat: THREE.Material) {
  // Material.dispose() does not free assigned textures; module-cached
  // textures (fireflyTexture/haloTexture) are marked shared and must survive.
  const map = (mat as THREE.Material & { map?: THREE.Texture | null }).map;
  if (map && !map.userData.shared) map.dispose();
  mat.dispose();
}
