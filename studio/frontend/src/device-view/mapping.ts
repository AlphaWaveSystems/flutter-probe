// Pure geometry and gesture helpers. All pointer math is done in canvas
// backing pixels so devicePixelRatio and CSS scaling are handled uniformly.

export interface Size {
  w: number;
  h: number;
}

export interface Rect {
  x: number;
  y: number;
  w: number;
  h: number;
}

/** Where the device image sits inside a canvas (aspect-preserving). */
export function letterbox(canvasW: number, canvasH: number, deviceW: number, deviceH: number): Rect {
  if (!(canvasW > 0 && canvasH > 0 && deviceW > 0 && deviceH > 0)) {
    return { x: 0, y: 0, w: 0, h: 0 };
  }
  const scale = Math.min(canvasW / deviceW, canvasH / deviceH);
  const w = deviceW * scale;
  const h = deviceH * scale;
  return { x: (canvasW - w) / 2, y: (canvasH - h) / 2, w, h };
}

/** Canvas backing store size for a CSS size and devicePixelRatio. */
export function backingSize(cssW: number, cssH: number, dpr: number): Size {
  const r = Number.isFinite(dpr) && dpr > 0 ? dpr : 1;
  return { w: Math.max(1, Math.round(cssW * r)), h: Math.max(1, Math.round(cssH * r)) };
}

export interface PointerInput {
  clientX: number;
  clientY: number;
  /** Canvas bounding client rect (CSS pixels). */
  rect: Rect;
  /** Canvas backing store size (device pixels). */
  canvas: Size;
  /** Device logical screen size reported by the hub. */
  screen: Size;
}

export interface MappedPoint {
  /** Device logical x, clamped to [0, screen.w]. */
  x: number;
  /** Device logical y, clamped to [0, screen.h]. */
  y: number;
  /** False when the pointer is in the letterbox bars (the value is clamped). */
  inside: boolean;
}

/** Map a pointer position to device logical coordinates; clamps outside. */
export function mapPointer(p: PointerInput): MappedPoint | null {
  const { rect, canvas, screen } = p;
  if (!(rect.w > 0 && rect.h > 0)) return null;
  const lb = letterbox(canvas.w, canvas.h, screen.w, screen.h);
  if (!(lb.w > 0 && lb.h > 0)) return null;
  const bx = (p.clientX - rect.x) * (canvas.w / rect.w);
  const by = (p.clientY - rect.y) * (canvas.h / rect.h);
  const fx = (bx - lb.x) / lb.w;
  const fy = (by - lb.y) / lb.h;
  const inside = fx >= 0 && fx <= 1 && fy >= 0 && fy <= 1;
  const clamp01 = (v: number) => Math.min(1, Math.max(0, v));
  return { x: clamp01(fx) * screen.w, y: clamp01(fy) * screen.h, inside };
}

/** Device logical rectangle to canvas backing pixels (for the overlay). */
export function deviceRectToCanvas(r: Rect, canvas: Size, screen: Size): Rect {
  const lb = letterbox(canvas.w, canvas.h, screen.w, screen.h);
  if (!(lb.w > 0) || !(screen.w > 0) || !(screen.h > 0)) return { x: 0, y: 0, w: 0, h: 0 };
  const sx = lb.w / screen.w;
  const sy = lb.h / screen.h;
  return { x: lb.x + r.x * sx, y: lb.y + r.y * sy, w: r.w * sx, h: r.h * sy };
}

export const TAP_MAX_MOVE_PX = 10;
export const TAP_MAX_MS = 300;
export const LONG_PRESS_MIN_MS = 500;

export type GestureKind = "tap" | "swipe" | "long_press";

export interface GestureSample {
  /** Largest distance from the down point reached during the gesture (CSS px). */
  maxMovePx: number;
  /** Time between pointer down and up (ms). */
  durationMs: number;
}

/**
 * Classify a finished pointer gesture.
 *  - moved more than 10 px at any time: swipe (drag)
 *  - held >= 500 ms without moving: long press
 *  - otherwise (including 300..500 ms): tap
 */
export function classifyGesture(g: GestureSample): GestureKind {
  if (g.maxMovePx > TAP_MAX_MOVE_PX) return "swipe";
  if (g.durationMs >= LONG_PRESS_MIN_MS) return "long_press";
  return "tap";
}

/**
 * Local hit test against a flat node list: the smallest-area node containing
 * the point, preferring interactive or keyed nodes. Used only when the hub
 * has no hit capability but streams a tree.
 */
export function hitTestNodes<
  T extends { x: number; y: number; w: number; h: number; interactive: boolean; key: string | null },
>(nodes: ReadonlyArray<T>, x: number, y: number): T | null {
  let best: T | null = null;
  let bestScore = Infinity;
  for (const n of nodes) {
    if (!(x >= n.x && x <= n.x + n.w && y >= n.y && y <= n.y + n.h)) continue;
    const preferred = n.interactive || (n.key !== null && n.key !== "");
    const score = n.w * n.h + (preferred ? 0 : 1e12);
    if (score < bestScore) {
      best = n;
      bestScore = score;
    }
  }
  return best;
}

export function clamp(v: number, lo: number, hi: number): number {
  return Math.min(hi, Math.max(lo, v));
}
