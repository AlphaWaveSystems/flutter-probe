import "./style.css";
import * as monaco from "monaco-editor";
import { PROBESCRIPT_LANGUAGE_ID, registerProbeScript } from "./probescript";
import { initDeviceStream, startStream, stopStream } from "./device-stream";
import { initResults } from "./results";
import { toast } from "./toast";

import {
  Connect,
  ConnectWiFi,
  Disconnect,
  Lint,
  ListDevices,
  ListDir,
  PickWorkspace,
  ReadFile,
  RunFile,
  StartWiFiDiscovery,
  Status,
  StopWiFiDiscovery,
  WriteFile,
} from "../wailsjs/go/main/App";
import { EventsOn } from "../wailsjs/runtime/runtime";

const WORKSPACE_KEY = "fp.studio.workspace";

type Diagnostic = {
  severity: number;
  message: string;
  startLineNumber: number;
  startColumn: number;
  endLineNumber: number;
  endColumn: number;
};

type ConnectionStatus = {
  connected: boolean;
  deviceId: string;
  deviceName: string;
  platform: string;
};

// ---- DOM helpers ---------------------------------------------------------

function $(id: string): HTMLElement {
  const el = document.getElementById(id);
  if (!el) throw new Error(`missing element #${id}`);
  return el;
}

function clearChildren(el: Element) {
  while (el.firstChild) el.removeChild(el.firstChild);
}

function makeOption(value: string, label: string): HTMLOptionElement {
  const opt = document.createElement("option");
  opt.value = value;
  opt.textContent = label;
  return opt;
}

// ---- Editor --------------------------------------------------------------

registerProbeScript();

monaco.editor.defineTheme("probe-dark", {
  base: "vs-dark",
  inherit: true,
  rules: [
    { token: "type", foreground: "60a5fa", fontStyle: "bold" },
    { token: "keyword", foreground: "c4b5fd" },
    { token: "string", foreground: "86efac" },
    { token: "string.escape", foreground: "fbbf24" },
    { token: "variable", foreground: "fbbf24" },
    { token: "annotation", foreground: "f472b6" },
    { token: "comment", foreground: "64748b", fontStyle: "italic" },
    { token: "number", foreground: "fcd34d" },
  ],
  colors: {
    "editor.background": "#1b2636",
    "editor.lineHighlightBackground": "#23314a55",
    "editorLineNumber.foreground": "#475569",
    "editorLineNumber.activeForeground": "#94a3b8",
    "editorCursor.foreground": "#60a5fa",
    "editor.selectionBackground": "#3b82f644",
    "editorIndentGuide.background": "#23314a",
    "editorIndentGuide.activeBackground": "#334155",
  },
});

const editor = monaco.editor.create($("editor"), {
  value: [
    "# FlutterProbe Studio — preview",
    "# Open a .probe file from the left to start, or pick a device and click Run.",
    "",
    "test \"smoke\":",
    "  see \"Welcome\"",
    "",
  ].join("\n"),
  language: PROBESCRIPT_LANGUAGE_ID,
  theme: "probe-dark",
  fontSize: 13,
  fontFamily: "ui-monospace, SF Mono, Menlo, Monaco, monospace",
  minimap: { enabled: false },
  automaticLayout: true,
  scrollBeyondLastLine: false,
  renderWhitespace: "boundary",
  smoothScrolling: true,
  cursorBlinking: "smooth",
  padding: { top: 12 },
});

const model = editor.getModel()!;

let lintHandle: number | undefined;
function scheduleLint() {
  if (lintHandle !== undefined) clearTimeout(lintHandle);
  lintHandle = window.setTimeout(runLint, 150);
}

async function runLint() {
  const content = model.getValue();
  let diags: Diagnostic[] = [];
  try {
    diags = (await Lint(content)) ?? [];
  } catch (err) {
    console.error("lint failed", err);
  }
  monaco.editor.setModelMarkers(
    model,
    "probescript",
    diags.map((d) => ({
      severity: d.severity,
      message: d.message,
      startLineNumber: d.startLineNumber,
      startColumn: d.startColumn,
      endLineNumber: d.endLineNumber,
      endColumn: d.endColumn,
    }))
  );
}

editor.onDidChangeModelContent(() => {
  scheduleLint();
  setDirty(true);
});

// ---- File browser --------------------------------------------------------

let currentPath: string | null = null;
let currentDir = "tests";
let dirty = false;

function setDirty(d: boolean) {
  dirty = d;
  $("editor-dirty").textContent = d ? "● unsaved" : "";
  ($("btn-save") as HTMLButtonElement).disabled = !d || !currentPath;
}

