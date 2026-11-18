// The question when an unsaved buffer (a recording, a scratch buffer) is saved: the name of the new file. It is in the page, not a native prompt(): the
// window's webview shows none. Escape and Cancel say no, Enter in the name field saves (the person typed the name), a click on the dark area is not an
// answer, and Tab stays inside. A name that is not allowed is refused in the dialog with the reason, and the dialog stays open.

import { saveAsName } from "./save-as-name";

const $ = (id: string) => document.getElementById(id) as HTMLElement;

let open: { resolve: (name: string | null) => void; existing: readonly string[]; opener: Element | null } | null = null;

function close(name: string | null): void {
  if (!open) return;
  const { resolve, opener } = open;
  open = null;
  document.removeEventListener("keydown", trapKey, true);
  document.removeEventListener("focusin", keepFocus, true);
  $("save-as-overlay").hidden = true;
  // After the caller's own code has run: the button that opened the dialog is disabled while it is open and enabled again when the answer has been handled,
  // and a disabled button cannot take the focus.
  if (opener instanceof HTMLElement) setTimeout(() => opener.focus(), 0);
  resolve(name);
}

function submit(): void {
  if (!open) return;
  const input = $("save-as-name") as HTMLInputElement;
  const r = saveAsName(input.value, open.existing);
  if (!r.ok) {
    $("save-as-error").textContent = r.reason;
    input.setAttribute("aria-invalid", "true");
    input.focus();
    return;
  }
  close(r.name);
}

function focusables(): HTMLElement[] {
  return Array.from($("save-as-card").querySelectorAll<HTMLElement>("input, button")).filter((e) => !(e as HTMLButtonElement).disabled);
}

function trapKey(ev: KeyboardEvent): void {
  if (!open) return;
  if (ev.key === "Escape") {
    ev.preventDefault();
    ev.stopPropagation();
    close(null);
    return;
  }
  if (ev.key === "Tab") {
    const f = focusables();
    const i = f.indexOf(document.activeElement as HTMLElement);
    ev.preventDefault();
    ev.stopPropagation();
    f[(i + (ev.shiftKey ? f.length - 1 : 1)) % f.length].focus();
  }
}

function keepFocus(ev: FocusEvent): void {
  if (open && !$("save-as-card").contains(ev.target as Node)) ($("save-as-name") as HTMLInputElement).focus();
}

/** Asks for the name of a new file. Resolves with the name (with .probe, checked against `existing`), or null for every other way out. */
export function askSaveAsName(defaultName: string, existing: readonly string[], folder: string, returnTo: HTMLElement | null = null): Promise<string | null> {
  if (open) return Promise.resolve(null);
  return new Promise((resolve) => {
    open = { resolve, existing, opener: returnTo ?? document.activeElement };
    const input = $("save-as-name") as HTMLInputElement;
    input.value = defaultName;
    input.removeAttribute("aria-invalid");
    $("save-as-error").textContent = "";
    $("save-as-folder").textContent = folder;
    $("save-as-overlay").hidden = false;
    document.addEventListener("keydown", trapKey, true);
    document.addEventListener("focusin", keepFocus, true);
    input.focus();
    input.select();
  });
}

export function initSaveAsDialog(): void {
  $("btn-save-as-cancel").addEventListener("click", () => close(null));
  $("btn-save-as-ok").addEventListener("click", submit);
  ($("save-as-name") as HTMLInputElement).addEventListener("keydown", (ev) => {
    if (ev.key === "Enter") {
      ev.preventDefault();
      submit();
    }
  });
  ($("save-as-name") as HTMLInputElement).addEventListener("input", () => {
    $("save-as-error").textContent = "";
    ($("save-as-name") as HTMLInputElement).removeAttribute("aria-invalid");
  });
  $("save-as-overlay").addEventListener("mousedown", (ev) => {
    if (ev.target === $("save-as-overlay")) ev.preventDefault();
  });
}
