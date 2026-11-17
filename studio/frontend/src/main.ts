import "./style.css";
import * as monaco from "monaco-editor";
import { chatLabelRefresher, labelText, type ChatLabelParts } from "./chat-label";
import { PROBESCRIPT_LANGUAGE_ID, registerProbeScript } from "./probescript";
import { initDeviceStream, onElementSelected, startStream, stopStream } from "./device-stream";
import { initDeviceSettings } from "./device-settings";
import { planTapInsert } from "./device-view/insertStep";
import { latestWins } from "./latest-wins";
import { clampLine, placeLabel, planJump, type JumpTarget } from "./goto-line";
import { connectReason, nextStep, recordReason, runReason, saveReason, workspacePaneText, type FirstRunInput } from "./first-run";
import { ExplainSession, explainCardModel, previewNote, type ExplainPreview, type ExplainResult } from "./explain";
import { FixSession, REPLACE_NOTE, STALE_VERDICT_NOTE, canOfferReplace, fixCardModel, fixPreviewNote, enclosingTest, planReplace, rerunGate, rerunVerdict, againOffer, type FixAnswer, type FixPreview, type RerunTarget } from "./selector-fix";
import { initNetworkPanel, resetNetworkPanel } from "./network-panel";
import { renderProblems } from "./problems-panel";
import { initConfirmDialog } from "./confirm-dialog";
import { recordedScript, recordingHeader } from "./recording-script";
import { askDiscard, initDiscardDialog } from "./discard-dialog";
import { askSaveAsName, initSaveAsDialog } from "./save-as-dialog";
import { connectFailureLine } from "./connect-failure";
import { progressText, signalNote, waitingLine } from "./still-waiting";
import { deviceHasNoSignal } from "./device-stream";
import { LintSession, toMarkers, type Problem } from "./lint-problems";
import { initSendPanel } from "./send-panel";
import { buildWiFiItem, type DocLike } from "./wifi-label";
import { placeStep } from "./send-insert";
import { initResults, type RunPlan, type RunStep } from "./results";
import { runScopeArgs, type RunMode } from "./run-scope";
import { toast } from "./toast";

