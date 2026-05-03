import "./style.css";
import * as monaco from "monaco-editor";
import { PROBESCRIPT_LANGUAGE_ID, registerProbeScript } from "./probescript";
import { initDeviceStream, startStream, stopStream } from "./device-stream";
import { initResults } from "./results";
import { toast } from "./toast";

import {
  Chat,
  Connect,
  ConnectWiFi,
  DeleteAPIKey,
  Disconnect,
  GetAPIKey,
  Lint,
  ListDevices,
  ListDir,
  LoadWorkspaceSettings,
  PickWorkspace,
  ReadFile,
  RunFile,
  SaveWorkspaceSettings,
  SetAPIKey,
  StartRecording,
  StartWiFiDiscovery,
  Status,
  StopRecording,
  StopWiFiDiscovery,
  WriteFile,
} from "../wailsjs/go/main/App";
import { main } from "../wailsjs/go/models";
type ChatMessage = { role: string; content: string };
type ChatResponse = { content: string; inputTokens: number; outputTokens: number; costUSD: number };
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
    closeSettingsOverlay();
    closeWiFiOverlay();
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
      if (e.shiftKey) {
        ($("btn-record") as HTMLButtonElement).click();
      } else {
        ($("btn-run") as HTMLButtonElement).click();
      }
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
    case "a":
      if (e.shiftKey) {
        e.preventDefault();
        showChatPane(!chatVisible);
      }
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

// ---- Workspace settings overlay ----------------------------------------

async function openSettingsOverlay() {
  const overlay = $("settings-overlay");
  const noWorkspace = $("settings-no-workspace") as HTMLElement;
  const form = $("settings-form") as HTMLFormElement;
  overlay.hidden = false;

  if (!currentDir || currentDir === "tests" || currentDir === ".") {
    noWorkspace.hidden = false;
    form.hidden = true;
    return;
  }
  noWorkspace.hidden = true;
  form.hidden = false;

  try {
    const s = await LoadWorkspaceSettings(currentDir);
    ($("settings-port") as HTMLInputElement).value = s.agentPort ? String(s.agentPort) : "";
    ($("settings-timeout") as HTMLInputElement).value = s.defaultsTimeout ?? "";
    ($("settings-ios") as HTMLInputElement).value = s.iosDeviceId ?? "";
    ($("settings-android") as HTMLInputElement).value = s.androidDeviceId ?? "";
  } catch (err) {
    toast(`Cannot load settings: ${err}`, "error");
  }
}

function closeSettingsOverlay() {
  $("settings-overlay").hidden = true;
}

$("btn-settings").addEventListener("click", openSettingsOverlay);
$("btn-settings-close").addEventListener("click", closeSettingsOverlay);
$("settings-overlay").addEventListener("click", (e) => {
  if ((e.target as HTMLElement).id === "settings-overlay") closeSettingsOverlay();
});

$("btn-settings-save").addEventListener("click", async () => {
  if (!currentDir) return;
  const portRaw = ($("settings-port") as HTMLInputElement).value.trim();
  const port = portRaw === "" ? 0 : Number(portRaw);
  if (Number.isNaN(port)) {
    toast("Agent port must be a number.", "error");
    return;
  }
  const settings = main.WorkspaceSettings.createFrom({
    agentPort: port,
    defaultsTimeout: ($("settings-timeout") as HTMLInputElement).value.trim(),
    iosDeviceId: ($("settings-ios") as HTMLInputElement).value.trim(),
    androidDeviceId: ($("settings-android") as HTMLInputElement).value.trim(),
  });
  try {
    await SaveWorkspaceSettings(currentDir, settings);
    toast(`Saved probe.yaml in ${currentDir}`, "success", 2200);
    closeSettingsOverlay();
  } catch (err) {
    toast(`Save failed: ${err}`, "error");
  }
});

// ---- WiFi discovery overlay --------------------------------------------

type WiFiDevice = { name: string; host: string; port: number; version: string };

const WIFI_TOKENS_KEY = "fp.studio.wifi-tokens";

// Token store: deviceName → token. Persisted in localStorage so a user who
// has connected once doesn't need to paste the token again on subsequent
// launches. Keyed by the Bonjour instance name (typically the device's
// hostname) since the IP can change across DHCP leases but the name is
// stable. Tokens for an LAN-only test agent inside a desktop app are an
// acceptable trust boundary; this is not a credential vault.
function loadTokenStore(): Record<string, string> {
  try {
    const raw = localStorage.getItem(WIFI_TOKENS_KEY);
    return raw ? (JSON.parse(raw) as Record<string, string>) : {};
  } catch {
    return {};
  }
}

