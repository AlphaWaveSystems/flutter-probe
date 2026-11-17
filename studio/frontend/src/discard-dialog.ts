// The question before unsaved changes are thrown away (opening another file while the editor has changes). It is in the page, not the browser's
// confirm(): the window's webview does not show a native confirm, which answered no and left the click doing nothing. The safe defaults are the
// clear-data question's: "Keep editing" has the focus, Escape says no, Enter on the discard button does nothing (a click, or Space, says yes), and a
// click on the dark area around the card is not an answer.

import { keyAction } from "./confirm-copy";

const $ = (id: string) => document.getElementById(id) as HTMLElement;

let open: { resolve: (discard: boolean) => void; opener: Element | null } | null = null;

function close(discard: boolean): void {
  if (!open) return;
  const { resolve, opener } = open;
  open = null;
  document.removeEventListener("keydown", trapKey, true);
  document.removeEventListener("focusin", keepFocus, true);
  $("discard-overlay").hidden = true;
  if (opener instanceof HTMLElement) opener.focus();
  resolve(discard);
}

function trapKey(ev: KeyboardEvent): void {
  if (!open) return;
  const active = document.activeElement as HTMLElement | null;
  const inside = !!active && $("discard-card").contains(active);
  const focused = !inside ? "other" : active.id === "btn-discard-cancel" ? "cancel" : active.id === "btn-discard-ok" ? "danger" : "other";
  const act = keyAction(ev.key, ev.shiftKey, focused);
  if (!inside) {
    ev.preventDefault();
    ev.stopPropagation();
    if (act === "deny") close(false);
    else $("btn-discard-cancel").focus();
    return;
  }
  if (act === "default") return;
  ev.preventDefault();
  ev.stopPropagation();
  if (act === "deny") close(false);
  else if (act === "press") close(true);
  else if (act === "trap-forward") ($(focused === "cancel" ? "btn-discard-ok" : "btn-discard-cancel") as HTMLButtonElement).focus();
  else if (act === "trap-back") ($(focused === "danger" ? "btn-discard-cancel" : "btn-discard-ok") as HTMLButtonElement).focus();
}

function keepFocus(ev: FocusEvent): void {
  if (open && !$("discard-card").contains(ev.target as Node)) $("btn-discard-cancel").focus();
}

/** Asks whether unsaved changes may be thrown away. Resolves true only for a press on "Discard changes"; every other way out is a no. */
export function askDiscard(): Promise<boolean> {
  if (open) return Promise.resolve(false); // one question at a time: a second one is a no
  return new Promise<boolean>((resolve) => {
    open = { resolve, opener: document.activeElement };
    $("discard-overlay").hidden = false;
    document.addEventListener("keydown", trapKey, true);
    document.addEventListener("focusin", keepFocus, true);
    $("btn-discard-cancel").focus(); // never the button that discards
  });
}

export function initDiscardDialog(): void {
  $("btn-discard-cancel").addEventListener("click", () => close(false));
  $("btn-discard-ok").addEventListener("click", (ev) => {
    if ((ev as MouseEvent).detail === 0) return; // a click made by a key (Enter on the focused button) is not a press by the person
    close(true);
  });
  $("discard-overlay").addEventListener("mousedown", (ev) => {
    if (ev.target === $("discard-overlay")) ev.preventDefault();
  });
}
