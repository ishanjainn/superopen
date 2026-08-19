"use client";

import { useEffect, useRef } from "react";
import * as THREE from "three";
import { OrbitControls } from "three/examples/jsm/controls/OrbitControls.js";
import {
  BlendFunction,
  BloomEffect,
  EffectComposer,
  EffectPass,
  RenderPass,
} from "postprocessing";
import { useThemeOptional } from "@/components/shell/theme-provider";
import { DEFAULT_EDGE_COLOR, EDGE_COLORS, labelColor } from "./colors";
import {
  bloomIntensityScale,
  edgeIntensityScale,
  nodeBoostScale,
  nodeGlowBoost,
} from "./density";
import {
  DEFAULT_GRAPH_DISPLAY,
  type GraphData,
  type GraphDisplaySettings,
  type GraphNode,
} from "./types";

const POINT_MODE_THRESHOLD = 75_000;
const IDLE_ROTATE_MS = 60_000;
const BLOOM_BASE = 1.45;
/** Light stage: colors fade toward paper instead of toward black. */
const PAPER = new THREE.Color("#ffffff");

const LABEL_FONT_PX = 64;
const LABEL_FONT = `600 ${LABEL_FONT_PX}px Inter, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif`;
const LABEL_MAX_TEXT_WIDTH = 720;
const LABEL_PAD_X = 24;
const LABEL_PAD_Y = 14;
const LABEL_STROKE = 8;

export interface StellarGraphSceneProps {
  data: GraphData;
  highlightedIds?: Set<number> | null;
  focusIds?: Set<number> | null;
  showLabels?: boolean;
  display?: GraphDisplaySettings;
  className?: string;
  onHover?: (node: GraphNode | null) => void;
  onNodeClick?: (node: GraphNode) => void;
  onBackgroundClick?: () => void;
}

function sphereDetail(count: number): [number, number, number] {
  if (count <= 8000) return [1, 32, 24];
  if (count <= 25_000) return [1, 16, 12];
  return [1, 10, 7];
}

function clusterKey(path?: string): string {
  const parts = (path || "").split("/");
  return parts.slice(0, Math.min(2, parts.length)).join("/");
}

/** Fit camera to the highlighted set: center plus spread-scaled distance. */
function cameraTarget(nodes: GraphNode[], ids: Set<number>) {
  let x = 0;
  let y = 0;
  let z = 0;
  let count = 0;
  for (const node of nodes) {
    if (!ids.has(node.id)) continue;
    x += node.x;
    y += node.y;
    z += node.z;
    count++;
  }
  if (count === 0) return null;
  x /= count;
  y /= count;
  z /= count;
  let spread = 0;
  for (const node of nodes) {
    if (!ids.has(node.id)) continue;
    const d = Math.sqrt((node.x - x) ** 2 + (node.y - y) ** 2 + (node.z - z) ** 2);
    if (d > spread) spread = d;
  }
  const base = count <= 5 ? 300 : 200;
  const distance = Math.max(base, spread * 3);
  return {
    lookAt: new THREE.Vector3(x, y, z),
    position: new THREE.Vector3(x + distance * 0.2, y + distance * 0.15, z + distance),
  };
}

/** Dark stage glows above white; light stage fades toward paper instead. */
function paintNode(
  color: THREE.Color,
  lit: boolean,
  dark: boolean,
  nodeBoost: number,
) {
  if (!lit) {
    if (dark) color.multiplyScalar(0.15);
    else color.lerp(PAPER, 0.82);
    return;
  }
  if (!dark) {
    color.multiplyScalar(0.72);
    return;
  }
  const boost = nodeGlowBoost(color.r, color.g, color.b);
  color.multiplyScalar(1 + (boost - 1) * nodeBoost);
}

function paintEdge(color: THREE.Color, intensity: number, dark: boolean) {
  if (dark) color.multiplyScalar(intensity);
  else color.lerp(PAPER, 1 - Math.min(1, intensity * 1.6));
}

