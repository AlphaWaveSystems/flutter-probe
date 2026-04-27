// Device pane: subscribes to `device:frame` Wails events, renders the
// latest screenshot, and forwards bundled widget-tree updates to the
// inspector pane.
//
// This is a pure event-driven consumer — no polling, no client-side timers.
// The Go backend's streamLoop drives the cadence: each frame is the result
// of one parallel screenshot+tree RPC pair, so frame time ≈ max(rpc1, rpc2)
// rather than rpc1+rpc2.

import { EventsOn } from "../wailsjs/runtime/runtime";

type FrameEvent = {
  screenshot: string;   // base64 PNG, no data: prefix
  widgetTree: string;
  timestampMs: number;
  frameMs: number;      // wall time the backend spent capturing this frame
};

const FPS_WINDOW_MS = 1000;

let viewEl: HTMLElement | null = null;
let imgEl: HTMLImageElement | null = null;
let placeholderEl: HTMLElement | null = null;
let fpsLabel: HTMLElement | null = null;
let inspectorEl: HTMLElement | null = null;

let framesInWindow = 0;
let windowStartMs = 0;
let connected = false;

export function initDeviceStream(): void {
  viewEl = document.getElementById("device-view");
  fpsLabel = document.getElementById("device-fps");
  placeholderEl = viewEl?.querySelector(".placeholder") ?? null;
  inspectorEl = document.getElementById("inspector-content");

  EventsOn("device:frame", (frame: FrameEvent) => {
    if (!connected) return; // ignore stragglers after disconnect
    paint(frame.screenshot);
    updateInspector(frame.widgetTree);
    tickFps(frame.frameMs);
  });

  EventsOn("device:stream-error", (msg: string) => {
    if (fpsLabel) fpsLabel.textContent = "no signal";
    console.warn("stream error:", msg);
  });
}

export function startStream(): void {
  // The backend's streamLoop is already running because Connect started it.
  // We just flip the local "accept frames" flag and reset FPS counters so
  // disconnected stragglers don't leak into a fresh session.
  connected = true;
  framesInWindow = 0;
  windowStartMs = performance.now();
  if (fpsLabel) fpsLabel.textContent = "…";
}

export function stopStream(): void {
  connected = false;
  if (imgEl) {
    imgEl.remove();
    imgEl = null;
  }
  if (placeholderEl && viewEl && !viewEl.contains(placeholderEl)) {
    viewEl.appendChild(placeholderEl);
  }
  if (fpsLabel) fpsLabel.textContent = "";
  if (inspectorEl) {
    inspectorEl.classList.add("dim");
    inspectorEl.textContent = "Connect to load the widget tree.";
  }
}

function paint(b64: string): void {
  if (!viewEl) return;
  if (!imgEl) {
    imgEl = document.createElement("img");
    imgEl.alt = "Device";
    if (placeholderEl && viewEl.contains(placeholderEl)) {
      placeholderEl.remove();
    }
    viewEl.appendChild(imgEl);
  }
  imgEl.src = `data:image/png;base64,${b64}`;
}

function updateInspector(tree: string): void {
  if (!inspectorEl) return;
  if (!tree) return; // best-effort field — keep last-known tree on missing
  inspectorEl.classList.remove("dim");
  inspectorEl.textContent = tree;
}

function tickFps(frameMs: number): void {
  framesInWindow++;
  const now = performance.now();
  if (now - windowStartMs >= FPS_WINDOW_MS) {
    if (fpsLabel) {
      fpsLabel.textContent = `${framesInWindow} fps · ${frameMs}ms/frame`;
    }
    framesInWindow = 0;
    windowStartMs = now;
  }
}
