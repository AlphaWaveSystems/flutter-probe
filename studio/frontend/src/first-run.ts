// First-run guidance: what Studio says when there is something the person has to do next, and why a primary button is disabled. Nothing
// here touches the DOM, so the node tests run it directly. Every string is set with textContent, and none is shown while the workspace is
// still loading (a hint that appears and goes a moment later is worse than none).

export interface FirstRunInput {
  loading: boolean; // the workspace is still being opened at start-up
  workspaceOpen: boolean;
  fileOpen: boolean; // a .probe file is open in the editor
  dirty: boolean;
  devicesLoaded: boolean; // the first device listing has finished
  deviceCount: number;
  bootedCount: number;
  deviceSelected: boolean;
  deviceBooted: boolean; // the selected device can be connected
  connected: boolean;
}

/** The one thing to do next, or "" when nothing is in the way (or while loading). The order is the order of a first run. */
export function nextStep(i: FirstRunInput): string {
  if (i.loading) return "";
  if (!i.workspaceOpen) return "Open your project folder to start: press the folder button in the Workspace pane.";
  if (!i.fileOpen) return "Open a .probe file from the Workspace pane.";
  if (i.connected) return "";
  if (!i.devicesLoaded) return "";
  if (i.deviceCount === 0) return "No device found: boot a simulator or plug in a phone, then press the refresh button.";
  if (i.bootedCount === 0) return "Every device is shut down: boot a simulator or emulator, then press the refresh button.";
  if (!i.deviceSelected) return "Pick a device in the toolbar, then press Connect.";
  if (!i.deviceBooted) return "That device is shut down: boot it, then press the refresh button.";
  return "Press Connect to see the device and run the file.";
}

/** Why Run is disabled, or "" when it is not. */
export function runReason(i: FirstRunInput): string {
  if (i.connected && i.fileOpen) return "";
  if (!i.fileOpen) return "Open a .probe file first.";
  return "Connect a device first.";
}

/** Why Record is disabled, or "". */
export function recordReason(i: FirstRunInput): string {
  return i.connected ? "" : "Connect a device first.";
}

/** Why Save is disabled, or "". */
export function saveReason(i: FirstRunInput): string {
  if (!i.fileOpen) return "Open a .probe file to save.";
  return i.dirty ? "" : "There are no unsaved changes.";
}

/** Why Connect is disabled, or "" (the device's own hint, when it has one, is passed in). */
export function connectReason(i: FirstRunInput, deviceHint = ""): string {
  if (i.connected) return "";
  if (!i.deviceSelected) return "Pick a device first.";
  if (!i.deviceBooted) return deviceHint || "Boot the simulator or emulator first (for example in Xcode or Android Studio).";
  return "";
}

/** The line in an empty Workspace pane: null while loading or when the pane has files. */
export function workspacePaneText(i: Pick<FirstRunInput, "loading" | "workspaceOpen">, entries: number): { text: string; action: boolean } | null {
  if (i.loading) return null;
  if (!i.workspaceOpen) return { text: "No workspace open. Pick your project folder (the one with probe.yaml).", action: true };
  if (entries === 0) return { text: "No .probe files here. Open another folder, or add tests to this one.", action: false };
  return null;
}
