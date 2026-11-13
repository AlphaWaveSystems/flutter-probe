import "./style.css";
import * as monaco from "monaco-editor";
import { chatLabelRefresher, labelText, type ChatLabelParts } from "./chat-label";
import { PROBESCRIPT_LANGUAGE_ID, registerProbeScript } from "./probescript";
import { initDeviceStream, onElementSelected, startStream, stopStream } from "./device-stream";
import { initDeviceSettings } from "./device-settings";
import { planTapInsert } from "./device-view/insertStep";
import { ExplainSession, explainCardModel, previewNote, type ExplainPreview, type ExplainResult } from "./explain";
import { initResults, type RunPlan, type RunStep } from "./results";
import { runScopeArgs, type RunMode } from "./run-scope";
import { toast } from "./toast";

import {
  CancelRun,
  Chat,
  Connect,
  ConnectWiFi,
  ConnectWiFiSaved,
  DeleteAPIKey,
  Disconnect,
  ForgetWiFiToken,
  GetAPIKey,
  InitialWorkspace,
  SetWorkspace,
  Lint,
  ListAIModels,
  ListDevices,
  ListDir,
  LoadAISettings,
  LoadWorkspaceSettings,
  PickWorkspace,
  PreviewAIChat,
  AIChatLabelParts,
  ExplainAvailability,
  ExplainPreviewed,
  PreviewExplain,
  FindLocalModels,
  ReadFile,
  RunFileScoped,
  SaveAISettings,
  SaveWiFiToken,
  SaveWorkspaceSettings,
  SetAPIKey,
  StartRecording,
  StartWiFiDiscovery,
  Status,
  StopRecording,
  StopWiFiDiscovery,
  TestAIConnection,
  TestWiFi,
  WiFiTokenSaved,
  WriteFile,
} from "../wailsjs/go/main/App";
import { main } from "../wailsjs/go/models";
type ChatMessage = { role: string; content: string };
type ChatResponse = { content: string; inputTokens: number; outputTokens: number; costUSD: number; notice?: string };
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
  glyphMargin: true,
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
// The workspace root: the folder given to SetWorkspace. The Go side reads its probe.yaml for the chat settings, so the
// settings overlay and the chat line read the same file. currentDir moves when the file pane is browsed; this does not.
let workspaceRoot = "";
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
      // Expose rows as buttons so assistive tech (and the Studio E2E suite,
      // which drives the real accessibility tree) can find and activate them.
      li.setAttribute("role", "button");
      li.tabIndex = 0;
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
  // Studio lists only inside the open workspace, so the up button stops at its root.
  if (workspaceRoot && !(parent === workspaceRoot || parent.startsWith(`${workspaceRoot}/`))) return;
  refreshFiles(parent);
});

$("btn-files-refresh").addEventListener("click", () => refreshFiles(currentDir));