import {
  CancelRun,
  CheckReplaceLine,
  CloseWorkspace,
  SelectorFixAttempts,
  Chat,
  Connect,
  ConnectWiFi,
  ConnectWiFiSaved,
  DeleteAPIKey,
  Disconnect,
  GetStudioPrefs,
  SetClearDataWithoutAsking,
  CanExportRunResults,
  ExportRunResults,
  ForgetWiFiToken,
  GetAPIKey,
  InitialWorkspace,
  SetWorkspace,
  Lint,
  LintProblems,
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
  PreviewSelectorFix,
  SelectorFixPreviewed,
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
  WorkspaceNote,
  DeviceListNote,
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
  appBuild?: string; // what the app reports about its build ("App built with Flutter 3.44.8 (stable), Dart 3.12.2"); absent when it reports nothing
  toolsNote?: string; // the connection uses a program that probe.yaml names ("Using adb from probe.yaml: adb")
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
    "# This is a scratch buffer, not a file. Open a .probe file in the Workspace pane to edit and run your tests.",
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

// The project's lint rules (recipes, data files, program rules) for the open file, from the editor's text. It never blocks Run: a failure is a
// line in the Problems panel. The call waits 400 ms after the last change, and an answer for text that has changed since is dropped.
const problemsSession = new LintSession();
let problemsHandle: number | undefined;
function scheduleProblems() {
  if (problemsHandle !== undefined) clearTimeout(problemsHandle);
  problemsSession.cancel(); // what is on the way is for older text
  problemsHandle = window.setTimeout(runProblems, 400);
}

async function runProblems() {
  const path = currentPath;
  const content = model.getValue();
  if (!path) {
    monaco.editor.setModelMarkers(model, "probescript-lint", []);
    renderProblems(null, [], "");
    return;
  }
  const token = problemsSession.begin();
  let problems: Problem[] = [];
  let note = "";
  try {
    const res = await LintProblems(path, content);
    if (res?.stale) return; // a newer request replaced this one before it ran
    problems = res?.problems ?? [];
    note = res?.unavailable ?? "";
  } catch (err) {
    note = "Lint unavailable: " + String(err);
  }
  if (!problemsSession.accept(token, content, model.getValue()) || currentPath !== path) return; // the text or the file changed meanwhile
  monaco.editor.setModelMarkers(model, "probescript-lint", toMarkers(problems, model.getLineCount(), (l) => model.getLineMaxColumn(l)));
  renderProblems(path, problems, note);
}

editor.onDidChangeModelContent(() => {
  scheduleLint();
  scheduleProblems();
  setDirty(true);
});

// ---- File browser --------------------------------------------------------

let currentPath: string | null = null;
let currentDir = "tests";
// The workspace root: the folder given to SetWorkspace. The Go side reads its probe.yaml for the chat settings, so the
// settings overlay and the chat line read the same file. currentDir moves when the file pane is browsed; this does not.
let workspaceRoot = "";
let dirty = false;
// First-run guidance: nothing is said while the workspace is still being opened at start-up, and the device list is not judged empty
// before its first listing has arrived.
let bootLoading = true;
let devicesLoaded = false;
let listing: FirstRunInput["listing"] = "unknown";
let adbNote = ""; // why Android devices may be missing from the list (adb not found), from Go; plain text
// The replaced line the card can re-run, whether that re-run is ours, and whether a run is going (declared here: setDirty reads them at start-up).
let fixRerun: RerunTarget | null = null;
let rerunInFlight = false;
let runInProgress = false;

function setDirty(d: boolean) {
  dirty = d;
  $("editor-dirty").textContent = d ? "● unsaved" : "";
  ($("btn-save") as HTMLButtonElement).disabled = !d || (!currentPath && !workspaceRoot); // a buffer with no file is saved as a new one in the workspace
  refreshRerun();
  refreshFirstRun();
}

async function refreshFiles(dir: string = currentDir) {
  currentDir = dir;
  $("file-path").textContent = dir;
  const list = $("file-list");
  clearChildren(list);
  try {
    const entries = await ListDir(dir);
    listing = entries.length === 0 ? "empty" : "files";
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
    if (entries.length === 0) renderEmptyPane(list);
  } catch (err) {
    console.error("ListDir failed", err);
    listing = "failed";
    toast(`Cannot read ${dir}: ${err}`, "error");
    renderEmptyPane(list, true);
  }
  refreshFirstRun();
}

// renderEmptyPane says what an empty Workspace pane needs: a workspace to open, or .probe files to add. It says nothing while the workspace
// is loading (no flash), and the line is plain text.
function renderEmptyPane(list: HTMLElement, failed = false): void {
  const say = workspacePaneText({ loading: bootLoading, workspaceOpen: workspaceRoot !== "" }, 0, failed);
  if (!say) return;
  const li = document.createElement("li");
  li.classList.add("empty", "pane-empty");
  li.append(textEl("span", "", say.text));
  if (say.action) {
    const btn = textEl("button", "btn", "Open workspace");
    btn.id = "btn-workspace-hint";
    btn.setAttribute("type", "button");
    btn.addEventListener("click", () => void pickWorkspace());
    li.append(document.createElement("br"), btn);
  }
  list.appendChild(li);
}

// firstRunInput is what the guidance reads, from the window's own state.
function firstRunInput(): FirstRunInput {
  const select = $("device-picker") as HTMLSelectElement;
  return {
    loading: bootLoading,
    workspaceOpen: workspaceRoot !== "",
    fileOpen: currentPath !== null,
    dirty,
    devicesLoaded,
    deviceCount: lastDeviceList.length,
    bootedCount: lastDeviceList.filter((d) => d.booted).length,
    deviceSelected: select.value !== "",
    deviceBooted: selectedDeviceIsBooted(),
    connected,
    running: runInProgress,
    listing,
    adbNote,
  };
}

// refreshFirstRun shows the one next step and gives each disabled primary button its reason: as its title and as a description the button
// points to (aria-describedby), so a screen reader reads why it is disabled. A button that is enabled has no reason and no description.
function refreshFirstRun(): void {
  const i = firstRunInput();
  const step = nextStep(i);
  const strip = $("next-step");
  strip.textContent = step;
  strip.hidden = step === "";
  const set = (id: string, reasonId: string, reason: string, enabledTitle: string) => {
    const btn = $(id) as HTMLButtonElement;
    const text = btn.disabled ? reason : "";
    $(reasonId).textContent = text;
    if (text) btn.setAttribute("aria-describedby", reasonId);
    else btn.removeAttribute("aria-describedby");
    btn.title = text || enabledTitle;
  };
  set("btn-run", "reason-run", runReason(i), "Run the open file");
  set("btn-record", "reason-record", recordReason(i), "Record interactions (⌘⇧R)");
  set("btn-save", "reason-save", saveReason(i), "Save (⌘S)");
  const dev = lastDeviceList.find((d) => d.id === ($("device-picker") as HTMLSelectElement).value);
  set("btn-connect", "reason-connect", connectReason(i, dev?.hint ?? ""), connected ? "Disconnect from device" : "Connect to selected device");
}

async function onFileClick(path: string, isDir: boolean) {
  if (isDir) {
    refreshFiles(path);
    return;
  }
  if (!path.endsWith(".probe")) return;
  await openProbeFile(path);
}

// openProbeFile puts a file in the editor. It asks about unsaved changes first, reads the file through ReadFile (Studio refuses a path
// outside the workspace, with a .. segment, through a link out of it, and a file that is not there) and changes nothing until that has
// worked. It returns whether the file is open now.
async function openProbeFile(path: string): Promise<boolean> {
  if (dirty && !(await askDiscard())) return false;
  try {
    const content = await ReadFile(path);
    model.setValue(content);
    currentPath = path;
    $("editor-filename").textContent = path;
    setDirty(false);
    runLint();
    scheduleProblems();
    document
      .querySelectorAll("#file-list li")
      .forEach((li) =>
        li.classList.toggle(
          "active",
          (li as HTMLElement).dataset.path === path
        )
      );
    updateRunButton();
    return true;
  } catch (err) {
    console.error("ReadFile failed", err);
    toast(`Cannot open ${path}: ${err}`, "error");
    return false;
  }
}

$("btn-files-up").addEventListener("click", () => {
  if (currentDir === "." || currentDir === "/") return;
  const parent = currentDir.split("/").slice(0, -1).join("/") || ".";
  // Studio lists only inside the open workspace, so the up button stops at its root.
  if (workspaceRoot && !(parent === workspaceRoot || parent.startsWith(`${workspaceRoot}/`))) return;
  refreshFiles(parent);
});

$("btn-files-refresh").addEventListener("click", () => {
  void refreshWorkspaceNote(); // probe.yaml may have been edited outside Studio
  void refreshFiles(currentDir);
});

// refreshWorkspaceNote says in the Workspace pane what in the workspace's probe.yaml will trip a first run: no file, a file that cannot be read,
// or no project.app. It reads one file in the workspace, shows plain text, and is hidden when there is nothing to say.
async function refreshWorkspaceNote(): Promise<void> {
  const el = $("workspace-note");
  try {
    const n = await WorkspaceNote();
    el.textContent = n?.text ?? "";
    el.hidden = !n?.text;
  } catch {
    el.textContent = "";
    el.hidden = true;
  }
}

async function pickWorkspace() {
  try {
    const path = await PickWorkspace();
    if (!path) return;
    localStorage.setItem(WORKSPACE_KEY, path);
    await SetWorkspace(path);
    resetAIPreview();
    workspaceRoot = path;
    showWorkspaceClose();
    resetNetworkPanel();
    refreshFiles(path);
    void refreshWorkspaceNote();
    toast(`Workspace: ${path}`, "success", 1800);
  } catch (err) {
    toast(`Open failed: ${err}`, "error");
  }
}

$("btn-pick-workspace").addEventListener("click", pickWorkspace);

// The Close button is there only while a workspace is open. Closing brings back the state of a first start (no workspace, no file, the guidance in
// the panes), and forgets the remembered folder, so the next start does not reopen it. PROBE_STUDIO_WORKSPACE, when set, still opens its folder at start.
function showWorkspaceClose(): void {
  ($("btn-close-workspace") as HTMLButtonElement).hidden = workspaceRoot === "";
}

async function closeWorkspace(): Promise<void> {
  if (workspaceRoot === "") return;
  if (dirty && !(await askDiscard())) return; // unsaved changes: the same question as opening another file
  try {
    await CloseWorkspace(); // Go refuses while a run or a recording is going (and says so)
  } catch (err) {
    toast(String(err).replace(/^Error:\s*/, ""), "info", 4000);
    return;
  }
  localStorage.removeItem(WORKSPACE_KEY);
  workspaceRoot = "";
  resetAIPreview();
  resetNetworkPanel();
  model.setValue("");
  currentPath = null;
  $("editor-filename").textContent = "untitled.probe";
  setDirty(false);
  updateRunButton();
  showWorkspaceClose();
  void refreshWorkspaceNote();
  // The tree is cleared here, not listed again: with a folder from the environment still reachable, a listing of a relative path would be refused and say so.
  currentDir = "tests";
  $("file-path").textContent = currentDir;
  const list = $("file-list");
  clearChildren(list);
  listing = "empty";
  renderEmptyPane(list);
  refreshFirstRun();
  toast("Workspace closed", "info", 1800);
}

$("btn-close-workspace").addEventListener("click", () => void closeWorkspace());

// ---- Save ---------------------------------------------------------------

// saveAs gives an unsaved buffer (a recording, a scratch buffer) a file: the name is asked in the page, a file that exists is never replaced, and the new
// file becomes the open one (so Run and Save work on it).
async function saveAs(): Promise<void> {
  if (!workspaceRoot) {
    toast("Open a workspace first: Save needs a folder to put the new file in.", "info", 4000);
    return;
  }
  // the folder the Workspace pane shows, when it is inside the workspace; the workspace root otherwise
  const dir = currentDir === workspaceRoot || currentDir.startsWith(`${workspaceRoot}/`) ? currentDir : workspaceRoot;
  let existing: string[];
  try {
    existing = (await ListDir(dir)).map((e) => e.name);
  } catch (err) {
    toast(`Cannot save here: ${err}`, "error");
    return;
  }
  const typed = $("editor-filename").textContent === "recorded.probe" ? "recorded.probe" : "untitled.probe";
  const name = await askSaveAsName(typed, existing, dir);
  if (name === null) return;
  const path = `${dir.replace(/\/+$/, "")}/${name}`;
  const content = model.getValue();
  try {
    await WriteFile(path, content);
  } catch (err) {
    toast(`Save failed: ${err}`, "error");
    return;
  }
  currentPath = path;
  $("editor-filename").textContent = path;
  setDirty(false);
  updateRunButton();
  await refreshFiles(dir);
  document.querySelectorAll("#file-list li").forEach((li) => li.classList.toggle("active", (li as HTMLElement).dataset.path === path));
  toast(`Saved ${path}`, "success", 1800);
}

async function saveCurrent() {
  if (!currentPath) {
    if (dirty) await saveAs();
    return;
  }
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
  void loadStudioPrefs();

  try {
    const s = await LoadWorkspaceSettings(workspaceRoot);
    loadedSettings = s; // what the form shows now: Save sends it back, and writes only what the person changed from it
    ($("settings-port") as HTMLInputElement).value = s.agentPort ? String(s.agentPort) : "";
    ($("settings-timeout") as HTMLInputElement).value = s.defaultsTimeout ?? "";
    ($("settings-clear-each-test") as HTMLInputElement).checked = s.clearDataBeforeEachTest === true;
    ($("settings-test-timeout") as HTMLInputElement).value = s.testTimeout ?? "";
    ($("settings-retry") as HTMLInputElement).value = s.retryFailedTests ? String(s.retryFailedTests) : "";
    ($("settings-implicit-wait") as HTMLInputElement).value = s.implicitWait ?? "";
    ($("settings-ios") as HTMLInputElement).value = s.iosDeviceId ?? "";
    const iosProblem = $("settings-ios-problem");
    iosProblem.textContent = s.iosDeviceIdProblem ?? ""; // text, never markup: it quotes the value from probe.yaml
    iosProblem.hidden = !s.iosDeviceIdProblem;
    ($("settings-android") as HTMLInputElement).value = s.androidDeviceId ?? "";
    ($("settings-ai-run-results") as HTMLInputElement).checked = s.aiContextRunResults === true;
    ($("settings-ai-widget-tree") as HTMLInputElement).checked = s.aiContextWidgetTree === true;
    fillAIForm(await LoadAISettings(workspaceRoot));
  } catch (err) {
    toast(`Cannot load settings: ${err}`, "error");
  }
}

// The settings the form was filled from, sent back with Save so that Studio writes only what changed since, and notices a probe.yaml that changed on disk.
let loadedSettings: main.WorkspaceSettings | undefined;

// The person's own Studio preferences (not in probe.yaml). The box is saved the moment it is pressed, and put back if Studio refuses.
async function loadStudioPrefs(): Promise<void> {
  const box = $("settings-clear-data-sim") as HTMLInputElement;
  const warning = $("settings-prefs-warning");
  try {
    const prefs = await GetStudioPrefs();
    box.checked = prefs.clearDataWithoutAsking === true;
    warning.textContent = prefs.warning ?? ""; // text, never markup
    warning.hidden = !prefs.warning;
  } catch {
    box.checked = false;
    warning.hidden = true;
  }
}
$("settings-clear-data-sim").addEventListener("change", async (e) => {
  const box = e.target as HTMLInputElement;
  const want = box.checked;
  try {
    await SetClearDataWithoutAsking(want);
    toast(want ? "clear app data now runs without asking on simulators and emulators" : "clear app data asks again", "info", 4000);
  } catch (err) {
    box.checked = !want;
    toast(`Not saved: ${err}`, "error", 6000);
  }
});

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
// change. The gate lives in the window: Chat sends without it, also through the headless bridge. (A selector fix cannot: it takes PreviewSelectorFix and then SelectorFixPreviewed, and the second sends only what the first returned.)
let aiPreviewConfirmed: string | null = null;

function resetAIPreview(): void {
  aiPreviewConfirmed = null;
  explainSession.reset(); // the same events change where an Explain goes, so its preview is shown again, and one in flight is dropped
  dropExplainView();
  fixSession.invalidate(); // and where a suggested fix goes: Studio asks for a new preview, and one in flight is dropped
  dropFixView();
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
// The AI form as it was loaded (or last saved). Save sends it back as `base`, so Studio writes only what the person changed from it and
// notices a probe.yaml that changed on disk under the form.
let loadedAI: main.AISettings | undefined;

function fillAIForm(s: main.AISettings): void {
  setAIFields(s);
  $("settings-ai-error").hidden = true;
  $("settings-ai-models").replaceChildren();
  $("settings-ai-find-status").textContent = "";
  $("settings-ai-result").replaceChildren();
}

// setAIFields puts the settings the file holds into the form's fields, and remembers them as the form's base: the next Save writes only what
// the person changes from them. It leaves the connection test and the model list alone, so it can refresh a form that stays open.
function setAIFields(s: main.AISettings): void {
  loadedAI = main.AISettings.createFrom({ ...s, base: undefined });
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
    base: loadedAI,
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
const refreshExplainOffer = latestWins(
  () => ExplainAvailability(),
  (offer) => {
    $("explain-bar").hidden = offer.available !== true;
    if (offer.available !== true) {
      hideExplainCard();
      // A passing re-run leaves no failed test, so the offer goes; the card that shows its verdict stays until the person closes it.
      if (!fixRerun) hideFixCard();
    }
  },
  () => {
    $("explain-bar").hidden = true;
    hideExplainCard();
    if (!fixRerun) hideFixCard();
  },
);

// dropExplainView is what a new run, a change of workspace, settings or key does to an Explain on screen: the card of the old state
// goes, an open preview is closed as if it were cancelled, the status is cleared, and the button can be used at once. The Explain
// that was in flight is dropped by the session (see ExplainSession.invalidate), so its answer never shows.
let dismissExplainConfirm: (() => void) | null = null;
function dropExplainView(): void {
  dismissExplainConfirm?.();
  hideExplainCard();
  ($("btn-explain") as HTMLButtonElement).disabled = false;
  $("explain-status").textContent = "";
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
  hideFixCard(); // one advisory card at a time
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
    const jump = placeButton(row.file, row.line);
    if (jump) div.append(jump);
    card.append(div);
  }
  card.hidden = false;
}

// placeButton is the small button on a card that opens the place a failure is at, with the same click as a failing row in Results (the
// jump decides what may open). It names the line (`L12`) and the file's name, never its folder; with no file it is nothing.
function placeButton(file: string | undefined, line: number | undefined): HTMLElement | null {
  const label = placeLabel(file, line);
  if (!label) return null;
  const btn = textEl("button", "btn-mini goto-place", label.text);
  btn.setAttribute("type", "button");
  btn.setAttribute("title", label.title);
  btn.addEventListener("click", () => document.dispatchEvent(new CustomEvent("studio:goto-line", { detail: { file, line } })));
  return btn;
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
      document.removeEventListener("keydown", onKey);
      dismissExplainConfirm = null;
      resolve(ok);
    };
    const onSend = () => finish(true);
    const onCancel = () => finish(false);
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") finish(false); // Escape is Cancel: nothing is sent
      if (e.key === "Tab") {
        // the dialog is modal: Tab stays between its two buttons
        e.preventDefault();
        (document.activeElement === cancel ? send : cancel).focus();
      }
    };
    send.addEventListener("click", onSend);
    cancel.addEventListener("click", onCancel);
    document.addEventListener("keydown", onKey);
    dismissExplainConfirm = () => finish(false);
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
    if (out?.kind === "result") showExplainCard(out.result); // a stale or cancelled Explain shows nothing
  } catch (err) {
    status.textContent = `Explain failed: ${err}`;
    return;
  } finally {
    btn.disabled = explainSession.running; // a newer Explain that is in flight keeps its button disabled
  }
  if (!explainSession.running) status.textContent = "";
});

