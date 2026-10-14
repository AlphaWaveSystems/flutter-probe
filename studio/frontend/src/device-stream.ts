// Device pane: subscribes to `device:frame` Wails events, renders the
// latest screenshot, and forwards bundled widget-tree updates to the
// inspector pane.
//
// This is a pure event-driven consumer — no polling, no client-side timers.
// The Go backend's streamLoop drives the cadence: each frame is the result
// of one parallel screenshot+tree RPC pair, so frame time ≈ max(rpc1, rpc2)
// rather than rpc1+rpc2.

import { EventsOn } from "../wailsjs/runtime/runtime";
import { GetDeviceView, GetWidgetTree } from "../wailsjs/go/main/App";
import { mountDeviceView, selectorFor } from "./device-view";
import { capsNote } from "./device-view/capsNote";
import type { DeviceViewHandle, DeviceNode } from "./device-view";
import { toast } from "./toast";

// Two frame sources. By default the backend's streamLoop pushes `device:frame` events (above). When Studio runs with
// PROBE_STUDIO_DEVICE_VIEW=1, GetDeviceView returns the address of a loopback hub and the pane mounts the shared
// device view component instead: frames, pointer input and hover/select all come over that WebSocket, and the
// inspector text is refreshed on a timer.
const TREE_POLL_MS = 1500;
let viewHandle: DeviceViewHandle | null = null;
let viewHost: HTMLElement | null = null;
let treeTimer: number | null = null;

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

/** Alt+click in the view: show the selector, copy it, and search the inspector for the element. */
// Text read from the device is untrusted: it never goes into the clipboard or a search box with control characters.
const CONTROL_CHARS = /[\u0000-\u001f\u007f-\u009f]/;

let capsText = { label: "", title: "" };
function setViewLabel(base: string): void {
  if (!fpsLabel) return;
  const note = base === "device view" && capsText.label ? ` · ${capsText.label}` : "";
  fpsLabel.textContent = base + note;
  fpsLabel.title = base === "device view" ? capsText.title : "";
}

function onSelect(node: DeviceNode): void {
  const selector = selectorFor(node);
  if (CONTROL_CHARS.test(selector)) {
    toast("That element's text contains control characters, so no selector was copied.", "error", 4000);
    return;
  }
  const needle = node.key ?? node.text ?? node.label ?? node.type;
  const search = document.getElementById("inspector-search") as HTMLInputElement | null;
  if (search && !CONTROL_CHARS.test(needle)) {
    search.value = needle;
    search.dispatchEvent(new Event("input"));
  }
  navigator.clipboard?.writeText(selector).catch(() => {
    /* clipboard unavailable: the toast still shows the selector */
  });
  toast(`Selected ${selector}`, "info", 2500);
}

async function pollTree(): Promise<void> {
  try {
    updateInspector(await GetWidgetTree());
  } catch {
    /* keep the last tree */
  }
}

async function mountSharedView(): Promise<boolean> {
  if (viewHandle) return true; // already mounted for this connection
  let info;
  try {
    info = await GetDeviceView();
  } catch (err) {
    console.warn("device view unavailable:", err);
    return false;
  }
  if (!info?.enabled || !info.url || !info.token || !viewEl || !connected) return false;
  placeholderEl?.remove();
  viewHost = document.createElement("div");
  viewHost.style.width = "100%";
  viewHost.style.height = "100%";
  viewEl.appendChild(viewHost);
  viewHandle = mountDeviceView(viewHost, {
    url: info.url,
    token: info.token,
    onSelect,
    onHello: (h) => {
      // The hub's capabilities for this device: say what is off so a read-only view is not mistaken for a broken one.
      capsText = capsNote(h.caps);
      setViewLabel("device view");
    },
    onStatus: (s) => {
      // "taken": another window took the view over. The component offers its own Reconnect button; Studio never
      // reconnects by itself.
      setViewLabel(s.state === "connected" ? "device view" : s.state === "taken" ? "taken over" : s.state);
    },
  });
  void pollTree();
  treeTimer = window.setInterval(() => void pollTree(), TREE_POLL_MS);
  return true;
}

function unmountSharedView(): void {
  capsText = { label: "", title: "" };
  if (treeTimer !== null) {
    window.clearInterval(treeTimer);
    treeTimer = null;
  }
  viewHandle?.destroy();
  viewHandle = null;
  viewHost?.remove();
  viewHost = null;
}

export function startStream(): void {
  // The backend's streamLoop is already running because Connect started it.
  // We just flip the local "accept frames" flag and reset FPS counters so
  // disconnected stragglers don't leak into a fresh session.
  connected = true;
  framesInWindow = 0;
  frameMsInWindow = [];
  windowStartMs = performance.now();
  if (fpsLabel) fpsLabel.textContent = "…";
  // When the shared device view is on this replaces the event-driven frames; otherwise it returns false at once.
  void mountSharedView();
}

export function stopStream(): void {
  connected = false;
  unmountSharedView();
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

// Shows frames per second and the capture time per frame (average and slowest) over the last second. frameMs is
// measured by the backend around one screenshot + widget-tree pair; it does not include the time the page needs to
// decode and paint the image.
let frameMsInWindow: number[] = [];

function tickFps(frameMs: number): void {
  framesInWindow++;
  frameMsInWindow.push(frameMs);
  const now = performance.now();
  if (now - windowStartMs >= FPS_WINDOW_MS) {
    if (fpsLabel) {
      const avg = Math.round(frameMsInWindow.reduce((a, b) => a + b, 0) / frameMsInWindow.length);
      const max = Math.max(...frameMsInWindow);
      fpsLabel.textContent = `${framesInWindow} fps · ${avg}ms avg · ${max}ms max`;
    }
    framesInWindow = 0;
    frameMsInWindow = [];
    windowStartMs = now;
  }
}