function saveTokenStore(store: Record<string, string>) {
  try {
    localStorage.setItem(WIFI_TOKENS_KEY, JSON.stringify(store));
  } catch {
    // localStorage full or denied — non-fatal, user just re-enters next time
  }
}

function rememberToken(deviceName: string, token: string) {
  const store = loadTokenStore();
  store[deviceName] = token;
  saveTokenStore(store);
}

function forgetToken(deviceName: string) {
  const store = loadTokenStore();
  delete store[deviceName];
  saveTokenStore(store);
}

function recalledToken(deviceName: string): string | undefined {
  return loadTokenStore()[deviceName];
}

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
  const remembered = recalledToken(dev.name);
  $("wifi-selected-label").textContent = `Connecting to ${dev.name} (${dev.host}:${dev.port})`;
  ($("wifi-token-row") as HTMLElement).hidden = false;
  const input = $("wifi-token-input") as HTMLInputElement;
  input.value = remembered ?? "";
  input.focus();
}

function renderForgetButton(deviceName: string, container: HTMLElement) {
  const btn = document.createElement("button");
  btn.className = "btn-mini wifi-forget";
  btn.title = `Forget remembered token for ${deviceName}`;
  btn.textContent = "✕";
  btn.addEventListener("click", (e) => {
    e.stopPropagation();
    forgetToken(deviceName);
    btn.remove();
    toast(`Forgot token for ${deviceName}`, "info", 1500);
  });
  container.appendChild(btn);
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
  if (recalledToken(dev.name)) {
    const savedTag = document.createElement("span");
    savedTag.className = "wifi-saved-tag";
    savedTag.textContent = "🔑 saved";
    savedTag.title = "Token remembered from a previous connection";
    li.appendChild(savedTag);
    renderForgetButton(dev.name, li);
  }
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
    rememberToken(wifiSelected.name, token);
    setStatus(
      "connected",
      `Connected · ${wifiSelected.name}`,
      `${wifiSelected.name} (${wifiSelected.host}:${wifiSelected.port}) · WiFi transport`
    );
    closeWiFiOverlay();
    startStream();
  } catch (err) {
    const detail = enrichError(err);
    setStatus("error", "WiFi connect failed", detail);
    toast(`Connect failed: ${detail}`, "error", 8000);
  }
});

// ---- Connection ---------------------------------------------------------

let connected = false;

function setStatus(
  state: "disconnected" | "connecting" | "connected" | "error",
  text: string,
  tooltip?: string
) {
  const dot = $("status-dot");
  dot.classList.remove("status-disconnected", "status-connecting", "status-connected", "status-error");
  dot.classList.add(`status-${state}`);
  $("status-text").textContent = text;
  // Mirror the human-readable text in title attributes so a hover gives
  // device id / transport / last error without taking up toolbar space.
  const tip = tooltip ?? text;
  dot.title = tip;
  $("status-text").title = tip;
  connected = state === "connected";
  updateRunButton();
  updateRecordButton();
  const inspectorStatus = document.getElementById("inspector-status");
  if (inspectorStatus) {
    inspectorStatus.textContent = connected ? "live" : "";
  }
}

// enrichError maps known error fragments to a one-line actionable hint.
// Connection failures often come from missing host tools (iproxy, adb) or
// the app not being built with the agent flag — surfacing the fix inline
// beats sending the user to a wiki page.
function enrichError(raw: unknown): string {
  const msg = String(raw);
  if (/iproxy/i.test(msg)) {
    return `${msg}\n\nFix: brew install libimobiledevice`;
  }
  if (/idevicesyslog/i.test(msg)) {
    return `${msg}\n\nFix: brew install libimobiledevice (idevicesyslog ships with it)`;
  }
  if (/adb/i.test(msg) && /not found|not in PATH/i.test(msg)) {
    return `${msg}\n\nFix: install Android SDK platform-tools and ensure adb is on your PATH`;
  }
  if (/token/i.test(msg) && /(not found|timeout)/i.test(msg)) {
    return `${msg}\n\nFix: confirm your Flutter app is running with --dart-define=PROBE_AGENT=true`;
  }
  if (/PROBE_AGENT/.test(msg)) {
    return `${msg}\n\nFix: rebuild your Flutter app with --dart-define=PROBE_AGENT=true`;
  }
  if (/refused|reset by peer|EOF/i.test(msg)) {
    return `${msg}\n\nFix: the agent process likely isn't running. Start your Flutter app and try again.`;
  }
  return msg;
}