EventsOn("run:started", () => {
  explainSession.invalidate(); // an Explain of the previous run must not show for this one
  dropExplainView();
  fixSession.invalidate(); // nor a suggested fix for a step of the previous run
  if (!rerunInFlight) dropFixView(); // the re-run of a replaced line is the one run that keeps its card, to show the verdict there
});
EventsOn("run:finished", (results: unknown[]) => {
  stopWaitTimer();
  void refreshExplainOffer();
  setExportEnabled(Array.isArray(results) && results.length > 0);
});
EventsOn("run:started", () => setExportEnabled(false)); // the results of the last run are gone from the export until this one ends

// ---- Export the results of the last run (a JSON or JUnit report), to a place you choose -------------

// syncExportButtons asks Studio whether there is a finished run to export and no run going. It is asked after a dialog and when the window
// loads, so a reload that kept the results, or a run that started during the dialog, shows the true state.
async function syncExportButtons(): Promise<void> {
  try {
    setExportEnabled(await CanExportRunResults());
  } catch {
    setExportEnabled(false);
  }
}
void syncExportButtons();

function setExportEnabled(on: boolean): void {
  for (const id of ["btn-export-json", "btn-export-junit"]) ($(id) as HTMLButtonElement).disabled = !on;
}

async function exportResults(format: "json" | "junit"): Promise<void> {
  setExportEnabled(false); // one save dialog at a time
  try {
    const out = await ExportRunResults(format);
    if (out.saved) toast(`Saved ${out.name} (${out.tests} test${out.tests === 1 ? "" : "s"})`, "success");
  } catch (err) {
    toast(`Export failed: ${err}`, "error", 6000);
  } finally {
    await syncExportButtons(); // what the buttons do follows what Studio holds now, not what they were before the dialog
  }
}
$("btn-export-json").addEventListener("click", () => void exportResults("json"));
$("btn-export-junit").addEventListener("click", () => void exportResults("junit"));