async function refreshFiles(dir: string = currentDir) {
  currentDir = dir;
  $("file-path").textContent = dir;
  const list = $("file-list");
  clearChildren(list);
  try {
    const entries = await ListDir(dir);
    for (const e of entries) {
      const li = document.createElement("li");
      li.textContent = e.name;
      if (e.isDir) li.classList.add("dir");
      li.dataset.path = e.path;
      li.dataset.isDir = String(e.isDir);
      li.onclick = () => onFileClick(e.path, e.isDir);
      list.appendChild(li);
    }
    if (entries.length === 0) {
      const li = document.createElement("li");
      li.classList.add("empty");
      li.textContent = "(empty)";
      list.appendChild(li);
    }
  } catch (err) {
    console.error("ListDir failed", err);
    toast(`Cannot read ${dir}: ${err}`, "error");
  }
}

async function onFileClick(path: string, isDir: boolean) {
  if (isDir) {
    refreshFiles(path);
    return;
  }
  if (!path.endsWith(".probe")) return;
  if (dirty && !confirm("Discard unsaved changes?")) return;
  try {
    const content = await ReadFile(path);
    model.setValue(content);
    currentPath = path;
    $("editor-filename").textContent = path;
    setDirty(false);
    runLint();
    document
      .querySelectorAll("#file-list li")
      .forEach((li) =>
        li.classList.toggle(
          "active",
          (li as HTMLElement).dataset.path === path
        )
      );
    updateRunButton();
  } catch (err) {
    console.error("ReadFile failed", err);
    toast(`Cannot open ${path}: ${err}`, "error");
  }
}

$("btn-files-up").addEventListener("click", () => {
  if (currentDir === "." || currentDir === "/") return;
  const parent = currentDir.split("/").slice(0, -1).join("/") || ".";
  refreshFiles(parent);
});

$("btn-files-refresh").addEventListener("click", () => refreshFiles(currentDir));

async function pickWorkspace() {
  try {
    const path = await PickWorkspace();
    if (!path) return;
    localStorage.setItem(WORKSPACE_KEY, path);
    refreshFiles(path);
    toast(`Workspace: ${path}`, "success", 1800);
  } catch (err) {
    toast(`Open failed: ${err}`, "error");
  }
}

$("btn-pick-workspace").addEventListener("click", pickWorkspace);

// ---- Save ---------------------------------------------------------------

async function saveCurrent() {
  if (!currentPath) return;
  try {
    await WriteFile(currentPath, model.getValue());
    setDirty(false);
    toast(`Saved ${currentPath}`, "success", 1800);
  } catch (err) {
    console.error("WriteFile failed", err);
    toast(`Save failed: ${err}`, "error");
  }
}

$("btn-save").addEventListener("click", saveCurrent);

// Global keyboard shortcuts. Editor-local shortcuts (text editing) are
// handled by Monaco; the modifiers below are non-conflicting commands.
window.addEventListener("keydown", (e) => {
  const mod = e.metaKey || e.ctrlKey;

  if (e.key === "Escape") {
    closeHelpOverlay();
    return;
  }
  if (e.key === "?" && !mod) {
    // Avoid triggering when the editor has focus and the user types a literal `?`.
    if (document.activeElement?.closest("#editor")) return;
    e.preventDefault();
    openHelpOverlay();
    return;
  }
  if (!mod) return;
  switch (e.key.toLowerCase()) {
    case "s":
      e.preventDefault();
      saveCurrent();
      break;
    case "r":
      e.preventDefault();
      ($("btn-run") as HTMLButtonElement).click();
      break;
    case "b":
      e.preventDefault();
      ($("btn-connect") as HTMLButtonElement).click();
      break;
    case "p":
      e.preventDefault();
      pickWorkspace();
      break;
    case "k":
      e.preventDefault();
      refreshDevices();
      break;
  }
});

function openHelpOverlay() {
  $("help-overlay").hidden = false;
}
function closeHelpOverlay() {
  $("help-overlay").hidden = true;
}
$("btn-help-close").addEventListener("click", closeHelpOverlay);
$("help-overlay").addEventListener("click", (e) => {
  // Click outside the card dismisses.
  if ((e.target as HTMLElement).id === "help-overlay") closeHelpOverlay();
});

// ---- WiFi discovery overlay --------------------------------------------

type WiFiDevice = { name: string; host: string; port: number; version: string };

let wifiSelected: WiFiDevice | null = null;
const wifiSeen = new Map<string, WiFiDevice>();

