// The Send request panel of the Network tab: one request typed by a person, sent by Studio's SendRequest, shown redacted. Before a host is
// called for the first time in a session the person is shown where it leads and has to allow it. Everything is set with textContent,
// nothing is stored in the browser, and the only calls are Studio's own CheckTarget, AllowHost and SendRequest.

import { AllowHost, BuildCallStep, CheckTarget, SendRequest } from "../wailsjs/go/main/App";
import { main } from "../wailsjs/go/models";
import { allowText, errorText, parseHeaders, savedSummary, summarize, type SendResultLike, type TargetInfo } from "./send";
import { bodyView, type NetBody, type NetHeader } from "./network";

const $ = <T extends HTMLElement = HTMLElement>(id: string): T => document.getElementById(id) as T;

interface SendResult extends SendResultLike {
  sent: string;
  requestHeaders: NetHeader[];
  url: string;
  responseHeaders: NetHeader[];
  moreHeaders?: number;
  responseBody: NetBody;
}

let sending = false;

function text(tag: string, className: string, value: string): HTMLElement {
  const el = document.createElement(tag);
  if (className) el.className = className;
  el.textContent = value;
  return el;
}

function headerGrid(hs: NetHeader[]): HTMLElement {
  const grid = document.createElement("div");
  grid.className = "net-headers";
  for (const h of hs ?? []) grid.append(text("span", "net-hname", h.name), text("span", "net-hvalue", h.value));
  return grid;
}

function renderResult(r: SendResult): void {
  const out = $("send-result");
  out.replaceChildren();
  const s = summarize(r);
  out.append(text("div", "send-line", r.sent), text("div", "send-line", s.line));
  if (s.hops.length) out.append(text("h4", "", "Redirects followed"), ...s.hops.map((h) => text("div", "send-line", h)));
  if (s.stopped) out.append(text("div", "send-notice", s.stopped));
  out.append(text("h4", "", "Request headers"), headerGrid(r.requestHeaders));
  out.append(text("h4", "", "Response headers"), headerGrid(r.responseHeaders));
  if (r.moreHeaders) out.append(text("div", "dim", `${r.moreHeaders} more not shown`));
  const b = bodyView(r.responseBody);
  out.append(text("h4", "", `Response body: ${b.note}`));
  if (b.text !== "") out.append(text("pre", "", b.text));
}

function status(message: string): void {
  $("send-status").textContent = message;
}

// confirmAllow shows where a host leads and waits for Allow or Cancel (or Escape, which is Cancel). Nothing is sent before Allow.
function confirmAllow(t: TargetInfo): Promise<boolean> {
  const view = allowText(t);
  $("send-allow-title").textContent = view.title;
  $("send-allow-lines").textContent = view.lines.join(" ");
  const warning = $("send-allow-warning");
  warning.textContent = view.warning;
  warning.hidden = view.warning === "";
  const overlay = $("send-allow-overlay");
  const ok = $("btn-send-allow-ok");
  const cancel = $("btn-send-allow-cancel");
  overlay.hidden = false;
  return new Promise((resolve) => {
    const finish = (yes: boolean) => {
      overlay.hidden = true;
      ok.removeEventListener("click", onOk);
      cancel.removeEventListener("click", onCancel);
      document.removeEventListener("keydown", onKey);
      resolve(yes);
    };
    const onOk = () => finish(true);
    const onCancel = () => finish(false);
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") finish(false);
    };
    ok.addEventListener("click", onOk);
    cancel.addEventListener("click", onCancel);
    document.addEventListener("keydown", onKey);
  });
}

async function send(): Promise<void> {
  if (sending) return;
  const url = $<HTMLInputElement>("send-url").value.trim();
  const parsed = parseHeaders($<HTMLTextAreaElement>("send-headers").value);
  if (parsed.error) {
    status(parsed.error);
    return;
  }
  sending = true;
  ($("btn-send") as HTMLButtonElement).disabled = true;
  $("send-result").replaceChildren();
  status("");
  try {
    const info = (await CheckTarget(url)) as TargetInfo;
    if (info.refused && !info.allowed) {
      status(`This address cannot be called from here: ${info.refused}.`);
      return;
    }
    if (!info.allowed) {
      if (!(await confirmAllow(info))) {
        status("Cancelled. Nothing was sent.");
        return;
      }
      await AllowHost(info.key);
    }
    status("Sending…");
    const result = (await SendRequest(main.SendInput.createFrom({ method: $<HTMLSelectElement>("send-method").value, url, headers: parsed.headers, body: $<HTMLTextAreaElement>("send-body").value }))) as SendResult;
    status("");
    renderResult(result);
  } catch (err) {
    status(errorText(err));
  } finally {
    sending = false;
    ($("btn-send") as HTMLButtonElement).disabled = false;
  }
}

/** Saves the typed request as a call step in the open file. Headers are not part of it; Go builds the lines from the method, address and body. */
async function saveStep(insertLines: (lines: string[]) => boolean): Promise<void> {
  const headerLines = $<HTMLTextAreaElement>("send-headers").value.split("\n").filter((l) => l.trim() !== "").length;
  try {
    const step = await BuildCallStep(main.SaveStepInput.createFrom({ method: $<HTMLSelectElement>("send-method").value, url: $<HTMLInputElement>("send-url").value.trim(), body: $<HTMLTextAreaElement>("send-body").value }));
    if (!insertLines(step.lines)) {
      status("Nothing was inserted: no file is open.");
      return;
    }
    status(savedSummary(step.fill, headerLines));
  } catch (err) {
    status(errorText(err));
  }
}

export function initSendPanel(editor: { insertLines: (lines: string[]) => boolean }): void {
  const showSend = (on: boolean) => {
    $("send-panel").hidden = !on;
    $("net-body").hidden = on;
  };
  $("btn-net-send").addEventListener("click", () => showSend(true));
  $("btn-send-back").addEventListener("click", () => showSend(false));
  $("btn-send").addEventListener("click", () => void send());
  $("btn-send-save").addEventListener("click", () => void saveStep(editor.insertLines));
  $("send-url").addEventListener("keydown", (e) => {
    if ((e as KeyboardEvent).key === "Enter") void send();
  });
}