// ---- Suggest a fix: one ProbeScript line for the failed step of the last run ------------

const fixSession = new FixSession();

// dismissFixConfirm closes an open preview as if it were cancelled.
let dismissFixConfirm: (() => void) | null = null;

function hideFixCard(): void {
  fixRerun = null;
  const card = $("fix-card");
  card.hidden = true;
  card.replaceChildren();
}

// dropFixView is what a new run, a change of workspace, settings or key does to a suggestion on screen: the card goes, an open preview
// closes as if cancelled, the status is cleared and the button can be used at once.
function dropFixView(): void {
  dismissFixConfirm?.();
  hideFixCard();
  ($("btn-fix") as HTMLButtonElement).disabled = false;
}

// showFixCard writes the advisory card. It sits beside the results and never touches them: the pass and fail counts and the rows stay as
// the run left them. Nothing in it is applied to the file.
function showFixCard(answer: FixAnswer, step: string): void {
  fixRerun = null; // a new suggestion replaces the card of the replacement before it
  const model = fixCardModel(answer, step);
  hideExplainCard(); // one advisory card at a time
  const card = $("fix-card");
  card.replaceChildren();
  const head = document.createElement("div");
  head.className = "explain-card-head";
  const close = textEl("button", "btn-mini explain-card-close", "✕");
  close.setAttribute("type", "button");
  close.setAttribute("title", "Close");
  close.addEventListener("click", hideFixCard);
  head.append(textEl("span", "explain-card-title", model.title), textEl("span", "explain-card-dest", model.destination), textEl("span", `explain-place explain-place-${model.place}`, model.place), close);
  card.append(head, textEl("p", "explain-disclaimer", model.disclaimer));
  for (const row of model.rows) {
    const div = document.createElement("div");
    div.className = `explain-row explain-row-${row.kind}`;
    div.append(textEl("div", "explain-row-label", row.label), textEl("p", "explain-row-text", row.text));
    const jump = placeButton(row.file, row.line);
    if (jump) div.append(jump);
    card.append(div);
  }
  if (canOfferReplace(answer)) {
    const bar = document.createElement("div");
    bar.className = "explain-actions";
    const btn = textEl("button", "btn btn-primary", "Replace the line") as HTMLButtonElement;
    btn.setAttribute("type", "button");
    btn.id = "btn-fix-replace";
    btn.setAttribute("title", REPLACE_NOTE);
    btn.addEventListener("click", () => void replaceFailedLine(answer, btn));
    bar.append(btn, textEl("span", "dim", REPLACE_NOTE));
    card.append(bar);
  }
  card.hidden = false;
}