async function pickWorkspace() {
  try {
    const path = await PickWorkspace();
    if (!path) return;
    localStorage.setItem(WORKSPACE_KEY, path);
    await SetWorkspace(path);
    resetAIPreview();
    workspaceRoot = path;
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

  if (!workspaceRoot) {
    noWorkspace.hidden = false;
    form.hidden = true;
    return;
  }
  noWorkspace.hidden = true;
  form.hidden = false;

  try {
    const s = await LoadWorkspaceSettings(workspaceRoot);
    ($("settings-port") as HTMLInputElement).value = s.agentPort ? String(s.agentPort) : "";
    ($("settings-timeout") as HTMLInputElement).value = s.defaultsTimeout ?? "";
    ($("settings-ios") as HTMLInputElement).value = s.iosDeviceId ?? "";
    ($("settings-android") as HTMLInputElement).value = s.androidDeviceId ?? "";
    ($("settings-ai-run-results") as HTMLInputElement).checked = s.aiContextRunResults === true;
    ($("settings-ai-widget-tree") as HTMLInputElement).checked = s.aiContextWidgetTree === true;
    fillAIForm(await LoadAISettings(workspaceRoot));
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

// ---- AI provider (Settings) and the first-send preview -----------------

// aiPreviewConfirmed is the "Sent to:" line of the last preview the person confirmed with Send. A message whose preview names a
// different destination is shown again, and so is the first one after the workspace, the provider settings or the saved key
// change. The gate lives in the window: Chat and SuggestSelectorFix send without it, also through the headless bridge.
let aiPreviewConfirmed: string | null = null;

function resetAIPreview(): void {
  aiPreviewConfirmed = null;
  explainSession.reset(); // the same events change where an Explain goes, so its preview is shown again
  void refreshExplainOffer();
  // The same events change where a message goes, so they change the header too, but only while the chat pane is open: the header is
  // not visible otherwise, and asking for it may read the keychain, which should not happen at launch for a pane nobody opened.
  if (!$("chat-pane").hidden) void refreshChatLabel();
}

// refreshChatLabel shows where the next message would go: the provider and the model id, or that AI is off. The text comes from
// probe.yaml, so it is set with textContent, never as markup. The latest call wins: two refreshes that finish out of order cannot
// leave an older label on screen.
// The model is its own element, so a long id is shortened with an ellipsis while the place after it stays in view. The whole line is
// the title, set as an attribute, never as markup.
function showChatLabel(parts: ChatLabelParts | null): void {
  const el = $("chat-provider");
  el.replaceChildren();
  el.removeAttribute("title");
  if (parts === null) return; // a refresh that failed leaves the header blank
  el.append(document.createTextNode(parts.prefix));
  if (parts.model !== "") {
    const model = document.createElement("span");
    model.className = "chat-provider-model";
    model.textContent = parts.model;
    el.append(model);
  }
  if (parts.suffix !== "") {
    const place = document.createElement("span");
    place.className = "chat-provider-place";
    place.textContent = parts.suffix;
    el.append(place);
  }
  el.setAttribute("title", labelText(parts));
}

const refreshChatLabel = chatLabelRefresher(() => AIChatLabelParts(), showChatLabel);

// previewLabel is the destination line PreviewAIChat puts first, such as "Sent to: local endpoint http://127.0.0.1:11434/v1, model m".
function previewLabel(preview: string): string {
  return preview.split("\n", 1)[0];
}

// plainKeyNoteText says what to do about a plain key in probe.yaml. A saved keychain key goes only to Anthropic, so for OpenAI
// and a local endpoint the way out is a variable, or no key at all.
function plainKeyNoteText(provider: string): string {
  const base = "probe.yaml holds a plain key, which Studio does not use and does not change. ";
  if (provider === "openai" || provider === "local") {
    return base + "Name an environment variable here, or delete that line: Studio sends a saved keychain key only to Anthropic.";
  }
  return base + "Save the key in the keychain and delete that line, or name an environment variable here.";
}

let aiPlainKeyInFile = false;

function refreshPlainKeyNote(): void {
  const note = $("settings-ai-plain-key");
  note.hidden = !aiPlainKeyInFile;
  note.textContent = plainKeyNoteText(($("settings-ai-provider") as HTMLSelectElement).value);
}

$("settings-ai-provider").addEventListener("change", refreshPlainKeyNote);

// The key is never in the form. probe.yaml names only the environment variable, and the keychain holds a saved key.
function fillAIForm(s: main.AISettings): void {
  ($("settings-ai-provider") as HTMLSelectElement).value = s.provider ?? "";
  ($("settings-ai-endpoint") as HTMLInputElement).value = s.endpoint ?? "";
  ($("settings-ai-model") as HTMLInputElement).value = s.model ?? "";
  // A plain key already in probe.yaml is its own source, offered only while the file holds one, so a form that was loaded from
  // such a file can be saved unchanged.
  const hasPlainKey = s.keySource === "file";
  ($("settings-ai-key-file") as HTMLOptionElement).hidden = !hasPlainKey;
  ($("settings-ai-key-source") as HTMLSelectElement).value = s.keySource === "env" ? "env" : hasPlainKey ? "file" : "keychain";
  ($("settings-ai-key-env") as HTMLInputElement).value = s.keyEnvName ?? "";
  ($("settings-ai-timeout") as HTMLInputElement).value = s.timeout ?? "";
  ($("settings-ai-vision") as HTMLInputElement).checked = s.vision !== false;
  ($("settings-ai-remote") as HTMLInputElement).checked = s.allowRemoteEndpoint === true;
  aiPlainKeyInFile = s.plainKeyInFile === true;
  refreshPlainKeyNote();
  $("settings-ai-error").hidden = true;
  $("settings-ai-models").replaceChildren();
  $("settings-ai-find-status").textContent = "";
  $("settings-ai-result").replaceChildren();
}

function readAIForm(): main.AISettings {
  return main.AISettings.createFrom({
    provider: ($("settings-ai-provider") as HTMLSelectElement).value,
    endpoint: ($("settings-ai-endpoint") as HTMLInputElement).value.trim(),
    model: ($("settings-ai-model") as HTMLInputElement).value.trim(),
    keySource: ($("settings-ai-key-source") as HTMLSelectElement).value,
    keyEnvName: ($("settings-ai-key-env") as HTMLInputElement).value.trim(),
    timeout: ($("settings-ai-timeout") as HTMLInputElement).value.trim(),
    vision: ($("settings-ai-vision") as HTMLInputElement).checked,
    allowRemoteEndpoint: ($("settings-ai-remote") as HTMLInputElement).checked,
    plainKeyInFile: false,
  });
}

function aiResultItem(text: string, kind: "ok" | "fail" | "dim"): HTMLLIElement {
  const li = document.createElement("li");
  li.className = kind;
  li.textContent = text;
  return li;
}

// The model list and the connection test use the saved settings, so the form is saved first.
$("btn-ai-list-models").addEventListener("click", async () => {
  try {
    const ids = (await ListAIModels()) ?? [];
    $("settings-ai-models").replaceChildren(
      ...ids.map((id) => {
        const option = document.createElement("option");
        option.value = id;
        return option;
      }),
    );
    if (ids.length === 0) {
      toast("This provider lists no models here. Type the model name.", "info", 4000);
    } else {
      toast(`${ids.length} models listed.`, "success", 2200);
    }
  } catch (err) {
    toast(`Cannot list models: ${err}`, "error");
  }
});

// Find local models asks this machine's own model server, on this click only. It fills only empty fields and says what it filled.
// It never saves, so nothing reaches probe.yaml until Save. The model is filled only when exactly one is listed; with several, the
// list is offered as suggestions and the person picks. Everything from the server is set as option values and text, never markup.
$("btn-ai-find-models").addEventListener("click", async () => {
  const status = $("settings-ai-find-status");
  const provider = $("settings-ai-provider") as HTMLSelectElement;
  const endpoint = $("settings-ai-endpoint") as HTMLInputElement;
  const model = $("settings-ai-model") as HTMLInputElement;
  status.textContent = "Looking on this machine…";
  try {
    const found = await FindLocalModels(endpoint.value.trim());
    const models = found.models ?? [];
    $("settings-ai-models").replaceChildren(
      ...models.map((id) => {
        const option = document.createElement("option");
        option.value = id;
        return option;
      }),
    );
    const filled: string[] = [];
    if (provider.value === "") {
      provider.value = "local";
      filled.push("provider");
      refreshPlainKeyNote();
    }
    if (provider.value === "local") {
      if (endpoint.value.trim() === "") {
        endpoint.value = found.endpoint;
        filled.push("endpoint");
      }
      if (model.value.trim() === "" && models.length === 1) {
        model.value = models[0];
        filled.push("model");
      }
    }
    const parts = [`Found ${models.length} model${models.length === 1 ? "" : "s"} at ${found.host}.`];
    parts.push(filled.length > 0 ? `Filled: ${filled.join(", ")}.` : "Filled nothing: the fields already have a value.");
    if (models.length === 0) parts.push("The server has no models yet.");
    else if (models.length > 1 && provider.value === "local" && model.value.trim() === "") parts.push("Pick one in the Model field.");
    if (found.skipped) parts.push(`${found.skipped}.`);
    parts.push("Nothing is saved until you press Save.");
    status.textContent = parts.join(" ");
  } catch (err) {
    status.textContent = `${err}`;
  }
});

$("btn-ai-test").addEventListener("click", async () => {
  const out = $("settings-ai-result");
  out.replaceChildren(aiResultItem("Testing the saved settings…", "dim"));
  try {
    const rep = await TestAIConnection();
    const rows = rep.checks.map((c) => aiResultItem(`${c.ok ? "✓" : "✗"} ${c.name}: ${c.detail}`, c.ok ? "ok" : "fail"));
    let summary = "The provider is not usable for text features.";
    if (rep.ok) {
      summary = !rep.visionChecked || rep.vision
        ? "Text works."
        : "Text works. The model does not accept images, so visual assertions stay off.";
    }
    out.replaceChildren(...rows, aiResultItem(summary, rep.ok ? "ok" : "fail"));
  } catch (err) {
    out.replaceChildren(aiResultItem(`Cannot test: ${err}`, "fail"));
  }
});

// ---- Explain: advice on the failed tests of the last run ----------------

const explainSession = new ExplainSession();

// refreshExplainOffer shows the Explain button only while AI is on and the last run has a failed test with error text. The latest
// answer wins, and a failure to ask hides the button.
let explainOfferSeq = 0;
async function refreshExplainOffer(): Promise<void> {
  const seq = ++explainOfferSeq;
  let available = false;
  try {
    available = (await ExplainAvailability()).available === true;
  } catch {
    available = false;
  }
  if (seq !== explainOfferSeq) return;
  $("explain-bar").hidden = !available;
  if (!available) hideExplainCard();
}

function hideExplainCard(): void {
  const card = $("explain-card");
  card.hidden = true;
  card.replaceChildren();
}

function textEl(tag: string, className: string, text: string): HTMLElement {
  const el = document.createElement(tag);
  el.className = className;
  el.textContent = text; // text only: advice and test names come from a model and a run, never markup
  return el;
}

// showExplainCard writes the advisory card. It sits beside the results and never touches them: the pass and fail counts and the rows
// stay as the run left them.
function showExplainCard(r: ExplainResult): void {
  const model = explainCardModel(r);
  const card = $("explain-card");
  card.replaceChildren();
  const head = document.createElement("div");
  head.className = "explain-card-head";
  const close = textEl("button", "btn-mini explain-card-close", "✕");
  close.setAttribute("type", "button");
  close.setAttribute("title", "Close");
  close.addEventListener("click", hideExplainCard);
  head.append(textEl("span", "explain-card-title", model.title), textEl("span", "explain-card-dest", model.destination), textEl("span", `explain-place explain-place-${model.place}`, model.place), close);
  card.append(head, textEl("p", "explain-disclaimer", model.disclaimer));
  for (const row of model.rows) {
    const div = document.createElement("div");
    div.className = `explain-row explain-row-${row.kind}`;
    if (row.label) div.append(textEl("div", "explain-row-label", row.label));
    div.append(textEl("p", "explain-row-text", row.text));
    card.append(div);
  }
  card.hidden = false;
}

// confirmExplainSend shows what Explain would send and waits for Send or Cancel. Nothing is sent before Send.
function confirmExplainSend(p: ExplainPreview): Promise<boolean> {
  const overlay = $("explain-preview-overlay");
  const send = $("btn-explain-preview-send");
  const cancel = $("btn-explain-preview-cancel");
  $("explain-preview-note").textContent = previewNote(p.hosted);
  $("explain-preview-text").textContent = p.text;
  overlay.hidden = false;
  return new Promise((resolve) => {
    const finish = (ok: boolean) => {
      overlay.hidden = true;
      send.removeEventListener("click", onSend);
      cancel.removeEventListener("click", onCancel);
      resolve(ok);
    };
    const onSend = () => finish(true);
    const onCancel = () => finish(false);
    send.addEventListener("click", onSend);
    cancel.addEventListener("click", onCancel);
  });
}

$("btn-explain").addEventListener("click", async () => {
  const btn = $("btn-explain") as HTMLButtonElement;
  const status = $("explain-status");
  btn.disabled = true;
  status.textContent = "";
  try {
    // The preview comes first and the request only after Send; the request is given the destination the person saw.
    const out = await explainSession.run({
      preview: () => PreviewExplain(),
      confirm: confirmExplainSend,
      explain: (destination) => {
        status.textContent = "Asking the model…";
        return ExplainPreviewed(destination);
      },
    });
    if (out?.kind === "result") showExplainCard(out.result);
  } catch (err) {
    status.textContent = `Explain failed: ${err}`;
    return;
  } finally {
    btn.disabled = false;
  }
  status.textContent = "";
});

EventsOn("run:started", () => hideExplainCard());
EventsOn("run:finished", () => void refreshExplainOffer());

// confirmFirstChatSend shows what the next chat message carries and waits for Send or Cancel. Nothing is sent before Send.
function confirmFirstChatSend(preview: string): Promise<boolean> {
  const overlay = $("ai-preview-overlay");
  const send = $("btn-ai-preview-send");
  const cancel = $("btn-ai-preview-cancel");
  $("ai-preview-text").textContent = preview;
  overlay.hidden = false;
  return new Promise((resolve) => {
    const finish = (ok: boolean) => {
      overlay.hidden = true;
      send.removeEventListener("click", onSend);
      cancel.removeEventListener("click", onCancel);
      resolve(ok);
    };
    const onSend = () => finish(true);
    const onCancel = () => finish(false);
    send.addEventListener("click", onSend);
    cancel.addEventListener("click", onCancel);
  });
}

$("btn-settings-save").addEventListener("click", async () => {
  if (!workspaceRoot) return;
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
    aiContextRunResults: ($("settings-ai-run-results") as HTMLInputElement).checked,
    aiContextWidgetTree: ($("settings-ai-widget-tree") as HTMLInputElement).checked,
  });
  // The AI form is saved first, because it is the one that can be refused. A refused AI form never stops the rest from saving:
  // its reason is shown on the AI section, and the other settings are written.
  const aiError = $("settings-ai-error");
  aiError.hidden = true;
  let aiFailure: unknown = null;
  try {
    await SaveAISettings(workspaceRoot, readAIForm());
  } catch (err) {
    aiFailure = err;
  }
  try {
    await SaveWorkspaceSettings(workspaceRoot, settings);
  } catch (err) {
    toast(`Save failed: ${err}`, "error");
    return;
  }
  resetAIPreview(); // changed provider settings are shown again before the next message
  void updateChatContextNote();
  if (aiFailure !== null) {
    aiError.textContent = `The AI provider section was not saved: ${aiFailure}`;
    aiError.hidden = false;
    toast(`Saved probe.yaml, except the AI provider section: ${aiFailure}`, "error", 6000);
    return; // the form stays open on the section that needs a change
  }
  toast(`Saved probe.yaml in ${workspaceRoot}`, "success", 2200);
  closeSettingsOverlay();
});

// ---- WiFi discovery overlay --------------------------------------------

type WiFiDevice = { name: string; host: string; port: number; version: string };

// Earlier versions kept WiFi tokens in plain text in localStorage. Those copies are removed on start; tokens now live in
// the OS keychain, one item per device.
try {
  localStorage.removeItem("fp.studio.wifi-tokens");
} catch {
  /* storage unavailable: nothing to remove */
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
  ($("wifi-manual-token") as HTMLInputElement).value = "";
  $("wifi-manual-msg").hidden = true;
  restoreManualWiFi();
  $("wifi-overlay").hidden = false;
  StartWiFiDiscovery().catch((err) => toast(`mDNS browse failed: ${err}`, "error"));
}

function closeWiFiOverlay() {
  $("wifi-overlay").hidden = true;
  StopWiFiDiscovery();
}

async function selectWiFiDevice(dev: WiFiDevice) {
  wifiSelected = dev;
  for (const li of Array.from($("wifi-devices").children) as HTMLElement[]) {
    li.classList.toggle("selected", li.dataset.key === `${dev.host}:${dev.port}`);
  }
  ($("wifi-token-row") as HTMLElement).hidden = false;
  const input = $("wifi-token-input") as HTMLInputElement;
  input.value = "";
  // A saved token is used when the field is left blank. The window never receives the saved token itself.
  const saved = await savedWiFiToken(dev.name);
  if (wifiSelected !== dev) return;
  $("wifi-selected-label").textContent =
    `Connecting to ${dev.name} (${dev.host}:${dev.port})` +
    (saved ? " · a saved token is used if the field is blank" : "");
  input.focus();
}

// Whether a WiFi token is saved for the device. A keychain that cannot be read counts as none saved.
async function savedWiFiToken(device: string): Promise<boolean> {
  try {
    return await WiFiTokenSaved(device);
  } catch {
    return false;
  }
}

// Marks a device that has a saved token, with the ✕ control that forgets it.
async function markSavedWiFi(li: HTMLElement, dev: WiFiDevice) {
  if (!(await savedWiFiToken(dev.name))) return;
  const tag = document.createElement("span");
  tag.className = "wifi-saved-tag";
  tag.textContent = "🔑 saved";
  tag.title = "A token is saved for this device";
  const forget = document.createElement("button");
  forget.className = "btn-mini wifi-forget";
  forget.title = `Forget the saved token for ${dev.name}`;
  forget.textContent = "✕";
  forget.addEventListener("click", async (e) => {
    e.stopPropagation();
    try {
      await ForgetWiFiToken(dev.name);
      tag.remove();
      forget.remove();
      toast(`Forgot the token for ${dev.name}`, "info", 1500);
    } catch (err) {
      toast(`Could not forget the token: ${err}`, "error");
    }
  });
  li.append(tag, forget);
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
  void markSavedWiFi(li, dev);
  li.addEventListener("click", () => selectWiFiDevice(dev));
  list.appendChild(li);
});