let pointTexture: THREE.CanvasTexture | null = null;
function createPointTexture(): THREE.CanvasTexture {
  if (pointTexture) return pointTexture;
  const size = 64;
  const canvas = document.createElement("canvas");
  canvas.width = size;
  canvas.height = size;
  const ctx = canvas.getContext("2d")!;
  const gradient = ctx.createRadialGradient(
    size / 2,
    size / 2,
    0,
    size / 2,
    size / 2,
    size / 2,
  );
  gradient.addColorStop(0, "rgba(255,255,255,1)");
  gradient.addColorStop(0.5, "rgba(255,255,255,0.9)");
  gradient.addColorStop(1, "rgba(255,255,255,0)");
  ctx.fillStyle = gradient;
  ctx.fillRect(0, 0, size, size);
  pointTexture = new THREE.CanvasTexture(canvas);
  return pointTexture;
}

function truncateToWidth(
  ctx: CanvasRenderingContext2D,
  text: string,
  maxWidth: number,
): string {
  if (ctx.measureText(text).width <= maxWidth) return text;
  let low = 0;
  let high = text.length;
  while (low < high) {
    const mid = Math.ceil((low + high) / 2);
    const candidate = `${text.slice(0, mid)}...`;
    if (ctx.measureText(candidate).width <= maxWidth) low = mid;
    else high = mid - 1;
  }
  return `${text.slice(0, Math.max(1, low))}...`;
}

function createLabel(node: GraphNode, dark: boolean): THREE.Sprite | null {
  const canvas = document.createElement("canvas");
  const ctx = canvas.getContext("2d");
  if (!ctx) return null;
  ctx.font = LABEL_FONT;
  const text = truncateToWidth(ctx, node.name, LABEL_MAX_TEXT_WIDTH);
  const textWidth = Math.ceil(ctx.measureText(text).width);
  const width = Math.max(1, textWidth + LABEL_PAD_X * 2 + LABEL_STROKE * 2);
  const height = LABEL_FONT_PX + LABEL_PAD_Y * 2 + LABEL_STROKE * 2;
  const dpr = Math.min(window.devicePixelRatio || 1, 2);
  canvas.width = Math.ceil(width * dpr);
  canvas.height = Math.ceil(height * dpr);
  ctx.scale(dpr, dpr);
  ctx.font = LABEL_FONT;
  ctx.textAlign = "center";
  ctx.textBaseline = "middle";
  ctx.lineJoin = "round";
  ctx.lineWidth = LABEL_STROKE;
  const paint = new THREE.Color(node.color);
  if (!dark) paint.multiplyScalar(0.62);
  ctx.strokeStyle = dark ? "rgba(0, 0, 0, 0.9)" : "rgba(255, 255, 255, 0.92)";
  ctx.fillStyle = `#${paint.getHexString()}`;
  ctx.strokeText(text, width / 2, height / 2);
  ctx.fillText(text, width / 2, height / 2);

  const texture = new THREE.CanvasTexture(canvas);
  texture.colorSpace = THREE.SRGBColorSpace;
  texture.minFilter = THREE.LinearFilter;
  texture.magFilter = THREE.LinearFilter;
  texture.generateMipmaps = false;
  texture.needsUpdate = true;

  const material = new THREE.SpriteMaterial({
    map: texture,
    transparent: true,
    depthWrite: false,
    toneMapped: false,
  });
  const sprite = new THREE.Sprite(material);
  const spriteHeight = Math.max(1.8, node.size * 0.4) * (height / LABEL_FONT_PX);
  const spriteWidth = spriteHeight * (width / height);
  sprite.scale.set(spriteWidth, spriteHeight, 1);
  sprite.position.set(node.x, node.y + node.size * 0.7 + spriteHeight / 2, node.z);
  sprite.renderOrder = 20;
  sprite.frustumCulled = false;
  return sprite;
}