// replaceFailedLine asks Studio whether the suggestion may replace the failed step's line in the file as the editor holds it now, and
// applies it as one undo step when Studio says yes. It saves nothing and runs nothing.
async function replaceFailedLine(answer: FixAnswer, btn: HTMLButtonElement): Promise<void> {
  const status = $("explain-status");
  if (!answer.id || !currentPath) return;
  btn.disabled = true;
  try {
    const checked = model.getValue();
    const check = await CheckReplaceLine(answer.id, currentPath, checked);
    const plan = planReplace(check, checked, model.getValue());
    if (plan.kind === "refuse") {
      status.textContent = `Not replaced: ${plan.reason}`;
      return;
    }
    editor.pushUndoStop();
    editor.executeEdits("replace-line", [{ range: new monaco.Range(plan.line, 1, plan.line, model.getLineMaxColumn(plan.line)), text: plan.text }]);
    editor.pushUndoStop();
    editor.revealLineInCenterIfOutsideViewport(plan.line);
    status.textContent = `Line ${plan.line} replaced in the editor. It is not saved and not run; Undo restores it.`;
    showReplacedCard({ file: currentPath, line: plan.line, text: plan.text, lineCount: model.getLineCount(), header: enclosingTest(editorLines(), plan.line) });
  } catch (err) {
    status.textContent = `Not replaced: ${err instanceof Error ? err.message : String(err)}`;
  } finally {
    btn.disabled = false;
  }
}

// ---- After a replacement: Re-run the failed test ------------------------------------------------------------------------------------
//
// The card stays, with one button. It never saves (the person saves: Save, or Cmd+S) and it runs the replaced line's test only, once per
// click. It is for that line in that file: see rerunGate.

function editorLines(): string[] {
  return model.getLinesContent();
}

function rerunContext() {
  const lines = model.getLineCount();
  return {
    lineCount: lines,
    header: fixRerun ? enclosingTest(editorLines(), fixRerun.line) : null,
    openFile: currentPath,
    dirty,
    connected,
    running: runInProgress,
    lineText: fixRerun && fixRerun.line >= 1 && fixRerun.line <= lines ? model.getLineContent(fixRerun.line) : null,
  };
}

// refreshRerun enables or disables the button and says why. It runs whenever something it depends on changes.
function refreshRerun(): void {
  const btn = document.getElementById("btn-fix-rerun") as HTMLButtonElement | null;
  const hint = document.getElementById("fix-rerun-hint");
  if (!btn || !hint) return;
  const gate = rerunGate(fixRerun, rerunContext());
  btn.disabled = !gate.enabled;
  hint.textContent = gate.reason;
}

function showReplacedCard(target: RerunTarget): void {
  fixRerun = target;
  hideExplainCard();
  const card = $("fix-card");
  card.replaceChildren();
  const head = document.createElement("div");
  head.className = "explain-card-head";
  const close = textEl("button", "btn-mini explain-card-close", "✕");
  close.setAttribute("type", "button");
  close.setAttribute("title", "Close");
  close.addEventListener("click", hideFixCard);
  head.append(textEl("span", "explain-card-title", "Suggested fix"), close);
  const row = document.createElement("div");
  row.className = "explain-row";
  row.append(textEl("div", "explain-row-label", "Replaced"), textEl("p", "explain-row-text", `Line ${target.line} is replaced in the editor. It is not saved and not run: save the file, then re-run its test.`));
  const bar = document.createElement("div");
  bar.className = "explain-actions";
  const btn = textEl("button", "btn btn-primary", "Re-run the failed test") as HTMLButtonElement;
  btn.setAttribute("type", "button");
  btn.id = "btn-fix-rerun";
  btn.addEventListener("click", () => void rerunFixedTest());
  const hint = textEl("span", "dim", "");
  hint.id = "fix-rerun-hint";
  bar.append(btn, hint);
  const verdict = document.createElement("div");
  verdict.id = "fix-verdict";
  verdict.setAttribute("role", "status");
  verdict.setAttribute("aria-live", "polite");
  card.append(head, row, bar, verdict);
  card.hidden = false;
  refreshRerun();
}

