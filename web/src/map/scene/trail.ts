import * as THREE from "three";
import { trailColorForRecency } from "./sceneUtils";

const SAMPLES = 14;
const MAX_ARCS = 11; // playback keeps 12 recent targets → at most 11 arcs

// Fixed-capacity trail: the arc soup changes every playback tick, so the
// geometry and material are allocated once and only the attribute contents
// (plus the draw range) are rewritten in place.
export class TrailRenderer {
  readonly object: THREE.LineSegments;
  private readonly positions: THREE.BufferAttribute;
  private readonly colors: THREE.BufferAttribute;
  private readonly lift: number;
  private readonly arcPoints = Array.from({ length: SAMPLES + 1 }, () => new THREE.Vector3());
  private readonly mid = new THREE.Vector3();
  private readonly curve = new THREE.QuadraticBezierCurve3();
  private readonly color = new THREE.Color();

  constructor(lift: number) {
    this.lift = lift;
    const vertexCount = MAX_ARCS * SAMPLES * 2;
    const geometry = new THREE.BufferGeometry();
    this.positions = new THREE.BufferAttribute(new Float32Array(vertexCount * 3), 3);
    this.positions.setUsage(THREE.DynamicDrawUsage);
    this.colors = new THREE.BufferAttribute(new Float32Array(vertexCount * 3), 3);
    this.colors.setUsage(THREE.DynamicDrawUsage);
    geometry.setAttribute("position", this.positions);
    geometry.setAttribute("color", this.colors);
    geometry.setDrawRange(0, 0);
    // Normal blending so strokes keep alpha on the shared transparent paper.
    const material = new THREE.LineBasicMaterial({
      vertexColors: true,
      transparent: true,
      blending: THREE.NormalBlending,
      depthWrite: false,
      fog: false,
      toneMapped: false,
    });
    this.object = new THREE.LineSegments(geometry, material);
    this.object.frustumCulled = false;
    this.object.renderOrder = 12;
  }

  setDark(_dark: boolean) {
    const material = this.object.material as THREE.LineBasicMaterial;
    material.blending = THREE.NormalBlending;
    material.needsUpdate = true;
  }

  // points are the recent fixations, oldest first, y already at arc height
  update(points: THREE.Vector3[]) {
    const arcs = Math.min(Math.max(points.length - 1, 0), MAX_ARCS);
    if (arcs === 0) {
      this.object.geometry.setDrawRange(0, 0);
      return;
    }
    const start = points.length - 1 - arcs;
    let v = 0;
    for (let i = 1; i <= arcs; i++) {
      const a = points[start + i - 1];
      const b = points[start + i];
      this.mid.copy(a).lerp(b, 0.5);
      this.mid.y = Math.max(a.y, b.y) + a.distanceTo(b) * 0.22 + this.lift;
      this.curve.v0.copy(a);
      this.curve.v1.copy(this.mid);
      this.curve.v2.copy(b);
      for (let s = 0; s <= SAMPLES; s++) {
        this.curve.getPoint(s / SAMPLES, this.arcPoints[s]);
      }
      const recency = i / arcs;
      trailColorForRecency(recency, this.color);
      // Squared falloff: the newest hop burns, older ones drop away fast, so
      // the head of the walk is obvious in a dense arc soup.
      this.color.multiplyScalar(0.05 + 0.95 * recency * recency);
      for (let s = 0; s < SAMPLES; s++) {
        const pa = this.arcPoints[s];
        const pb = this.arcPoints[s + 1];
        this.positions.setXYZ(v, pa.x, pa.y, pa.z);
        this.colors.setXYZ(v, this.color.r, this.color.g, this.color.b);
        v++;
        this.positions.setXYZ(v, pb.x, pb.y, pb.z);
        this.colors.setXYZ(v, this.color.r, this.color.g, this.color.b);
        v++;
      }
    }
    this.object.geometry.setDrawRange(0, v);
    this.positions.needsUpdate = true;
    this.colors.needsUpdate = true;
  }
}