/** Anchored hover card. Built with textContent only - never markup. */
function createTooltip(): {
  el: HTMLDivElement;
  set: (node: GraphNode | null) => void;
} {
  const el = document.createElement("div");
  el.style.cssText = [
    "position:absolute",
    "left:0",
    "top:0",
    "z-index:6",
    "display:none",
    "pointer-events:none",
    "max-width:350px",
    "padding:8px 12px",
    "border-radius:8px",
    "border:1px solid rgba(255,255,255,.1)",
    "background:rgba(26,26,46,.95)",
    "backdrop-filter:blur(8px)",
    "box-shadow:0 20px 25px -5px rgb(0 0 0 / .4)",
    "font-size:12px",
    "line-height:1.45",
    "white-space:nowrap",
  ].join(";");

  const head = document.createElement("div");
  head.style.cssText = "display:flex;align-items:center;gap:6px;margin-bottom:4px";
  const dot = document.createElement("span");
  dot.style.cssText = "width:8px;height:8px;border-radius:9999px;flex:none";
  const name = document.createElement("span");
  name.style.cssText =
    "color:#fff;font-weight:500;overflow:hidden;text-overflow:ellipsis";
  const kind = document.createElement("span");
  kind.style.cssText = "color:rgba(255,255,255,.3);margin-left:4px;flex:none";
  head.append(dot, name, kind);

  const path = document.createElement("p");
  path.style.cssText = [
    "margin:0",
    "color:rgba(255,255,255,.3)",
    "font-family:ui-monospace,SFMono-Regular,Menlo,monospace",
    "font-size:11px",
    "overflow:hidden",
    "text-overflow:ellipsis",
  ].join(";");

  const hint = document.createElement("p");
  hint.style.cssText = "margin:4px 0 0;color:rgba(255,255,255,.2);font-size:10px";
  hint.textContent = "click to inspect";

  el.append(head, path, hint);

  const set = (node: GraphNode | null) => {
    if (!node) {
      el.style.display = "none";
      return;
    }
    dot.style.backgroundColor = labelColor(node.label);
    name.textContent = node.name;
    kind.textContent = node.label;
    const lines = node.start_line
      ? node.end_line && node.end_line !== node.start_line
        ? ` L${node.start_line}-${node.end_line}`
        : ` L${node.start_line}`
      : "";
    path.textContent = `${node.file_path || node.qualified_name}${lines}`;
    path.style.display = node.file_path || node.qualified_name ? "block" : "none";
    el.style.display = "block";
  };

  return { el, set };
}

function disposeObject(object: THREE.Object3D) {
  object.traverse((child) => {
    const mesh = child as THREE.Mesh;
    mesh.geometry?.dispose();
    const materials = Array.isArray(mesh.material) ? mesh.material : [mesh.material];
    for (const material of materials) {
      if (!material) continue;
      const sprite = material as THREE.SpriteMaterial;
      if (sprite.map && sprite.map !== pointTexture) sprite.map.dispose();
      material.dispose();
    }
  });
}