async function rerunFixedTest(): Promise<void> {
  const target = fixRerun;
  if (!target || !currentPath || rerunInFlight) return;
  if (!rerunGate(target, rerunContext()).enabled) {
    refreshRerun();
    return;
  }
  const startedWith = model.getValue();
  const verdictEl = $("fix-verdict");
  verdictEl.replaceChildren();
  rerunInFlight = true;
  setRunning(true);
  ($("btn-run") as HTMLButtonElement).disabled = true;
  refreshFirstRun(); // Run is disabled because a re-run is in flight, and says so (also while the person types: setDirty reads the state, not the button)
  let verdict;
  try {
    // The test that holds the replaced line, once. Nothing is saved first: the gate has already refused a file with unsaved changes.
    const results = await RunFileScoped(currentPath, target.line, "", false);
    const changed = model.getValue() !== startedWith || dirty;
    verdict = rerunVerdict(
      results.map((r) => ({ name: r.name, passed: r.passed, skipped: r.skipped, error: r.error })),
      changed,
    );
  } catch (err) {
    verdict = rerunVerdict([{ name: "the test", passed: false, skipped: false, error: String(err) }], false);
  } finally {
    rerunInFlight = false;
    setRunning(false);
    updateRunButton();
  }
  if (fixRerun !== target) return; // the card was closed or replaced while the test ran
  verdictEl.className = `fix-verdict fix-verdict-${verdict.kind}`;
  verdictEl.append(textEl("p", "explain-row-text", verdict.text));
  if (verdict.stale) verdictEl.append(textEl("p", "dim", STALE_VERDICT_NOTE));
  if (verdict.kind === "failed") {
    // Still failing: offer another suggestion, for as long as Studio allows one for this step. It is a button, never automatic.
    let live: { used: number; max: number } | null = null;
    try {
      live = await SelectorFixAttempts(); // Studio's own count for the step that failed, which is not what the replaced answer said
    } catch {
      live = null; // no count: the offer is there, and Studio still refuses past its cap
    }
    if (fixRerun !== target) return; // the card went while the count was read
    const offer = againOffer(live?.used, live?.max);
    const bar = document.createElement("div");
    bar.className = "explain-actions";
    const again = textEl("button", "btn", offer.label) as HTMLButtonElement;
    again.setAttribute("type", "button");
    again.id = "btn-fix-again";
    again.disabled = !offer.enabled;
    again.addEventListener("click", () => void askForFix());
    bar.append(again, textEl("span", "dim", offer.note));
    verdictEl.append(bar);
  }
  refreshRerun();
}

// confirmFixSend shows what the suggestion would send and waits for Send or Cancel. Nothing is sent before Send.
function confirmFixSend(p: FixPreview): Promise<boolean> {
  const overlay = $("fix-preview-overlay");
  const send = $("btn-fix-preview-send");
  const cancel = $("btn-fix-preview-cancel");
  $("fix-preview-note").textContent = fixPreviewNote(p.hosted, p.attempt, p.maxAttempts);
  $("fix-preview-text").textContent = p.text;
  const opener = document.activeElement as HTMLElement | null;
  overlay.hidden = false;
  cancel.focus(); // the safe choice has the focus: Enter does not send
  return new Promise((resolve) => {
    const finish = (ok: boolean) => {
      overlay.hidden = true;
      opener?.focus?.(); // the focus goes back to where it was
      send.removeEventListener("click", onSend);
      cancel.removeEventListener("click", onCancel);
      document.removeEventListener("keydown", onKey);
      dismissFixConfirm = null;
      resolve(ok);
    };
    const onSend = () => finish(true);
    const onCancel = () => finish(false);
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") finish(false); // Escape is Cancel: nothing is sent
    };
    send.addEventListener("click", onSend);
    cancel.addEventListener("click", onCancel);
    document.addEventListener("keydown", onKey);
    dismissFixConfirm = () => finish(false);
  });
}

$("btn-fix").addEventListener("click", () => void askForFix());

// askForFix is Suggest a fix, from the button next to Explain and from the card after a re-run that still failed: the preview first, the
// confirmation when Studio asks for it, then the one request. Studio counts the attempts and refuses a fourth for the same failed step.
async function askForFix(): Promise<void> {
  const btn = $("btn-fix") as HTMLButtonElement;
  const status = $("explain-status");
  btn.disabled = true;
  status.textContent = "";
  try {
    // The preview comes first. Studio says whether it must be shown; the request goes only after Send, and only the id of that preview.
    const out = await fixSession.run({
      preview: async () => (await PreviewSelectorFix(editor.getValue())) as FixPreview,
      confirm: confirmFixSend,
      send: async (id) => {
        status.textContent = "Asking the model…";
        return (await SelectorFixPreviewed(id)) as FixAnswer;
      },
    });
    if (out?.kind === "answer") showFixCard(out.answer, out.step); // a stale or cancelled suggestion shows nothing
  } catch (err) {
    status.textContent = `Suggest a fix failed: ${String(err).replace(/^Error:\s*/, "")}`;
    return;
  } finally {
    btn.disabled = fixSession.running; // a newer suggestion that is in flight keeps its button disabled
  }
  if (!fixSession.running) status.textContent = "";
}

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
  const retryRaw = ($("settings-retry") as HTMLInputElement).value.trim();
  const retries = retryRaw === "" ? 0 : Number(retryRaw);
  if (!Number.isInteger(retries)) {
    toast("Retries must be a whole number.", "error");
    return;
  }
  const settings = main.WorkspaceSettings.createFrom({
    base: loadedSettings,
    agentPort: port,
    clearDataBeforeEachTest: ($("settings-clear-each-test") as HTMLInputElement).checked,
    testTimeout: ($("settings-test-timeout") as HTMLInputElement).value.trim(),
    retryFailedTests: retries,
    implicitWait: ($("settings-implicit-wait") as HTMLInputElement).value.trim(),
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
    const form = readAIForm();
    await SaveAISettings(workspaceRoot, form);
    // The file now has what the person changed, and keeps any edit made elsewhere under the form: show what it holds, and make it the base
    // for the next Save (the form can stay open on an error). If it cannot be read back, the form itself is the base.
    try {
      setAIFields(await LoadAISettings(workspaceRoot));
    } catch {
      loadedAI = main.AISettings.createFrom({ ...form, base: undefined });
    }
  } catch (err) {
    aiFailure = err;
  }
  try {
    await SaveWorkspaceSettings(workspaceRoot, settings);
    loadedSettings = main.WorkspaceSettings.createFrom({ ...settings, base: undefined }); // the file now has what the form shows (the form can stay open on an AI error)
  } catch (err) {
    toast(`Save failed: ${err}`, "error");
    return;
  }
  void refreshWorkspaceNote(); // the settings write probe.yaml
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
    (saved ? " · a saved token is used if the field is blank (you are asked first if this address is not the one it was saved for)" : "");
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
  // The item is built with textContent only (wifi-label.ts): the name and version are text from any machine on the network.
  const li = buildWiFiItem(document as unknown as DocLike, dev, key) as unknown as HTMLElement;
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
      const wifiStatus = (await ConnectWiFi(host, port, token)) as ConnectionStatus;
      showAppBuild(wifiStatus.appBuild);
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

// showAppBuild shows what the app reports about its build under the Device title, and nothing when it reports nothing: no placeholder, no guess.
// The text is set as text, never as markup.
function showAppBuild(text?: string): void {
  const el = $("app-build");
  el.textContent = text ?? "";
  el.hidden = !text;
}

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
  if (state !== "connected") showAppBuild(undefined); // a build line belongs to a connection
  connected = state === "connected";
  refreshRerun();
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
  refreshFirstRun();
}