$("btn-wifi").addEventListener("click", openWiFiOverlay);
$("btn-wifi-close").addEventListener("click", closeWiFiOverlay);
$("wifi-overlay").addEventListener("click", (e) => {
  if ((e.target as HTMLElement).id === "wifi-overlay") closeWiFiOverlay();
});

// ---- Manual WiFi entry ---------------------------------------------------
// For a phone that does not advertise itself: host, port and token typed in. The Go side validates and
// explains what to check; the overlay only shows what it says.

const WIFI_LAST_KEY = "fp.studio.wifi-last"; // host and port only: the token is never stored

function manualWiFiValues(): { host: string; port: number; token: string } {
  return {
    host: ($("wifi-manual-host") as HTMLInputElement).value.trim(),
    port: Number(($("wifi-manual-port") as HTMLInputElement).value) || 0,
    token: ($("wifi-manual-token") as HTMLInputElement).value.trim(),
  };
}

function showManualMsg(text: string, kind: "ok" | "err") {
  const el = $("wifi-manual-msg");
  el.textContent = text;
  el.className = `wifi-manual-msg ${kind}`;
  el.hidden = false;
}

function restoreManualWiFi() {
  try {
    const last = JSON.parse(localStorage.getItem(WIFI_LAST_KEY) || "null");
    if (last && typeof last.host === "string") {
      ($("wifi-manual-host") as HTMLInputElement).value = last.host;
      ($("wifi-manual-port") as HTMLInputElement).value = String(last.port || 48686);
    }
  } catch {
    /* storage unavailable: the fields stay empty */
  }
}