export function StellarGraphScene({
  data,
  highlightedIds = null,
  focusIds = null,
  showLabels = true,
  display = DEFAULT_GRAPH_DISPLAY,
  className,
  onHover,
  onNodeClick,
  onBackgroundClick,
}: StellarGraphSceneProps) {
  const hostRef = useRef<HTMLDivElement | null>(null);
  const callbacksRef = useRef({ onHover, onNodeClick, onBackgroundClick });
  useEffect(() => {
    callbacksRef.current = { onHover, onNodeClick, onBackgroundClick };
  });
  /** Keeps the camera where the user left it across filter/data rebuilds. */
  const viewRef = useRef<{ position: THREE.Vector3; target: THREE.Vector3 } | null>(
    null,
  );
  const dark = useThemeOptional()?.resolved === "dark";

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;
    if (getComputedStyle(host).position === "static") {
      host.style.position = "relative";
    }

    const scene = new THREE.Scene();
    // Transparent canvas: the paper grid behind the stage stays visible.
    scene.background = null;
    const camera = new THREE.PerspectiveCamera(
      50,
      host.clientWidth / Math.max(host.clientHeight, 1),
      0.1,
      100_000,
    );
    if (viewRef.current) {
      camera.position.copy(viewRef.current.position);
    } else {
      camera.position.set(0, 0, 800);
    }

    const renderer = new THREE.WebGLRenderer({
      antialias: false,
      alpha: true,
      powerPreference: "high-performance",
    });
    renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 1.5));
    renderer.setSize(host.clientWidth || 1, host.clientHeight || 1);
    renderer.setClearColor(0x000000, 0);
    renderer.outputColorSpace = THREE.SRGBColorSpace;
    renderer.toneMapping = THREE.NoToneMapping;
    renderer.domElement.style.display = "block";
    renderer.domElement.style.width = "100%";
    renderer.domElement.style.height = "100%";
    renderer.domElement.style.touchAction = "none";
    host.appendChild(renderer.domElement);

    // Bloom only reads as light on the dark stage; skip it on paper.
    let composer: EffectComposer | null = null;
    if (dark && display.bloom > 0) {
      composer = new EffectComposer(renderer, {
        frameBufferType: THREE.HalfFloatType,
        multisampling: 0,
      });
      composer.addPass(new RenderPass(scene, camera));
      composer.addPass(
        new EffectPass(
          camera,
          new BloomEffect({
            blendFunction: BlendFunction.ADD,
            luminanceThreshold: 0.3,
            luminanceSmoothing: 0.7,
            intensity: BLOOM_BASE * bloomIntensityScale(data.nodes.length) * display.bloom,
            mipmapBlur: true,
            radius: 0.6,
          }),
        ),
      );
      composer.setSize(host.clientWidth || 1, host.clientHeight || 1);
    }

    const controls = new OrbitControls(camera, renderer.domElement);
    controls.enableDamping = true;
    controls.dampingFactor = 0.08;
    controls.rotateSpeed = 0.5;
    controls.zoomSpeed = 1.5;
    controls.minDistance = 10;
    controls.maxDistance = 50_000;
    controls.autoRotateSpeed = 0.4;
    if (viewRef.current) {
      controls.target.copy(viewRef.current.target);
      controls.update();
    }

    const graph = new THREE.Group();
    scene.add(graph);
    const hasHighlight = Boolean(highlightedIds?.size);
    const nodeBoost = nodeBoostScale(data.nodes.length) * display.nodeGlow;
    const blending = dark ? THREE.AdditiveBlending : THREE.NormalBlending;
    const tempColor = new THREE.Color();
    let pickObject: THREE.Object3D;

    if (data.nodes.length > POINT_MODE_THRESHOLD) {
      const positions = new Float32Array(data.nodes.length * 3);
      const colors = new Float32Array(data.nodes.length * 3);
      for (let i = 0; i < data.nodes.length; i++) {
        const node = data.nodes[i];
        positions.set([node.x, node.y, node.z], i * 3);
        tempColor.set(node.color);
        paintNode(
          tempColor,
          !hasHighlight || Boolean(highlightedIds?.has(node.id)),
          dark,
          nodeBoost,
        );
        colors.set([tempColor.r, tempColor.g, tempColor.b], i * 3);
      }
      const geometry = new THREE.BufferGeometry();
      geometry.setAttribute("position", new THREE.BufferAttribute(positions, 3));
      geometry.setAttribute("color", new THREE.BufferAttribute(colors, 3));
      const material = new THREE.PointsMaterial({
        vertexColors: true,
        size: 4,
        sizeAttenuation: true,
        map: createPointTexture(),
        alphaTest: 0.35,
        transparent: true,
        toneMapped: false,
      });
      const points = new THREE.Points(geometry, material);
      graph.add(points);
      pickObject = points;
    } else {
      const [radius, widthSegments, heightSegments] = sphereDetail(data.nodes.length);
      const geometry = new THREE.SphereGeometry(radius, widthSegments, heightSegments);
      const material = new THREE.MeshBasicMaterial({
        vertexColors: true,
        toneMapped: false,
      });
      const cloud = new THREE.InstancedMesh(geometry, material, data.nodes.length);
      cloud.frustumCulled = false;
      const matrix = new THREE.Matrix4();
      const offset = new THREE.Vector3();
      const rotation = new THREE.Quaternion();
      const scale = new THREE.Vector3();
      const color = new THREE.Color();
      // vertexColors reads the geometry's `color` attribute, so it must be an
      // instanced attribute - setColorAt would leave every sphere black.
      const colors = new Float32Array(data.nodes.length * 3);
      for (let i = 0; i < data.nodes.length; i++) {
        const node = data.nodes[i];
        const lit = !hasHighlight || Boolean(highlightedIds?.has(node.id));
        const size = node.size * (lit ? 0.5 : 0.2);
        offset.set(node.x, node.y, node.z);
        scale.set(size, size, size);
        matrix.compose(offset, rotation, scale);
        cloud.setMatrixAt(i, matrix);
        color.set(node.color);
        paintNode(color, lit, dark, nodeBoost);
        colors.set([color.r, color.g, color.b], i * 3);
      }
      geometry.setAttribute("color", new THREE.InstancedBufferAttribute(colors, 3));
      cloud.instanceMatrix.needsUpdate = true;
      graph.add(cloud);
      pickObject = cloud;
    }

    const byID = new Map(data.nodes.map((node) => [node.id, node]));
    const edgePositions = new Float32Array(data.edges.length * 6);
    const edgeColors = new Float32Array(data.edges.length * 6);
    const density = edgeIntensityScale(data.edges.length) * display.edgeBrightness;
    let edgeCount = 0;
    for (const edge of data.edges) {
      const source = byID.get(edge.source);
      const target = byID.get(edge.target);
      if (!source || !target) continue;
      const sourceLit = !hasHighlight || highlightedIds?.has(source.id);
      const targetLit = !hasHighlight || highlightedIds?.has(target.id);
      if (hasHighlight && !sourceLit && !targetLit) continue;
      let intensity =
        clusterKey(source.file_path) === clusterKey(target.file_path) ? 0.25 : 0.06;
      intensity = hasHighlight
        ? sourceLit && targetLit
          ? 0.5
          : 0.04 * density
        : intensity * density;
      const offset = edgeCount * 6;
      edgePositions.set(
        [source.x, source.y, source.z, target.x, target.y, target.z],
        offset,
      );
      tempColor.set(EDGE_COLORS[edge.type] ?? DEFAULT_EDGE_COLOR);
      paintEdge(tempColor, intensity, dark);
      edgeColors.set(
        [tempColor.r, tempColor.g, tempColor.b, tempColor.r, tempColor.g, tempColor.b],
        offset,
      );
      edgeCount++;
    }
    const edgeGeometry = new THREE.BufferGeometry();
    edgeGeometry.setAttribute(
      "position",
      new THREE.BufferAttribute(edgePositions.slice(0, edgeCount * 6), 3),
    );
    edgeGeometry.setAttribute(
      "color",
      new THREE.BufferAttribute(edgeColors.slice(0, edgeCount * 6), 3),
    );
    graph.add(
      new THREE.LineSegments(
        edgeGeometry,
        new THREE.LineBasicMaterial({
          vertexColors: true,
          transparent: true,
          opacity: 1,
          blending,
          depthWrite: false,
          toneMapped: false,
        }),
      ),
    );

    if (showLabels) {
      const labelNodes = (
        hasHighlight
          ? data.nodes.filter((node) => highlightedIds?.has(node.id))
          : [...data.nodes]
      )
        .sort((a, b) => b.size - a.size)
        .slice(0, 80);
      for (const node of labelNodes) {
        const label = createLabel(node, dark);
        if (label) graph.add(label);
      }
    }

    // Fly only toward an explicit focus; never re-fit the whole graph.
    let animationTarget: ReturnType<typeof cameraTarget> = null;
    let animationProgress = 1;
    if (focusIds?.size) {
      animationTarget = cameraTarget(data.nodes, focusIds);
      animationProgress = 0;
    }

    const tooltip = createTooltip();
    host.appendChild(tooltip.el);
    const tooltipVec = new THREE.Vector3();
    let tooltipNode: GraphNode | null = null;

    const raycaster = new THREE.Raycaster();
    raycaster.params.Points = { threshold: 3 };
    const pointer = new THREE.Vector2();
    let hoveredIndex = -1;
    let downAt: { x: number; y: number } | null = null;
    let hoverFrame = 0;
    let lastInteraction = Date.now();
    const pick = (event: PointerEvent): GraphNode | null => {
      const rect = renderer.domElement.getBoundingClientRect();
      pointer.x = ((event.clientX - rect.left) / rect.width) * 2 - 1;
      pointer.y = -((event.clientY - rect.top) / rect.height) * 2 + 1;
      raycaster.setFromCamera(pointer, camera);
      const hit = raycaster.intersectObject(pickObject, false)[0];
      const index =
        hit?.instanceId ?? (typeof hit?.index === "number" ? hit.index : undefined);
      return index !== undefined ? data.nodes[index] ?? null : null;
    };
    const onMove = (event: PointerEvent) => {
      if (hoverFrame) return;
      hoverFrame = requestAnimationFrame(() => {
        hoverFrame = 0;
        const node = pick(event);
        const index = node ? data.nodes.indexOf(node) : -1;
        if (index === hoveredIndex) return;
        hoveredIndex = index;
        tooltipNode = node;
        tooltip.set(node);
        renderer.domElement.style.cursor = node ? "pointer" : "grab";
        callbacksRef.current.onHover?.(node);
      });
    };
    const onDown = (event: PointerEvent) => {
      lastInteraction = Date.now();
      downAt = { x: event.clientX, y: event.clientY };
    };
    const onUp = (event: PointerEvent) => {
      lastInteraction = Date.now();
      if (!downAt) return;
      const moved = Math.hypot(event.clientX - downAt.x, event.clientY - downAt.y);
      downAt = null;
      if (moved > 5) return;
      const node = pick(event);
      if (node) callbacksRef.current.onNodeClick?.(node);
      else callbacksRef.current.onBackgroundClick?.();
    };
    const onWheel = () => {
      lastInteraction = Date.now();
    };
    renderer.domElement.addEventListener("pointermove", onMove);
    renderer.domElement.addEventListener("pointerdown", onDown);
    renderer.domElement.addEventListener("pointerup", onUp);
    renderer.domElement.addEventListener("wheel", onWheel, { passive: true });

    const resize = () => {
      const width = host.clientWidth || 1;
      const height = host.clientHeight || 1;
      camera.aspect = width / height;
      camera.updateProjectionMatrix();
      renderer.setSize(width, height);
      composer?.setSize(width, height);
    };
    const observer = new ResizeObserver(resize);
    observer.observe(host);

    const placeTooltip = () => {
      if (!tooltipNode) return;
      tooltipVec
        .set(
          tooltipNode.x,
          tooltipNode.y + tooltipNode.size * 0.7,
          tooltipNode.z,
        )
        .project(camera);
      if (tooltipVec.z >= 1) {
        tooltip.el.style.display = "none";
        return;
      }
      const width = host.clientWidth || 1;
      const height = host.clientHeight || 1;
      const x = (tooltipVec.x * 0.5 + 0.5) * width;
      const y = (-tooltipVec.y * 0.5 + 0.5) * height;
      tooltip.el.style.display = "block";
      tooltip.el.style.transform = `translate(${x}px, ${y}px) translate(-50%, calc(-100% - 10px))`;
    };

    let frame = 0;
    const render = () => {
      controls.autoRotate = Date.now() - lastInteraction > IDLE_ROTATE_MS;
      if (animationTarget && animationProgress < 1) {
        animationProgress = Math.min(1, animationProgress + 0.02);
        const easing = 1 - Math.pow(1 - animationProgress, 3);
        camera.position.lerp(animationTarget.position, easing * 0.08);
        controls.target.lerp(animationTarget.lookAt, easing * 0.08);
      }
      controls.update();
      if (composer) composer.render();
      else renderer.render(scene, camera);
      placeTooltip();
      frame = requestAnimationFrame(render);
    };
    render();

    return () => {
      viewRef.current = {
        position: camera.position.clone(),
        target: controls.target.clone(),
      };
      cancelAnimationFrame(frame);
      if (hoverFrame) cancelAnimationFrame(hoverFrame);
      observer.disconnect();
      renderer.domElement.removeEventListener("pointermove", onMove);
      renderer.domElement.removeEventListener("pointerdown", onDown);
      renderer.domElement.removeEventListener("pointerup", onUp);
      renderer.domElement.removeEventListener("wheel", onWheel);
      callbacksRef.current.onHover?.(null);
      tooltip.el.remove();
      controls.dispose();
      disposeObject(graph);
      composer?.dispose();
      renderer.dispose();
      scene.clear();
      if (renderer.domElement.parentNode === host) host.removeChild(renderer.domElement);
    };
  }, [data, highlightedIds, focusIds, showLabels, display, dark]);

  return <div ref={hostRef} className={className || "h-full w-full"} />;
}