function openWiFiOverlay() {
  wifiSelected = null;
  wifiSeen.clear();
  const list = $("wifi-devices");
  clearChildren(list);
  const empty = document.createElement("li");
  empty.className = "empty";
  empty.textContent = "Searching…";
  list.appendChild(empty);
  ($("wifi-token-row") as HTMLElement).hidden = true;
  ($("wifi-token-input") as HTMLInputElement).value = "";
  $("wifi-overlay").hidden = false;
  StartWiFiDiscovery().catch((err) => toast(`mDNS browse failed: ${err}`, "error"));
}

function closeWiFiOverlay() {
  $("wifi-overlay").hidden = true;
  StopWiFiDiscovery();
}

function selectWiFiDevice(dev: WiFiDevice) {
  wifiSelected = dev;
  for (const li of Array.from($("wifi-devices").children) as HTMLElement[]) {
    li.classList.toggle("selected", li.dataset.key === `${dev.host}:${dev.port}`);
  }
  $("wifi-selected-label").textContent = `Connecting to ${dev.name} (${dev.host}:${dev.port})`;
  ($("wifi-token-row") as HTMLElement).hidden = false;
  ($("wifi-token-input") as HTMLInputElement).focus();
}

EventsOn("wifi:device-found", (dev: WiFiDevice) => {
  const key = `${dev.host}:${dev.port}`;
  if (wifiSeen.has(key)) return;
  wifiSeen.set(key, dev);
  const list = $("wifi-devices");
  // Drop the "Searching…" placeholder once we have at least one entry.
  const placeholder = list.querySelector("li.empty");
  if (placeholder) placeholder.remove();
  const li = document.createElement("li");
  li.dataset.key = key;
  // Use DOM construction (not innerHTML) to keep agent-controlled strings
  // out of the HTML parser.
  const nameEl = document.createElement("strong");
  nameEl.textContent = dev.name;
  const hostEl = document.createElement("span");
  hostEl.className = "wifi-host";
  hostEl.textContent = `${dev.host}:${dev.port}${dev.version ? ` · v${dev.version}` : ""}`;
  li.append(nameEl, hostEl);
  li.addEventListener("click", () => selectWiFiDevice(dev));
  list.appendChild(li);
});

$("btn-wifi").addEventListener("click", openWiFiOverlay);
$("btn-wifi-close").addEventListener("click", closeWiFiOverlay);
$("wifi-overlay").addEventListener("click", (e) => {
  if ((e.target as HTMLElement).id === "wifi-overlay") closeWiFiOverlay();
});

$("btn-wifi-connect").addEventListener("click", async () => {
  if (!wifiSelected) return;
  const token = ($("wifi-token-input") as HTMLInputElement).value.trim();
  if (!token) {
    toast("Token is required.", "error");
    return;
  }
  setStatus("connecting", `Connecting to ${wifiSelected.host}…`);
  try {
    await ConnectWiFi(wifiSelected.host, wifiSelected.port, token);
    setStatus("connected", `Connected · ${wifiSelected.name}`);
    closeWiFiOverlay();
    startStream();
  } catch (err) {
    setStatus("error", `WiFi connect failed: ${err}`);
    toast(`Connect failed: ${err}`, "error");
  }
});

// ---- Connection ---------------------------------------------------------

let connected = false;

function setStatus(state: "disconnected" | "connecting" | "connected" | "error", text: string) {
  const dot = $("status-dot");
  dot.classList.remove("status-disconnected", "status-connecting", "status-connected", "status-error");
  dot.classList.add(`status-${state}`);
  $("status-text").textContent = text;
  connected = state === "connected";
  updateRunButton();
  const inspectorStatus = document.getElementById("inspector-status");
  if (inspectorStatus) {
    inspectorStatus.textContent = connected ? "live" : "";
  }
}

function updateRunButton() {
  ($("btn-run") as HTMLButtonElement).disabled = !connected || !currentPath;
}

type DeviceInfo = {
  id: string;
  name: string;
  platform: string;
  kind: string; // "simulator" | "emulator" | "physical"
  state: string;
  osVersion: string;
  booted: boolean;
};

let lastDeviceList: DeviceInfo[] = [];

