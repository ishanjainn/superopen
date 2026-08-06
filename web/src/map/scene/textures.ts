import * as THREE from "three";

let fireflyMap: THREE.Texture | null = null;
export function fireflyTexture(): THREE.Texture {
  if (fireflyMap) return fireflyMap;
  const size = 64;
  const canvas = document.createElement("canvas");
  canvas.width = size;
  canvas.height = size;
  const ctx = canvas.getContext("2d")!;
  const g = ctx.createRadialGradient(size / 2, size / 2, 0, size / 2, size / 2, size / 2);
  g.addColorStop(0, "rgba(255,255,255,1)");
  g.addColorStop(0.25, "rgba(45,212,191,0.55)");
  g.addColorStop(1, "rgba(13,148,136,0)");
  ctx.fillStyle = g;
  ctx.fillRect(0, 0, size, size);
  fireflyMap = new THREE.CanvasTexture(canvas);
  fireflyMap.userData.shared = true; // module cache: disposeGroup must not free it
  return fireflyMap;
}

let haloMap: THREE.Texture | null = null;
export function haloTexture(): THREE.Texture {
  if (haloMap) return haloMap;
  const size = 128;
  const canvas = document.createElement("canvas");
  canvas.width = size;
  canvas.height = size;
  const ctx = canvas.getContext("2d")!;
  const g = ctx.createRadialGradient(size / 2, size / 2, 0, size / 2, size / 2, size / 2);
  g.addColorStop(0, "rgba(255,255,255,0.9)");
  g.addColorStop(0.4, "rgba(255,255,255,0.28)");
  g.addColorStop(1, "rgba(255,255,255,0)");
  ctx.fillStyle = g;
  ctx.fillRect(0, 0, size, size);
  haloMap = new THREE.CanvasTexture(canvas);
  haloMap.userData.shared = true; // module cache: disposeGroup must not free it
  return haloMap;
}

export function labelTexture(text: string): { texture: THREE.Texture; aspect: number } {
  const font = '600 30px ui-sans-serif, system-ui, sans-serif';
  const measure = document.createElement("canvas").getContext("2d")!;
  measure.font = font;
  const width = Math.ceil(measure.measureText(text).width) + 28;
  const height = 46;
  const canvas = document.createElement("canvas");
  canvas.width = width * 2;
  canvas.height = height * 2;
  const ctx = canvas.getContext("2d")!;
  ctx.scale(2, 2);
  ctx.font = font;
  ctx.textBaseline = "middle";
  ctx.textAlign = "center";
  // Soft white halo so names stay legible on the paper grid when zoomed in.
  ctx.lineJoin = "round";
  ctx.miterLimit = 2;
  ctx.lineWidth = 4;
  ctx.strokeStyle = "rgba(255, 255, 255, 0.92)";
  ctx.strokeText(text, width / 2, height / 2 + 1);
  ctx.fillStyle = "rgba(28, 25, 23, 0.94)";
  ctx.fillText(text, width / 2, height / 2 + 1);
  const texture = new THREE.CanvasTexture(canvas);
  texture.anisotropy = 4;
  return { texture, aspect: width / height };
}