function updateRunButton() {
  ($("btn-run") as HTMLButtonElement).disabled = !connected || !currentPath;
}

function updateRecordButton() {
  const btn = $("btn-record") as HTMLButtonElement;
  btn.disabled = !connected;
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
    setStatus(
      "connected",
      `${status.deviceName}  ·  ${status.platform}`,
      `${status.deviceName} (${status.deviceId}) · ${status.platform} · USB transport`
    );
    startStream();
    toast(`Connected to ${status.deviceName}`, "success");
  } catch (err) {
    const detail = enrichError(err);
    setStatus("error", "connection failed", detail);
    toast(`Connect failed: ${detail}`, "error", 8000);
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

// Search input scrolls the <pre> to the first line containing the query.
// Re-runs on every input AND every animation frame so live updates from
// the device stream don't push the matched line off-screen. Cheap O(n)
// over the visible widget tree text — acceptable for trees up to a few
// thousand lines, which is the practical limit anyway.
let inspectorQuery = "";

function scrollInspectorToMatch() {
  if (!inspectorQuery) return;
  const pre = document.getElementById("inspector-content");
  if (!pre) return;
  const text = pre.textContent ?? "";
  const idx = text.toLowerCase().indexOf(inspectorQuery.toLowerCase());
  if (idx < 0) return;
  // Approximate line number → scrollTop. The <pre> uses a monospace font
  // with consistent line height; this is precise enough to put the match
  // near the top of the visible area.
  const lineNumber = text.slice(0, idx).split("\n").length - 1;
  const lineHeight = parseFloat(getComputedStyle(pre).lineHeight) || 14;
  pre.scrollTop = Math.max(0, lineNumber * lineHeight - 8);
}

const inspectorSearch = document.getElementById("inspector-search") as HTMLInputElement | null;
if (inspectorSearch) {
  inspectorSearch.addEventListener("input", () => {
    inspectorQuery = inspectorSearch.value;
    scrollInspectorToMatch();
  });
  // Re-anchor on each frame the inspector content updates. The device-stream
  // module replaces innerHTML wholesale per frame; without a re-scroll the
  // matched line jumps back to the top.
  const inspectorEl = document.getElementById("inspector-content");
  if (inspectorEl) {
    new MutationObserver(() => scrollInspectorToMatch()).observe(inspectorEl, {
      childList: true,
      characterData: true,
      subtree: true,
    });
  }
}

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

// ---- Recorder -----------------------------------------------------------

let recording = false;

function setRecording(active: boolean) {
  recording = active;
  const btn = $("btn-record") as HTMLButtonElement;
  const icon = btn.querySelector(".btn-record-icon") as HTMLElement;
  const label = btn.querySelector(".btn-label") as HTMLElement;
  if (active) {
    icon.textContent = "■";
    label.textContent = "Stop";
    btn.classList.add("btn-recording");
    btn.title = "Stop recording (⌘⇧R)";
  } else {
    icon.textContent = "●";
    label.textContent = "Record";
    btn.classList.remove("btn-recording");
    btn.title = "Record interactions (⌘⇧R)";
  }
}

EventsOn("recorder:line", (line: string) => {
  if (!recording) return;
  const m = editor.getModel()!;
  const lastLine = m.getLineCount();
  const lastCol = m.getLineMaxColumn(lastLine);
  editor.executeEdits("recorder", [
    {
      range: new monaco.Range(lastLine, lastCol, lastLine, lastCol),
      text: "\n" + line,
    },
  ]);
  editor.revealLine(m.getLineCount());
});

$("btn-record").addEventListener("click", async () => {
  if (!connected) return;

  if (!recording) {
    const now = new Date().toLocaleString();
    const header = `# Recorded on ${now}\n\ntest "recorded flow"\n  open the app`;
    model.setValue(header);
    currentPath = null;
    $("editor-filename").textContent = "recorded.probe";
    setDirty(true);
    setRecording(true);
    try {
      await StartRecording();
    } catch (err) {
      setRecording(false);
      toast(`Recording failed: ${enrichError(err)}`, "error");
    }
  } else {
    try {
      const script = await StopRecording();
      if (script && script.trim()) {
        model.setValue(script);
      }
    } catch (err) {
      toast(`Stop failed: ${enrichError(err)}`, "error");
    } finally {
      setRecording(false);
    }
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

// ---- AI Chat ------------------------------------------------------------

let chatHistory: ChatMessage[] = [];
let chatTotalCost = 0;
let chatInputTokens = 0;
let chatOutputTokens = 0;
let chatVisible = false;

function showChatPane(visible: boolean) {
  chatVisible = visible;
  $("chat-pane").hidden = !visible;
  $("btn-ai-toggle").classList.toggle("btn-active", visible);
}

function updateCostDisplay() {
  const el = $("chat-cost");
  if (chatTotalCost === 0) {
    el.textContent = "";
    return;
  }
  const cost = chatTotalCost < 0.01
    ? `<$0.01`
    : `$${chatTotalCost.toFixed(4)}`;
  el.textContent = `${cost} · ${chatInputTokens.toLocaleString()} in / ${chatOutputTokens.toLocaleString()} out`;
}

function appendChatMessage(role: "user" | "assistant" | "system", text: string) {
  const container = $("chat-messages");
  const div = document.createElement("div");
  div.className = `chat-msg chat-msg-${role}`;
  const pre = document.createElement("pre");
  pre.className = "chat-msg-text";
  pre.textContent = text;
  div.appendChild(pre);
  container.appendChild(div);
  container.scrollTop = container.scrollHeight;
}

async function sendChat() {
  const input = $("chat-input") as HTMLTextAreaElement;
  const text = input.value.trim();
  if (!text) return;

  input.value = "";
  input.disabled = true;
  ($("btn-chat-send") as HTMLButtonElement).disabled = true;

  chatHistory.push({ role: "user", content: text });
  appendChatMessage("user", text);

  const fileContent = model.getValue();

  try {
    const resp = (await Chat(chatHistory, fileContent)) as ChatResponse;
    chatHistory.push({ role: "assistant", content: resp.content });
    appendChatMessage("assistant", resp.content);
    chatTotalCost += resp.costUSD;
    chatInputTokens += resp.inputTokens;
    chatOutputTokens += resp.outputTokens;
    updateCostDisplay();
  } catch (err) {
    chatHistory.pop(); // remove failed user msg from history
    appendChatMessage("system", `Error: ${err}`);
  } finally {
    input.disabled = false;
    ($("btn-chat-send") as HTMLButtonElement).disabled = false;
    input.focus();
  }
}

$("btn-ai-toggle").addEventListener("click", () => showChatPane(!chatVisible));

$("btn-chat-send").addEventListener("click", sendChat);

$("chat-input").addEventListener("keydown", (e) => {
  if (e.key === "Enter" && !e.shiftKey) {
    e.preventDefault();
    sendChat();
  }
});

$("btn-chat-clear").addEventListener("click", () => {
  chatHistory = [];
  chatTotalCost = 0;
  chatInputTokens = 0;
  chatOutputTokens = 0;
  $("chat-messages").innerHTML = "";
  updateCostDisplay();
});

$("btn-chat-key").addEventListener("click", openApiKeyOverlay);

// ---- API key overlay ----------------------------------------------------

async function openApiKeyOverlay() {
  try {
    const key = (await GetAPIKey()) as string;
    ($("api-key-input") as HTMLInputElement).value = key || "";
  } catch {
    ($("api-key-input") as HTMLInputElement).value = "";
  }
  $("api-key-overlay").hidden = false;
}

function closeApiKeyOverlay() {
  $("api-key-overlay").hidden = true;
}

$("btn-api-key-close").addEventListener("click", closeApiKeyOverlay);
$("api-key-overlay").addEventListener("click", (e) => {
  if (e.target === $("api-key-overlay")) closeApiKeyOverlay();
});

$("btn-api-key-save").addEventListener("click", async () => {
  const key = ($("api-key-input") as HTMLInputElement).value.trim();
  try {
    await SetAPIKey(key);
    toast(key ? "API key saved to keychain." : "API key cleared.", "success");
    closeApiKeyOverlay();
  } catch (err) {
    toast(`Failed to save key: ${err}`, "error");
  }
});

$("btn-api-key-delete").addEventListener("click", async () => {
  try {
    await DeleteAPIKey();
    ($("api-key-input") as HTMLInputElement).value = "";
    toast("API key removed from keychain.", "success");
    closeApiKeyOverlay();
  } catch (err) {
    toast(`Failed to remove key: ${err}`, "error");
  }
});

// Keyboard shortcut ⌘⇧A to toggle chat
// (wired into the existing keydown handler via the 'a' case)
