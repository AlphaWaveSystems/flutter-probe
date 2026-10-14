// DOM and canvas layer of the device view. All state that can be tested
// without a DOM lives in protocol.ts, mapping.ts, selector.ts, timing.ts and
// connection.ts. This file has no Wails or VS Code imports.
//
// Styling uses element.style (CSSOM) only, never <style> elements or style
// attributes in markup, so it also works in a webview with a strict CSP.
// Remote data is only ever written with textContent.

import { DeviceConnection } from "./connection.ts";
import type { WebSocketCtor } from "./connection.ts";
import {
  backingSize,
  classifyGesture,
  clamp,
  deviceRectToCanvas,
  hitTestNodes,
  letterbox,
  mapPointer,
  TAP_MAX_MOVE_PX,
} from "./mapping.ts";
import type { MappedPoint, Rect, Size } from "./mapping.ts";
import { chunkText } from "./protocol.ts";
import type {
  Caps,
  ClientMessage,
  Config,
  DecodedFrame,
  DeviceInfo,
  DeviceNode,
  Status,
} from "./protocol.ts";
import { selectorFor } from "./selector.ts";
import { LatestSlot, RateGate, TextBatcher } from "./timing.ts";

export interface DeviceViewOptions {
  /** Hub URL, `ws://127.0.0.1:PORT/device/v1`. The token is appended by the component. */
  url: string;
  /** Session token. Only used to build the WebSocket URL; never logged or shown. */
  token: string;
  onSelect?(node: DeviceNode): void;
  onStatus?(s: Status): void;
  onHello?(info: { device: DeviceInfo; screen: Size & { dpr: number }; caps: Caps }): void;
  /** Test hook: replaces the global WebSocket constructor. */
  webSocket?: WebSocketCtor;
}

export interface DeviceViewHandle {
  destroy(): void;
  setConfig(c: Config): void;
  send(msg: ClientMessage): void;
}

const HOVER_INTERVAL_MS = 100;
const SWIPE_MIN_MS = 100;
const SWIPE_MAX_MS = 2000;
const LONG_PRESS_MAX_MS = 5000;
const TEXT_BATCH_MS = 50;
const PENDING_HIT_LIMIT = 32;

interface Gesture {
  pointerId: number;
  startClientX: number;
  startClientY: number;
  startT: number;
  start: MappedPoint;
  last: MappedPoint;
  maxMovePx: number;
  select: boolean;
}

type HitPurpose = "hover" | "select";

