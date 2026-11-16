// The words of the confirmation dialog and what a key does in it. Pure: no DOM, no imports, so the node tests run it directly.

/** How long Clear data stays disabled after the dialog opens: a double click or a key already on its way to the toolbar must not wipe. */
export const CONFIRM_ARM_MS = 500;

export interface ConfirmRequest {
  id: string;
  kind: string;
  appId: string;
  deviceName: string;
}

/** The words of the dialog. Only the app id and the device name vary, and they are shown as text. */
export function confirmCopy(r: ConfirmRequest): { title: string; lead: string; appLabel: string; app: string; device: string; warning: string; cancel: string; danger: string } | null {
  if (r.kind !== "clear-app-data") return null; // a question this window does not know how to put is not shown, and so is not answered yes
  return {
    title: "Clear app data?",
    lead: "A test step wants to delete all the data of this app:",
    appLabel: "App (from probe.yaml)", // the app id of the workspace file, not something Studio found on the device
    app: r.appId || "(unknown app)",
    device: r.deviceName || "(unknown device)",
    warning: "This deletes the app's data and cannot be undone. Nothing is deleted unless you press Clear data.",
    cancel: "Cancel",
    danger: "Clear data",
  };
}

export type KeyAction = "deny" | "ignore" | "press" | "trap-forward" | "trap-back" | "default";

/** What a key does in the open dialog. Enter on the Clear data button does nothing, Space on it presses it (a keyboard user can still say yes, on purpose); Escape is always a no. */
export function keyAction(key: string, shift: boolean, focused: "cancel" | "danger" | "other"): KeyAction {
  if (key === "Escape") return "deny";
  if (key === "Enter" && focused === "danger") return "ignore";
  if (key === " " && focused === "danger") return "press";
  if (key === "Tab") return shift ? "trap-back" : "trap-forward";
  return "default";
}

