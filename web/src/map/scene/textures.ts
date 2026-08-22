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

// Glyphs fill most of the box: the sprite is scaled so the box height matches
// a screen-pixel target, so padding here is height the name does not get.
const LABEL_FONT_PX = 34;
const LABEL_BOX_PX = 44;
const LABEL_PAD_X = 14;

/**
 * Directory names: light type on a dark halo (night) or ink on a light halo (paper).
 */
export function labelTexture(text: string, dark = true): { texture: THREE.Texture; aspect: number } {
  const font = `650 ${LABEL_FONT_PX}px "Inter var", Inter, ui-sans-serif, system-ui, -apple-system, "Segoe UI", "Helvetica Neue", sans-serif`;
  const measure = document.createElement("canvas").getContext("2d")!;
  measure.font = font;
  const width = Math.ceil(measure.measureText(text).width) + LABEL_PAD_X * 2;
  const height = LABEL_BOX_PX;
  // Draw against device pixels, not CSS pixels, or the texture is already soft
  // before the sprite is scaled down.
  const scale = Math.min(2, Math.max(1, Math.round(window.devicePixelRatio || 1)));
  const canvas = document.createElement("canvas");
  canvas.width = width * scale;
  canvas.height = height * scale;
  const ctx = canvas.getContext("2d")!;
  ctx.scale(scale, scale);
  ctx.font = font;
  ctx.textBaseline = "middle";
  ctx.textAlign = "center";
  ctx.lineJoin = "round";
  ctx.miterLimit = 2;
  ctx.lineWidth = 5;
  ctx.strokeStyle = dark ? "rgba(0, 0, 0, 0.85)" : "rgba(255, 255, 255, 0.92)";
  ctx.strokeText(text, width / 2, height / 2);
  ctx.fillStyle = dark ? "rgba(250, 250, 250, 0.97)" : "rgba(23, 23, 23, 0.96)";
  ctx.fillText(text, width / 2, height / 2);
  const texture = new THREE.CanvasTexture(canvas);
  // Labels are always minified toward their screen-size target, so trilinear
  // mipmapping plus anisotropy is what keeps the strokes from crawling.
  texture.minFilter = THREE.LinearMipmapLinearFilter;
  texture.magFilter = THREE.LinearFilter;
  texture.generateMipmaps = true;
  texture.anisotropy = 8;
  return { texture, aspect: width / height };
}