export function mountDeviceView(container: HTMLElement, opts: DeviceViewOptions): DeviceViewHandle {
  const doc = container.ownerDocument;
  const win = doc.defaultView ?? window;

  // ---- DOM -----------------------------------------------------------
  const root = doc.createElement("div");
  root.style.position = "relative";
  root.style.width = "100%";
  root.style.height = "100%";
  root.style.minHeight = "200px";
  root.style.overflow = "hidden";
  root.style.background = "#111";

  const canvas = doc.createElement("canvas");
  canvas.tabIndex = 0;
  canvas.setAttribute("role", "application");
  canvas.setAttribute(
    "aria-label",
    "Device screen. Click to tap, drag to swipe, hold to long press, Alt and click to select an element, type to send text.",
  );
  canvas.style.display = "block";
  canvas.style.width = "100%";
  canvas.style.height = "100%";
  canvas.style.touchAction = "none";
  canvas.style.cursor = "crosshair";
  canvas.style.outline = "none";
  canvas.style.outlineOffset = "-3px";

  const statusEl = doc.createElement("div");
  statusEl.setAttribute("role", "status");
  statusEl.setAttribute("aria-live", "polite");
  Object.assign(statusEl.style, {
    position: "absolute",
    left: "8px",
    top: "8px",
    padding: "2px 8px",
    borderRadius: "10px",
    background: "rgba(0,0,0,0.65)",
    color: "#fff",
    font: "12px system-ui, sans-serif",
    pointerEvents: "none",
  } satisfies Partial<CSSStyleDeclaration>);

  const chip = doc.createElement("div");
  Object.assign(chip.style, {
    position: "absolute",
    left: "8px",
    right: "8px",
    bottom: "8px",
    display: "none",
    alignItems: "center",
    gap: "8px",
    padding: "4px 8px",
    borderRadius: "6px",
    background: "rgba(0,0,0,0.8)",
    color: "#fff",
    font: "13px ui-monospace, monospace",
  } satisfies Partial<CSSStyleDeclaration>);
  const chipText = doc.createElement("span");
  chipText.setAttribute("aria-label", "Selected element selector");
  chipText.style.flex = "1";
  chipText.style.overflow = "hidden";
  chipText.style.textOverflow = "ellipsis";
  chipText.style.whiteSpace = "nowrap";
  const copyBtn = doc.createElement("button");
  copyBtn.type = "button";
  copyBtn.textContent = "Copy";
  copyBtn.setAttribute("aria-label", "Copy selector");
  const closeBtn = doc.createElement("button");
  closeBtn.type = "button";
  closeBtn.textContent = "Clear";
  closeBtn.setAttribute("aria-label", "Clear selection");
  chip.append(chipText, copyBtn, closeBtn);

  root.append(canvas, statusEl, chip);
  container.appendChild(root);

  const ctx = canvas.getContext("2d");

  // ---- State ---------------------------------------------------------
  let screen: Size & { dpr: number } = { w: 0, h: 0, dpr: 1 };
  let caps: Caps = { input: false, hit: false, bounds: false, jpeg: false };
  let tree: DeviceNode[] = [];
  let bitmap: ImageBitmap | null = null;
  let hover: DeviceNode | null = null;
  let selected: DeviceNode | null = null;
  let gesture: Gesture | null = null;
  let nextHitId = 1;
  const pendingHits = new Map<number, HitPurpose>();
  let latestHoverId = 0;
  let hoverTimer: unknown = null;
  let hoverPoint: MappedPoint | null = null;
  let decoding = false;
  let destroyed = false;
  let rafPending = false;
  let docVisible = !doc.hidden;
  let inView = true;
  let lastStatus: Status = { state: "connecting", msg: "connecting" };
  const slot = new LatestSlot<DecodedFrame>();
  const hoverGate = new RateGate(HOVER_INTERVAL_MS, () => performance.now());

  function setStatusText(s: Status): void {
    lastStatus = s;
    // Only the state word is shown; the hub's free-text message is shown for errors.
    statusEl.textContent = s.state === "connected" ? "" : s.msg || s.state;
    statusEl.style.display = statusEl.textContent === "" ? "none" : "block";
    try {
      opts.onStatus?.(s);
    } catch {
      // host callback errors must not break the view
    }
  }

  // ---- Connection ----------------------------------------------------
  const conn = new DeviceConnection({
    url: opts.url,
    token: opts.token,
    WebSocketImpl: opts.webSocket,
    handlers: {
      onStatus: setStatusText,
      onHello(m) {
        screen = { ...m.screen };
        caps = m.caps;
        try {
          opts.onHello?.({ device: m.device, screen: { ...screen }, caps });
        } catch {
          // ignore host errors
        }
        scheduleDraw();
      },
      onFrame(f) {
        slot.offer(f);
        void decodeLoop();
      },
      onTree(nodes) {
        tree = nodes;
      },
      onHit(id, node) {
        const purpose = pendingHits.get(id);
        if (purpose === undefined) return;
        pendingHits.delete(id);
        if (purpose === "hover") {
          if (id !== latestHoverId) return;
          hover = node;
          scheduleDraw();
        } else {
          chooseSelection(node);
        }
      },
      onAck() {
        // input acknowledgements need no UI
      },
    },
  });

  // ---- Rendering -----------------------------------------------------
  function canvasSize(): Size {
    return { w: canvas.width, h: canvas.height };
  }

  async function decodeLoop(): Promise<void> {
    if (decoding) return;
    decoding = true;
    try {
      for (;;) {
        const frame = slot.take();
        if (!frame || destroyed) return;
        let bm: ImageBitmap;
        try {
          bm = await createImageBitmap(new Blob([frame.image as BlobPart],{ type: `image/${frame.fmt}` }));
        } catch {
          continue; // undecodable frame: skip it
        }
        if (destroyed) {
          bm.close();
          return;
        }
        // A newer frame arrived while decoding: drop this one, decode the newer.
        const newer = slotHasPending();
        if (newer) {
          bm.close();
          continue;
        }
        adoptFrameOrientation(frame);
        bitmap?.close();
        bitmap = bm;
        scheduleDraw();
      }
    } finally {
      decoding = false;
    }
  }

  function slotHasPending(): boolean {
    // LatestSlot has no peek; take and re-offer keeps the API small.
    const f = slot.take();
    if (!f) return false;
    slot.offer(f);
    return true;
  }

  function adoptFrameOrientation(f: DecodedFrame): void {
    if (screen.w <= 0 || screen.h <= 0) {
      screen = { w: f.w, h: f.h, dpr: screen.dpr };
    } else if (f.w > f.h !== screen.w > screen.h && f.w !== f.h) {
      // Rotation: the hub did not send a new hello, follow the frame.
      screen = { w: screen.h, h: screen.w, dpr: screen.dpr };
    }
  }

  function scheduleDraw(): void {
    if (rafPending || destroyed) return;
    rafPending = true;
    win.requestAnimationFrame(() => {
      rafPending = false;
      draw();
    });
  }

  function resizeBacking(): boolean {
    const dpr = win.devicePixelRatio || 1;
    const s = backingSize(canvas.clientWidth || root.clientWidth, canvas.clientHeight || root.clientHeight, dpr);
    if (canvas.width === s.w && canvas.height === s.h) return false;
    canvas.width = s.w;
    canvas.height = s.h;
    return true;
  }

  function strokeNode(n: DeviceNode, color: string, fill: string): void {
    if (!ctx) return;
    const r = deviceRectToCanvas({ x: n.x, y: n.y, w: n.w, h: n.h }, canvasSize(), screen);
    const lw = Math.max(1, Math.round(2 * (win.devicePixelRatio || 1)));
    ctx.fillStyle = fill;
    ctx.fillRect(r.x, r.y, r.w, r.h);
    ctx.lineWidth = lw;
    ctx.strokeStyle = color;
    ctx.strokeRect(r.x + lw / 2, r.y + lw / 2, Math.max(0, r.w - lw), Math.max(0, r.h - lw));
  }

  function draw(): void {
    if (!ctx || destroyed) return;
    resizeBacking();
    ctx.fillStyle = "#111";
    ctx.fillRect(0, 0, canvas.width, canvas.height);
    if (bitmap && screen.w > 0) {
      const lb: Rect = letterbox(canvas.width, canvas.height, screen.w, screen.h);
      ctx.drawImage(bitmap, lb.x, lb.y, lb.w, lb.h);
    }
    if (selected) strokeNode(selected, "#ff9800", "rgba(255,152,0,0.15)");
    if (hover && hover !== selected) strokeNode(hover, "#2196f3", "rgba(33,150,243,0.15)");
  }

  // ---- Pointer mapping ----------------------------------------------
  function toDevice(ev: { clientX: number; clientY: number }): MappedPoint | null {
    resizeBacking();
    const b = canvas.getBoundingClientRect();
    return mapPointer({
      clientX: ev.clientX,
      clientY: ev.clientY,
      rect: { x: b.left, y: b.top, w: b.width, h: b.height },
      canvas: canvasSize(),
      screen,
    });
  }

  function requestHit(p: MappedPoint, purpose: HitPurpose): void {
    if (caps.hit) {
      if (pendingHits.size >= PENDING_HIT_LIMIT) {
        const oldest = pendingHits.keys().next();
        if (!oldest.done) pendingHits.delete(oldest.value);
      }
      const id = nextHitId++;
      if (purpose === "hover") latestHoverId = id;
      if (conn.send({ t: "hit", id, x: p.x, y: p.y })) {
        pendingHits.set(id, purpose);
        return;
      }
    }
    // Fallback: local hit test against the last tree, when the hub sends one.
    const local = hitTestNodes(tree, p.x, p.y);
    if (purpose === "hover") {
      hover = local;
      scheduleDraw();
    } else {
      chooseSelection(local);
    }
  }

  function chooseSelection(node: DeviceNode | null): void {
    selected = node;
    if (node) {
      chipText.textContent = selectorFor(node);
      chip.style.display = "flex";
      try {
        opts.onSelect?.(node);
      } catch {
        // ignore host errors
      }
    } else {
      chip.style.display = "none";
    }
    scheduleDraw();
  }

  function sendHover(): void {
    hoverTimer = null;
    if (!hoverPoint || gesture) return;
    if (!hoverGate.tryAcquire()) return;
    const p = hoverPoint;
    hoverPoint = null;
    requestHit(p, "hover");
  }

  function queueHover(p: MappedPoint): void {
    hoverPoint = p;
    if (!caps.hit && tree.length === 0) return;
    if (hoverGate.tryAcquire()) {
      hoverPoint = null;
      requestHit(p, "hover");
      return;
    }
    // Throttled: make sure the final position is still sent (trailing call).
    if (hoverTimer === null) {
      hoverTimer = win.setTimeout(sendHover, HOVER_INTERVAL_MS);
    }
  }

  // ---- Pointer events ------------------------------------------------
  function onPointerDown(ev: PointerEvent): void {
    if (ev.button !== 0 || gesture) return;
    canvas.focus({ preventScroll: true });
    const p = toDevice(ev);
    if (!p || !p.inside) return;
    ev.preventDefault();
    try {
      canvas.setPointerCapture(ev.pointerId);
    } catch {
      // not capturable (synthetic event): continue without capture
    }
    gesture = {
      pointerId: ev.pointerId,
      startClientX: ev.clientX,
      startClientY: ev.clientY,
      startT: performance.now(),
      start: p,
      last: p,
      maxMovePx: 0,
      select: ev.altKey,
    };
  }

  function onPointerMove(ev: PointerEvent): void {
    const p = toDevice(ev);
    if (!p) return;
    if (gesture && ev.pointerId === gesture.pointerId) {
      gesture.last = p;
      const d = Math.hypot(ev.clientX - gesture.startClientX, ev.clientY - gesture.startClientY);
      if (d > gesture.maxMovePx) gesture.maxMovePx = d;
      return;
    }
    if (!gesture && p.inside) queueHover(p);
    else if (!p.inside && hover) {
      hover = null;
      scheduleDraw();
    }
  }

  function onPointerUp(ev: PointerEvent): void {
    const g = gesture;
    if (!g || ev.pointerId !== g.pointerId) return;
    gesture = null;
    try {
      canvas.releasePointerCapture(ev.pointerId);
    } catch {
      // already released
    }
    const end = toDevice(ev) ?? g.last;
    const durationMs = performance.now() - g.startT;
    const d = Math.hypot(ev.clientX - g.startClientX, ev.clientY - g.startClientY);
    const maxMovePx = Math.max(g.maxMovePx, d);
    if (g.select) {
      if (maxMovePx <= TAP_MAX_MOVE_PX) requestHit(g.start, "select");
      return;
    }
    if (!caps.input) return;
    const kind = classifyGesture({ maxMovePx, durationMs });
    if (kind === "tap") {
      conn.send({ t: "tap", x: g.start.x, y: g.start.y });
    } else if (kind === "long_press") {
      conn.send({
        t: "long_press",
        x: g.start.x,
        y: g.start.y,
        ms: Math.round(clamp(durationMs, 500, LONG_PRESS_MAX_MS)),
      });
    } else {
      conn.send({
        t: "swipe",
        x1: g.start.x,
        y1: g.start.y,
        x2: end.x,
        y2: end.y,
        ms: Math.round(clamp(durationMs, SWIPE_MIN_MS, SWIPE_MAX_MS)),
      });
    }
  }

  function onPointerCancel(ev: PointerEvent): void {
    if (gesture && ev.pointerId === gesture.pointerId) gesture = null;
  }

  function onPointerLeave(): void {
    hoverPoint = null;
    if (hoverTimer !== null) {
      win.clearTimeout(hoverTimer as number);
      hoverTimer = null;
    }
    if (hover) {
      hover = null;
      scheduleDraw();
    }
  }

  // ---- Keyboard ------------------------------------------------------
  const batcher = new TextBatcher(
    (s) => {
      for (const chunk of chunkText(s)) conn.send({ t: "text", s: chunk });
    },
    TEXT_BATCH_MS,
  );

  function onKeyDown(ev: KeyboardEvent): void {
    if (ev.key === "Escape" && selected) {
      chooseSelection(null);
      return;
    }
    if (!caps.input || ev.ctrlKey || ev.metaKey || ev.altKey || ev.isComposing) return;
    // Printable: exactly one code point.
    if ([...ev.key].length === 1) {
      ev.preventDefault();
      batcher.push(ev.key);
    }
  }

  function onPaste(ev: ClipboardEvent): void {
    if (!caps.input) return;
    const text = ev.clipboardData?.getData("text/plain");
    if (text) {
      ev.preventDefault();
      batcher.push(text);
    }
  }

  function onFocus(): void {
    canvas.style.outline = "2px solid #2196f3";
  }
  function onBlur(): void {
    canvas.style.outline = "none";
    batcher.flush();
  }

  // ---- Chip buttons --------------------------------------------------
  function onCopy(): void {
    const text = chipText.textContent ?? "";
    try {
      void win.navigator.clipboard?.writeText(text).catch(() => undefined);
    } catch {
      // clipboard not available in this context
    }
  }
  function onClear(): void {
    chooseSelection(null);
  }

  // ---- Visibility ----------------------------------------------------
  function updatePaused(): void {
    conn.setPaused(!(docVisible && inView));
  }
  function onVisibility(): void {
    docVisible = !doc.hidden;
    updatePaused();
  }
  let io: IntersectionObserver | null = null;
  if (typeof win.IntersectionObserver === "function") {
    io = new win.IntersectionObserver((entries) => {
      const last = entries[entries.length - 1];
      if (last) {
        inView = last.isIntersecting;
        updatePaused();
      }
    });
    io.observe(root);
  }
  let ro: ResizeObserver | null = null;
  if (typeof win.ResizeObserver === "function") {
    ro = new win.ResizeObserver(() => scheduleDraw());
    ro.observe(root);
  }

  // ---- Wire up -------------------------------------------------------
  canvas.addEventListener("pointerdown", onPointerDown);
  canvas.addEventListener("pointermove", onPointerMove);
  canvas.addEventListener("pointerup", onPointerUp);
  canvas.addEventListener("pointercancel", onPointerCancel);
  canvas.addEventListener("pointerleave", onPointerLeave);
  canvas.addEventListener("keydown", onKeyDown);
  canvas.addEventListener("paste", onPaste);
  canvas.addEventListener("focus", onFocus);
  canvas.addEventListener("blur", onBlur);
  copyBtn.addEventListener("click", onCopy);
  closeBtn.addEventListener("click", onClear);
  doc.addEventListener("visibilitychange", onVisibility);
  win.addEventListener("resize", scheduleDraw);

  setStatusText(lastStatus);
  conn.start();
  if (!docVisible) updatePaused();
  scheduleDraw();

  return {
    destroy(): void {
      if (destroyed) return;
      destroyed = true;
      batcher.cancel();
      if (hoverTimer !== null) win.clearTimeout(hoverTimer as number);
      canvas.removeEventListener("pointerdown", onPointerDown);
      canvas.removeEventListener("pointermove", onPointerMove);
      canvas.removeEventListener("pointerup", onPointerUp);
      canvas.removeEventListener("pointercancel", onPointerCancel);
      canvas.removeEventListener("pointerleave", onPointerLeave);
      canvas.removeEventListener("keydown", onKeyDown);
      canvas.removeEventListener("paste", onPaste);
      canvas.removeEventListener("focus", onFocus);
      canvas.removeEventListener("blur", onBlur);
      copyBtn.removeEventListener("click", onCopy);
      closeBtn.removeEventListener("click", onClear);
      doc.removeEventListener("visibilitychange", onVisibility);
      win.removeEventListener("resize", scheduleDraw);
      io?.disconnect();
      ro?.disconnect();
      slot.clear();
      bitmap?.close();
      bitmap = null;
      conn.destroy();
      root.remove();
    },
    setConfig(c: Config): void {
      conn.setConfig(c);
    },
    send(msg: ClientMessage): void {
      conn.send(msg);
    },
  };
}