async function manualWiFi(connect: boolean) {
  const { host, port, token } = manualWiFiValues();
  const btns = [$("btn-wifi-manual-test"), $("btn-wifi-manual-connect")] as HTMLButtonElement[];
  btns.forEach((b) => (b.disabled = true));
  showManualMsg(connect ? "Connecting…" : "Testing…", "ok");
  try {
    if (connect) {
      setStatus("connecting", `Connecting to ${host}…`);
      await ConnectWiFi(host, port, token);
      try {
        localStorage.setItem(WIFI_LAST_KEY, JSON.stringify({ host, port }));
      } catch {
        /* storage unavailable */
      }
      setStatus("connected", `Connected · ${host}`, `${host}:${port} · WiFi transport`);
      closeWiFiOverlay();
      startStream();
    } else {
      showManualMsg(await TestWiFi(host, port, token), "ok");
    }
  } catch (err) {
    const detail = enrichError(err);
    if (connect) setStatus("error", "WiFi connect failed", detail);
    showManualMsg(detail, "err");
  } finally {
    btns.forEach((b) => (b.disabled = false));
  }
}

$("btn-wifi-manual-test").addEventListener("click", () => manualWiFi(false));
$("btn-wifi-manual-connect").addEventListener("click", () => manualWiFi(true));