function updateRecordButton() {
  const btn = $("btn-record") as HTMLButtonElement;
  btn.disabled = !connected;
  refreshFirstRun();
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
    // Everything that is asked for is asked for before the list is touched: the picker is rebuilt in one synchronous step, never emptied while a
    // question is in flight (a dropdown that is open when its options are replaced loses the pick: the first selection after a launch was ignored).
    let note = "";
    try {
      note = (await DeviceListNote()) ?? "";
    } catch {
      note = "";
    }
    if (seq !== refreshSeq) return; // a newer refresh started while the note was read
    adbNote = note;
    lastDeviceList = devs;
    devicesLoaded = true;
    // Show booted devices first; non-booted are still listed so users see
    // their full simulator inventory, but the Connect button stays disabled.
    const sorted = [...devs].sort(
      (a, b) => Number(b.booted) - Number(a.booted) || a.name.localeCompare(b.name)
    );
    const entries = sorted.map((d) => {
      const tag = d.kind === "physical" ? "physical" : d.platform;
      const model = d.model && !d.name.includes(d.model) ? ` (${d.model})` : "";
      const via = d.transport === "network" ? "  · WiFi only" : d.transport === "usb" ? "  · USB" : "";
      const label = d.booted
        ? `${d.name}${model}  ·  ${tag}${via}`
        : d.kind === "physical"
          ? `${d.name}${model}  ·  ${tag}${via}`
          : `${d.name}  ·  ${tag}  · shutdown`;
      return { id: d.id, label, booted: d.booted };
    });
    // A list that is the same as the one shown is left alone, so an open dropdown and the pick in it are not disturbed by a refresh that found nothing new.
    const shown = Array.from(select.options).slice(1).map((o) => `${o.value}\u0000${o.textContent}\u0000${o.dataset.booted ?? ""}`);
    const wanted = entries.map((e) => `${e.id}\u0000${e.label}\u0000${e.booted ? "" : "false"}`);
    const unchanged = select.options.length === entries.length + 1 && select.options[0].value === "" && shown.every((x, i) => x === wanted[i]);
    if (!unchanged) {
      const previous = select.value; // read when the list is replaced, so a pick made while the refresh was in flight survives
      clearChildren(select);
      select.appendChild(makeOption("", "— select device —"));
      for (const e of entries) {
        const opt = makeOption(e.id, e.label);
        if (!e.booted) opt.dataset.booted = "false";
        select.appendChild(opt);
      }
      if (previous && devs.some((d) => d.id === previous)) {
        select.value = previous;
      }
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
    toast(`Device discovery failed: ${err}`, "error"); // the list was not read, so it is not judged empty either
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
    refreshFirstRun();
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
  refreshFirstRun();
}

($("device-picker") as HTMLSelectElement).addEventListener("change", () => {
  if (!connected && $("status-dot").classList.contains("status-error")) setStatus("disconnected", "disconnected"); // another device: the old failure is not about it
  updateConnectButton();
});
$("btn-refresh").addEventListener("click", refreshDevices);

let connectAttempt = 0;
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
  const attempt = ++connectAttempt; // a result that comes back after a newer attempt started is not about the screen any more
  setStatus("connecting", "connecting…");
  try {
    const status = (await Connect(id)) as ConnectionStatus;
    setStatus(
      "connected",
      `${status.deviceName}  ·  ${status.platform}`,
      `${status.deviceName} (${status.deviceId}) · ${status.platform} · USB transport`
    );
    showAppBuild(status.appBuild);
    startStream();
    toast(`Connected to ${status.deviceName}`, "success");
    if (status.toolsNote) toast(status.toolsNote, "info", 8000); // the connection uses a program that probe.yaml names: said, not silent
  } catch (err) {
    const detail = enrichError(err);
    if (attempt !== connectAttempt) return; // superseded: a newer Connect is under way or has finished, and its result is the one shown
    // The reason stays in the status (full text as the tooltip) until the person picks another device or connects again.
    setStatus("error", connectFailureLine(detail), detail);
    toast(`Connect failed: ${detail}`, "error", 8000);
  }
});

