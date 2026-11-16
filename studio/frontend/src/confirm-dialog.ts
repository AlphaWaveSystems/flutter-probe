// The question Studio asks before a step wipes the app's data. The window shows what Go sent (the app id and the device name, cleaned there)
// inside fixed text; every text is set with textContent. Cancel has the focus, Escape says no, and the button that wipes is never the default:
// Enter does not press it. Nothing here answers by itself.

import { EventsOn } from "../wailsjs/runtime/runtime";
import { AnswerConfirm } from "../wailsjs/go/main/App";
import { toast } from "./toast";
import { CONFIRM_ARM_MS, confirmCopy, keyAction, type ConfirmRequest } from "./confirm-copy";

const $ = (id: string) => document.getElementById(id) as HTMLElement;

let open: { id: string; opener: Element | null; armTimer: number } | null = null;

function close(id?: string): void {
  if (!open || (id !== undefined && id !== open.id)) return;
  const opener = open.opener;
  window.clearTimeout(open.armTimer);
  open = null;
  document.removeEventListener("keydown", trapKey, true);
  document.removeEventListener("focusin", keepFocus, true);
  $("confirm-overlay").hidden = true;
  if (opener instanceof HTMLElement) opener.focus();
}

async function answer(allow: boolean): Promise<void> {
  if (!open) return;
  const id = open.id;
  close(id);
  try {
    await AnswerConfirm(id, allow);
  } catch (err) {
    console.error("AnswerConfirm failed", err);
  }
}

const card = () => document.querySelector(".confirm-card") as HTMLElement;

/** While the dialog is open the keys belong to it, wherever the focus is: Escape says no, Tab stays inside, and any other key aimed at the page behind it is dropped. */
function trapKey(ev: KeyboardEvent): void {
  if (!open) return;
  const active = document.activeElement as HTMLElement | null;
  const inside = !!active && card().contains(active);
  const focused = !inside ? "other" : active.id === "btn-confirm-cancel" ? "cancel" : active.id === "btn-confirm-danger" ? "danger" : "other";
  const act = keyAction(ev.key, ev.shiftKey, focused);
  if (!inside) {
    ev.preventDefault();
    ev.stopPropagation();
    if (act === "deny") void answer(false);
    else $("btn-confirm-cancel").focus();
    return;
  }
  if (act === "default") return;
  ev.preventDefault();
  ev.stopPropagation();
  if (act === "deny") void answer(false);
  else if (act === "press") void answer(true);
  else if (act === "trap-forward") ($(focused === "cancel" ? "btn-confirm-danger" : "btn-confirm-cancel") as HTMLButtonElement).focus();
  else if (act === "trap-back") ($(focused === "danger" ? "btn-confirm-cancel" : "btn-confirm-danger") as HTMLButtonElement).focus();
}

/** Focus that leaves the dialog (a click on the page behind it) comes back to Cancel. */
function keepFocus(ev: FocusEvent): void {
  if (open && !card().contains(ev.target as Node)) $("btn-confirm-cancel").focus();
}

function show(r: ConfirmRequest): void {
  const copy = confirmCopy(r);
  if (!copy) {
    void AnswerConfirm(r.id, false); // an unknown kind is a no
    return;
  }
  if (open) void AnswerConfirm(r.id, false); // one question at a time: Go does not send a second while one is open, and a stray one is a no
  if (open) return;
  $("confirm-title").textContent = copy.title;
  $("confirm-lead").textContent = copy.lead;
  $("confirm-app").textContent = copy.app;
  $("confirm-device").textContent = copy.device;
  $("confirm-app-label").textContent = copy.appLabel;
  $("confirm-warning").textContent = copy.warning;
  $("btn-confirm-cancel").textContent = copy.cancel;
  $("btn-confirm-danger").textContent = copy.danger;
  const danger = $("btn-confirm-danger") as HTMLButtonElement;
  danger.disabled = true; // armed after CONFIRM_ARM_MS
  const id = r.id;
  open = { id, opener: document.activeElement, armTimer: window.setTimeout(() => { if (open && open.id === id) danger.disabled = false; }, CONFIRM_ARM_MS) };
  $("confirm-overlay").hidden = false;
  document.addEventListener("keydown", trapKey, true);
  document.addEventListener("focusin", keepFocus, true);
  $("btn-confirm-cancel").focus(); // Cancel, never the button that wipes
}

export function initConfirmDialog(): void {
  EventsOn("confirm:request", (r: ConfirmRequest) => show(r));
  EventsOn("confirm:close", (id: string) => close(id)); // answered elsewhere, timed out or the run was cancelled
  EventsOn("confirm:note", (text: string) => toast(String(text), "info", 6000));
  $("btn-confirm-cancel").addEventListener("click", () => void answer(false));
  $("btn-confirm-danger").addEventListener("click", (ev) => {
    if ((ev as MouseEvent).detail === 0) return; // a click made by a key (Enter on the focused button) is not a press by the person
    void answer(true);
  });
  // a click on the dark area around the card is not an answer and does not take the focus away from the dialog
  $("confirm-overlay").addEventListener("mousedown", (ev) => {
    if (ev.target === $("confirm-overlay")) ev.preventDefault();
  });
}