$("btn-wifi-connect").addEventListener("click", async () => {
  const dev = wifiSelected;
  if (!dev) return;
  const typed = ($("wifi-token-input") as HTMLInputElement).value.trim();
  const useSaved = !typed && (await savedWiFiToken(dev.name));
  if (!typed && !useSaved) {
    toast("Token is required.", "error");
    return;
  }
  setStatus("connecting", `Connecting to ${dev.host}…`);
  try {
    if (useSaved) {
      await ConnectWiFiSaved(dev.host, dev.port, dev.name);
    } else {
      await ConnectWiFi(dev.host, dev.port, typed);
      // Keep the typed token only once it has connected. A keychain that refuses the save does not undo the connection.
      try {
        await SaveWiFiToken(dev.name, typed);
      } catch {
        toast("Connected, but the token could not be saved to the keychain.", "info", 6000);
      }
    }
    setStatus(
      "connected",
      `Connected · ${dev.name}`,
      `${dev.name} (${dev.host}:${dev.port}) · WiFi transport`
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
  transport?: string;
  model?: string;
  hint?: string;
};

let lastDeviceList: DeviceInfo[] = [];

let refreshSeq = 0;

async function refreshDevices() {
  const select = $("device-picker") as HTMLSelectElement;
  // Two refreshes can overlap (the one at start-up and a click on ↻): only the newest one may fill
  // the list, and the list is rebuilt in one synchronous step, so no device is ever listed twice.
  const seq = ++refreshSeq;
  try {
    const devs = ((await ListDevices()) ?? []) as DeviceInfo[];
    if (seq !== refreshSeq) return;
    // Read the selection when the list has arrived, not when the refresh started: the user may have
    // picked a device while it was in flight, and that pick must survive the rebuild below.
    const previous = select.value;
    clearChildren(select);
    select.appendChild(makeOption("", "— select device —"));
    lastDeviceList = devs;
    // Show booted devices first; non-booted are still listed so users see
    // their full simulator inventory, but the Connect button stays disabled.
    const sorted = [...devs].sort(
      (a, b) => Number(b.booted) - Number(a.booted) || a.name.localeCompare(b.name)
    );
    for (const d of sorted) {
      const tag = d.kind === "physical" ? "physical" : d.platform;
      const model = d.model && !d.name.includes(d.model) ? ` (${d.model})` : "";
      const via = d.transport === "network" ? "  · WiFi only" : d.transport === "usb" ? "  · USB" : "";
      const label = d.booted
        ? `${d.name}${model}  ·  ${tag}${via}`
        : d.kind === "physical"
          ? `${d.name}${model}  ·  ${tag}${via}`
          : `${d.name}  ·  ${tag}  · shutdown`;
      const opt = makeOption(d.id, label);
      if (!d.booted) opt.dataset.booted = "false";
      select.appendChild(opt);
    }
    if (previous && devs.some((d) => d.id === previous)) {
      select.value = previous;
    }
    if (devs.length === 0) {
      toast("No simulators, emulators or phones detected. Start one, or plug in a phone, and click ↻. A phone on WiFi: use the 📡 button.", "info");
    } else if (devs.every((d) => !d.booted)) {
      const wifiOnly = devs.some((d) => d.transport === "network");
      toast(
        wifiOnly
          ? "A phone is paired over the network only: use the 📡 button, then Connect manually."
          : "All devices are shut down. Boot a simulator/emulator first.",
        "info"
      );
    }
  } catch (err) {
    if (seq !== refreshSeq) return;
    clearChildren(select);
    select.appendChild(makeOption("", "— select device —"));
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

// The line under the toolbar says what the selected physical device needs, so a phone never just fails or is missing.
function updateDeviceHint() {
  const select = $("device-picker") as HTMLSelectElement;
  const el = $("device-hint");
  const dev = lastDeviceList.find((d) => d.id === select.value);
  const text = dev?.hint ?? "";
  el.textContent = text;
  el.hidden = text === "";
}

function updateConnectButton() {
  updateDeviceHint();
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
    const dev = lastDeviceList.find((d) => d.id === select.value);
    btn.title = dev?.hint || "Boot the simulator/emulator first (e.g. via Xcode or Android Studio)";
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

// The scope control next to Run picks what runs: the whole file (the default), the test at the cursor, the tests with a
// tag, or the tests that failed last time. The tag field shows only for the tag scope.
function runScopeMode(): RunMode {
  return ($("run-scope") as HTMLSelectElement).value as RunMode;
}

function runTagField(): HTMLInputElement {
  return $("run-tag") as HTMLInputElement;
}

$("run-scope").addEventListener("change", () => {
  const tagging = runScopeMode() === "tag";
  runTagField().hidden = !tagging;
  if (tagging) runTagField().focus();
});

$("btn-run").addEventListener("click", async () => {
  if (!currentPath || !connected) return;
  const scope = runScopeArgs(runScopeMode(), editor.getPosition()?.lineNumber, runTagField().value);
  if (!scope.ok) {
    toast(scope.message, "info", 4000);
    return;
  }
  if (dirty) {
    await saveCurrent();
  }
  const btn = $("btn-run") as HTMLButtonElement;
  btn.disabled = true;
  setRunning(true);
  try {
    await RunFileScoped(currentPath, scope.args.line, scope.args.tag, scope.args.failedOnly);
  } catch (err) {
    toast(`Run failed: ${err}`, "error", 6000);
  } finally {
    setRunning(false);
    btn.disabled = !connected || !currentPath;
  }
});

$("btn-cancel").addEventListener("click", () => {
  ($("btn-cancel") as HTMLButtonElement).disabled = true;
  CancelRun();
});

// ---- Live step highlight ---------------------------------------------------
//
// `run:step` events carry the line of the step that is executing. The editor
// shows a gutter glyph plus a line highlight that follows the runner; the
// verdict (✓ / ✗ / –) stays on the line until the next run. The toolbar shows
// "step N of M" from the `run:plan` step counts.

const stepDecorations = editor.createDecorationsCollection();
const stepState = new Map<number, { status: RunStep["status"]; error?: string }>();
let runFilePath = "";
let stepsDone = 0;
let stepsTotal = 0;

function setRunning(active: boolean) {
  const cancel = $("btn-cancel") as HTMLButtonElement;
  cancel.hidden = !active;
  cancel.disabled = !active;
  const progress = $("run-progress");
  progress.hidden = !active;
  if (!active) progress.textContent = "";
}

function renderStepDecorations() {
  const decs: monaco.editor.IModelDeltaDecoration[] = [];
  for (const [line, st] of stepState) {
    const running = st.status === "started";
    decs.push({
      range: new monaco.Range(line, 1, line, 1),
      options: {
        isWholeLine: true,
        className: running ? "step-line-running" : st.status === "failed" ? "step-line-failed" : undefined,
        glyphMarginClassName: `step-glyph step-glyph-${running ? "running" : st.status}`,
        glyphMarginHoverMessage: st.error ? { value: st.error } : undefined,
        hoverMessage: st.error ? { value: st.error } : undefined,
        stickiness: monaco.editor.TrackedRangeStickiness.NeverGrowsWhenTypingAtEdges,
      },
    });
  }
  stepDecorations.set(decs);
}

function updateProgress() {
  const progress = $("run-progress");
  if (stepsTotal > 0) progress.textContent = `step ${Math.min(stepsDone + 1, stepsTotal)} of ${stepsTotal}`;
  else progress.textContent = `step ${stepsDone + 1}`;
}

EventsOn("run:started", (path: string) => {
  runFilePath = path;
  stepState.clear();
  stepDecorations.clear();
  stepsDone = 0;
  stepsTotal = 0;
});

EventsOn("run:plan", (plan: RunPlan) => {
  stepsTotal = (plan.tests ?? []).reduce((n, t) => n + t.steps, 0);
  updateProgress();
});

EventsOn("run:step", (s: RunStep) => {
  // Only decorate the file that is open in the editor.
  if (s.file !== runFilePath || s.file !== currentPath || s.line <= 0) return;
  if (s.status === "started") {
    stepState.set(s.line, { status: "started" });
    editor.revealLineInCenterIfOutsideViewport(s.line);
  } else {
    stepState.set(s.line, { status: s.status, error: s.error });
    if (s.depth === 0) {
      stepsDone++;
      updateProgress();
    }
  }
  renderStepDecorations();
});

document.addEventListener("studio:goto-line", (ev) => {
  const line = (ev as CustomEvent<number>).detail;
  if (!line) return;
  editor.revealLineInCenter(line);
  editor.setPosition({ lineNumber: line, column: 1 });
  editor.focus();
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

// "+ tap": add a `tap` step for the element last selected in the device pane (Alt+click) at the editor cursor.
let selectedSelector: string | null = null;
const insertTapBtn = $("btn-insert-tap") as HTMLButtonElement;
onElementSelected((selector) => {
  selectedSelector = selector;
  insertTapBtn.disabled = selector === null;
});
insertTapBtn.addEventListener("click", () => {
  if (selectedSelector === null) return;
  const pos = editor.getPosition() ?? { lineNumber: model.getLineCount(), column: 1 };
  const plan = planTapInsert(model.getLineContent(pos.lineNumber), selectedSelector);
  if (!plan) {
    toast("That element cannot be written as a tap step.", "error", 3000);
    return;
  }
  const col = plan.column + 1;
  editor.executeEdits("device-view", [{ range: new monaco.Range(pos.lineNumber, col, pos.lineNumber, col), text: plan.text }]);
  editor.focus();
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

initDeviceStream();
initDeviceSettings();

// PROBE_STUDIO_WORKSPACE (env) wins over the remembered workspace so
// automation can open a known folder without the native picker.
(async () => {
  let ws = "";
  try {
    ws = (await InitialWorkspace()) || "";
  } catch {
    ws = "";
  }
  if (!ws) ws = localStorage.getItem(WORKSPACE_KEY) ?? "";
  if (ws) {
    localStorage.setItem(WORKSPACE_KEY, ws);
    await SetWorkspace(ws);
    resetAIPreview();
    workspaceRoot = ws;
    refreshFiles(ws);
  } else {
    refreshFiles();
  }
})();

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
  if (visible) {
    void updateChatContextNote();
    void refreshChatLabel(); // the header is filled when the pane opens, so a hidden pane never asks
  }
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

// contextNoteText is the line shown above the chat input while the workspace switches send more than the open file.
function contextNoteText(runResults: boolean, widgetTree: boolean): string {
  if (runResults && widgetTree) return "Sending the last run and the screen to the AI provider";
  if (runResults) return "Sending the last run to the AI provider";
  if (widgetTree) return "Sending the screen to the AI provider";
  return "";
}

// updateChatContextNote shows that line when the workspace settings send the last run or the screen. It reads the same
// probe.yaml switches the Go side reads, so the line shows exactly when those parts go to the provider. If the settings
// cannot be read, the line stays hidden and the Go side sends nothing beyond the open file.
async function updateChatContextNote(): Promise<void> {
  const note = $("chat-context-note");
  let text = "";
  if (workspaceRoot) {
    try {
      const s = await LoadWorkspaceSettings(workspaceRoot);
      text = contextNoteText(s.aiContextRunResults === true, s.aiContextWidgetTree === true);
    } catch {
      text = "";
    }
  }
  note.textContent = text;
  note.hidden = text === "";
}

// chatSending is true while a message is waiting for its preview or for the provider. A second Enter or click in that time
// is ignored, so one Send cannot answer two messages.
let chatSending = false;

async function sendChat() {
  if (chatSending) return;
  chatSending = true;
  try {
    await sendChatTurn();
  } finally {
    chatSending = false;
  }
}

async function sendChatTurn() {
  const input = $("chat-input") as HTMLTextAreaElement;
  const text = input.value.trim();
  if (!text) return;

  // A message shows what it would send now whenever its destination is not the one the person last confirmed. Cancel keeps the
  // text in the input. The request builds its own turn when it runs, so a change in between, such as a new run result, can
  // make it differ from what was shown.
  let preview: string;
  try {
    preview = await PreviewAIChat([...chatHistory, { role: "user", content: text }], model.getValue());
  } catch (err) {
    appendChatMessage("system", `Error: ${err}`);
    return;
  }
  const label = previewLabel(preview);
  if (label !== aiPreviewConfirmed) {
    input.disabled = true;
    const send = await confirmFirstChatSend(preview);
    input.disabled = false;
    if (!send) {
      input.focus();
      return;
    }
    aiPreviewConfirmed = label;
  }

  input.value = "";
  input.disabled = true;
  ($("btn-chat-send") as HTMLButtonElement).disabled = true;

  chatHistory.push({ role: "user", content: text });
  appendChatMessage("user", text);

  await updateChatContextNote();
  const fileContent = model.getValue();

  try {
    const resp = (await Chat(chatHistory, fileContent)) as ChatResponse;
    if (resp.notice) appendChatMessage("system", resp.notice);
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
  // The stored key is never sent to the page: the field starts empty, and the placeholder says whether one is saved.
  const input = $("api-key-input") as HTMLInputElement;
  input.value = "";
  let saved = false;
  try {
    saved = await GetAPIKey();
  } catch {
    saved = false;
  }
  input.placeholder = saved ? "A key is saved. Type a new key to replace it." : "sk-ant-…";
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
  // An empty field must not clear a saved key: removing one is the separate Remove key action.
  if (!key) {
    toast("Type a key to save it, or use Remove key to delete the saved one.", "info");
    return;
  }
  try {
    await SetAPIKey(key);
    resetAIPreview();
    toast("API key saved to keychain.", "success");
    closeApiKeyOverlay();
  } catch (err) {
    toast(`Failed to save key: ${err}`, "error");
  }
});

$("btn-api-key-delete").addEventListener("click", async () => {
  try {
    await DeleteAPIKey();
    resetAIPreview();
    ($("api-key-input") as HTMLInputElement).value = "";
    toast("API key removed from keychain.", "success");
    closeApiKeyOverlay();
  } catch (err) {
    toast(`Failed to remove key: ${err}`, "error");
  }
});

// Keyboard shortcut ⌘⇧A to toggle chat
// (wired into the existing keydown handler via the 'a' case)