EventsOn("connection:changed", (status: ConnectionStatus) => {
  resetNetworkPanel(); // another app, or none: the request log on screen is not its own
  showAppBuild(status.connected ? status.appBuild : undefined);
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
initNetworkPanel();
initConfirmDialog();
initDiscardDialog();
initSaveAsDialog();
// Save as step puts lines under the cursor of the open file, inside the test the cursor is in, at the indent of its steps (send-insert.ts).
// Nothing is inserted when no file is open, when the editor is read-only, when the cursor is not inside a test, when the lines would add a
// parse error, or when the file changed while that was being checked. The file is written by the editor's own save, not here.
initSendPanel({
  insertLines: async (lines) => {
    if (!currentPath) return { ok: false, message: "Nothing was inserted: no file is open. Open a .probe file and put the cursor inside a test." };
    if (editor.getOption(monaco.editor.EditorOption.readOnly)) return { ok: false, message: "Nothing was inserted: the file is read-only." };
    const pos = editor.getPosition() ?? new monaco.Position(1, 1);
    const before = model.getValue();
    const placed = placeStep(before.split("\n"), pos.lineNumber);
    if ("reason" in placed) return { ok: false, message: `Nothing was inserted. ${placed.reason}` };
    const col = model.getLineMaxColumn(placed.after);
    const text = "\n" + lines.map((l) => placed.indent + l).join("\n");
    const offset = model.getOffsetAt({ lineNumber: placed.after, column: col });
    const next = before.slice(0, offset) + text + before.slice(offset);
    const errors = (ds: { severity: number }[] | null) => (ds ?? []).filter((d) => d.severity === 1).length; // Lint answers null when there is nothing to report
    if (errors(await Lint(next)) > errors(await Lint(before))) return { ok: false, message: "Nothing was inserted: the step would not parse there. Put the cursor on a step inside a test." };
    if (model.getValue() !== before) return { ok: false, message: "Nothing was inserted: the file changed while it was checked. Try again." };
    const applied = editor.executeEdits("send-save", [{ range: new monaco.Range(placed.after, col, placed.after, col), text, forceMoveMarkers: true }]);
    if (!applied) return { ok: false, message: "Nothing was inserted: the editor did not accept the edit." };
    editor.revealLine(placed.after + lines.length);
    return { ok: true };
  },
});

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
  refreshFirstRun();
  try {
    await RunFileScoped(currentPath, scope.args.line, scope.args.tag, scope.args.failedOnly);
  } catch (err) {
    toast(`Run failed: ${err}`, "error", 6000);
  } finally {
    setRunning(false);
    updateRunButton();
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
  runInProgress = active;
  refreshRerun();
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

// The running step and when it started, for the elapsed time and the "still waiting" line. A clock tick once a second updates them while a run
// goes on; the progress text is not a live region (it would be read out every second), the waiting line is and says one thing once.
let runningStep: { text: string; line: number; startedAt: number } | null = null;
let waitTimer: number | undefined;

function runningMs(): number | null {
  return runningStep ? Date.now() - runningStep.startedAt : null;
}

function updateProgress() {
  $("run-progress").textContent = progressText(stepsDone, stepsTotal, runningMs());
}

/** Draws the parts of the toolbar that depend on time and on the device's signal. */
function updateWaiting(): void {
  const active = waitTimer !== undefined; // a run is going (its run:started came and its run:finished has not)
  const noSignal = deviceHasNoSignal();
  const signal = $("run-signal");
  signal.textContent = active ? signalNote(noSignal) : "";
  signal.hidden = !signal.textContent;
  const line = active ? waitingLine(runningStep?.text ?? "", runningMs(), noSignal) : "";
  const el = $("run-waiting");
  if (el.textContent !== line) el.textContent = line;
  el.hidden = line === "";
  if (active) updateProgress();
}

function startWaitTimer(): void {
  if (waitTimer === undefined) waitTimer = window.setInterval(updateWaiting, 1000);
}

function stopWaitTimer(): void {
  if (waitTimer !== undefined) window.clearInterval(waitTimer);
  waitTimer = undefined;
  runningStep = null;
  updateWaiting();
}

document.addEventListener("studio:signal", () => updateWaiting());

EventsOn("run:started", (path: string) => {
  startWaitTimer();
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
  if (s.status === "started") {
    runningStep = { text: s.description, line: s.line, startedAt: Date.now() };
    updateWaiting();
  } else if (runningStep && runningStep.line === s.line) {
    runningStep = null;
    updateWaiting();
  }
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

// A click on a failing row asks for a file and a line (planJump decides what is allowed). Only a click moves the editor.
document.addEventListener("studio:goto-line", (ev) => void gotoFailingLine((ev as CustomEvent<JumpTarget>).detail));

async function gotoFailingLine(target: JumpTarget): Promise<void> {
  const plan = planJump(target ?? {}, { openFile: currentPath, workspaceRoot });
  if (plan.kind === "refuse") {
    toast(`Not opened: ${plan.reason}`, "error", 5000);
    return;
  }
  if (plan.kind === "open" && !(await openProbeFile(plan.path))) return; // cancelled, or refused by Studio: the editor is as it was
  const spot = clampLine(plan.line, model.getLineCount());
  if (spot.note) toast(spot.note, "info", 4000);
  if (spot.line > 0) {
    editor.revealLineInCenter(spot.line);
    editor.setPosition({ lineNumber: spot.line, column: 1 });
  }
  editor.focus();
}

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

let recordingHead = "";

$("btn-record").addEventListener("click", async () => {
  if (!connected) return;

  if (!recording) {
    recordingHead = recordingHeader(new Date().toLocaleString());
    model.setValue(recordingHead);
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
        // the header the recording started with stays: the steps go under its test block
        model.setValue(recordedScript(recordingHead, script));
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
    showWorkspaceClose();
    resetNetworkPanel();
    void refreshWorkspaceNote();
    await refreshFiles(ws);
  } else {
    await refreshFiles();
  }
  finishBoot();
})();

// finishBoot ends the start-up: from now on the guidance may speak. A pane that came back empty while the workspace was loading gets its
// line now, once, and not before.
function finishBoot(): void {
  bootLoading = false;
  const list = $("file-list");
  if ((listing === "empty" || listing === "failed") && !list.querySelector("li.pane-empty")) {
    clearChildren(list);
    renderEmptyPane(list, listing === "failed");
  }
  refreshFirstRun();
}

refreshDevices();
runLint();

// Restore connection if app was relaunched while connected
(async () => {
  try {
    const status = (await Status()) as ConnectionStatus;
    showAppBuild(status.connected ? status.appBuild : undefined);
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