async function refreshDevices() {
  const select = $("device-picker") as HTMLSelectElement;
  const previous = select.value;
  clearChildren(select);
  select.appendChild(makeOption("", "— select device —"));
  try {
    const devs = ((await ListDevices()) ?? []) as DeviceInfo[];
    lastDeviceList = devs;
    // Show booted devices first; non-booted are still listed so users see
    // their full simulator inventory, but the Connect button stays disabled.
    const sorted = [...devs].sort(
      (a, b) => Number(b.booted) - Number(a.booted) || a.name.localeCompare(b.name)
    );
    for (const d of sorted) {
      const tag = d.kind === "physical" ? "physical" : d.platform;
      const label = d.booted
        ? `${d.name}  ·  ${tag}`
        : `${d.name}  ·  ${tag}  · shutdown`;
      const opt = makeOption(d.id, label);
      if (!d.booted) opt.dataset.booted = "false";
      select.appendChild(opt);
    }
    if (previous && devs.some((d) => d.id === previous)) {
      select.value = previous;
    }
    if (devs.length === 0) {
      toast("No simulators or emulators detected. Start one and click ↻.", "info");
    } else if (devs.every((d) => !d.booted)) {
      toast("All devices are shut down. Boot a simulator/emulator first.", "info");
    }
  } catch (err) {
    console.error("ListDevices failed", err);
    toast(`Device discovery failed: ${err}`, "error");
  }
  updateConnectButton();
}

function selectedDeviceIsBooted(): boolean {
  const select = $("device-picker") as HTMLSelectElement;
  if (!select.value) return false;
  const dev = lastDeviceList.find((d) => d.id === select.value);
  return Boolean(dev?.booted);
}

function updateConnectButton() {
  const select = $("device-picker") as HTMLSelectElement;
  const btn = $("btn-connect") as HTMLButtonElement;
  if (connected) {
    btn.textContent = "Disconnect";
    btn.disabled = false;
    btn.title = "Disconnect from device";
    return;
  }
  btn.textContent = "Connect";
  if (!select.value) {
    btn.disabled = true;
    btn.title = "Pick a device first";
  } else if (!selectedDeviceIsBooted()) {
    btn.disabled = true;
    btn.title = "Boot the simulator/emulator first (e.g. via Xcode or Android Studio)";
  } else {
    btn.disabled = false;
    btn.title = "Connect to selected device";
  }
}

($("device-picker") as HTMLSelectElement).addEventListener("change", updateConnectButton);
$("btn-refresh").addEventListener("click", refreshDevices);

$("btn-connect").addEventListener("click", async () => {
  if (connected) {
    try {
      await Disconnect();
      stopStream();
      toast("Disconnected", "info", 1800);
    } catch (err) {
      toast(`Disconnect failed: ${err}`, "error");
    }
    return;
  }
  const select = $("device-picker") as HTMLSelectElement;
  const id = select.value;
  if (!id) return;
  setStatus("connecting", "connecting…");
  try {
    const status = (await Connect(id)) as ConnectionStatus;
    setStatus("connected", `${status.deviceName}  ·  ${status.platform}`);
    startStream();
    toast(`Connected to ${status.deviceName}`, "success");
  } catch (err) {
    setStatus("error", "connection failed");
    toast(`Connect failed: ${err}`, "error", 6000);
    setTimeout(() => setStatus("disconnected", "disconnected"), 2500);
  }
});

EventsOn("connection:changed", (status: ConnectionStatus) => {
  if (status.connected) {
    setStatus("connected", `${status.deviceName}  ·  ${status.platform}`);
  } else {
    stopStream();
    setStatus("disconnected", "disconnected");
  }
  updateConnectButton();
});

// ---- Inspector ----------------------------------------------------------
// The inspector pane is updated live by the device-stream module — every
// frame from the backend includes a widget tree dump. No manual refresh.

// ---- Run ----------------------------------------------------------------

initResults();

$("btn-run").addEventListener("click", async () => {
  if (!currentPath || !connected) return;
  if (dirty) {
    await saveCurrent();
  }
  const btn = $("btn-run") as HTMLButtonElement;
  btn.disabled = true;
  try {
    await RunFile(currentPath);
  } catch (err) {
    toast(`Run failed: ${err}`, "error", 6000);
  } finally {
    btn.disabled = !connected || !currentPath;
  }
});

// ---- Boot ---------------------------------------------------------------

// macOS traffic lights need ~78px of leading inset; other platforms don't.
// Wails uses standard chrome on Windows/Linux so reserve no space there.
const isMac = /Mac|iPhone|iPad/.test(navigator.userAgent);
if (!isMac) {
  document.documentElement.style.setProperty("--titlebar-inset", "0px");
}

initDeviceStream();

const savedWorkspace = localStorage.getItem(WORKSPACE_KEY);
if (savedWorkspace) {
  refreshFiles(savedWorkspace);
} else {
  refreshFiles();
}

refreshDevices();
runLint();

// Restore connection if app was relaunched while connected
(async () => {
  try {
    const status = (await Status()) as ConnectionStatus;
    if (status.connected) {
      setStatus("connected", `${status.deviceName}  ·  ${status.platform}`);
      startStream();
    } else {
      setStatus("disconnected", "disconnected");
    }
  } catch {
    setStatus("disconnected", "disconnected");
  }
})();
