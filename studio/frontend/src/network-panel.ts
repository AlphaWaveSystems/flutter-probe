// The Network tab of the Results pane: the requests and responses the app made, read from Studio's HTTPExchanges and HTTPExchange, which
// redact every string before it gets here. Everything is set with textContent, nothing is stored in the browser, and nothing is sent
// anywhere but to the two Studio methods that read the app's own request log.

import { EventsOn } from "../wailsjs/runtime/runtime";
import { HTTPExchange, HTTPExchanges } from "../wailsjs/go/main/App";
import { NetController, bodyView, emptyMessage, formatRow, type NetApi, type NetBody, type NetDetail, type NetHeader, type NetList, type NetRow, type NetView } from "./network";

const $ = <T extends HTMLElement = HTMLElement>(id: string): T => document.getElementById(id) as T;

function text(tag: string, className: string, value: string): HTMLElement {
  const el = document.createElement(tag);
  if (className) el.className = className;
  el.textContent = value;
  return el;
}

function renderList(rows: NetRow[], selected: number, emptyText: string): void {
  const list = $("net-list");
  list.replaceChildren();
  if (rows.length === 0) {
    list.append(text("li", "empty", emptyText));
    return;
  }
  for (const r of rows) {
    const v = formatRow(r);
    const li = document.createElement("li");
    li.dataset.seq = String(r.seq);
    if (r.seq === selected) li.className = "selected";
    li.append(text("span", "net-method", v.method), text("span", `net-status-${v.statusClass}`, v.status), text("span", "", v.time), text("span", "", v.size));
    const url = text("span", "net-url", v.url);
    if (v.mocked) url.append(" ", text("span", "net-mocked", "mocked"));
    url.title = v.url;
    li.append(url);
    li.addEventListener("click", () => void controller.select(r.seq));
    list.append(li);
  }
  list.scrollTop = list.scrollHeight;
}

function headers(title: string, hs: NetHeader[], more?: number): HTMLElement[] {
  const grid = document.createElement("div");
  grid.className = "net-headers";
  for (const h of hs ?? []) grid.append(text("span", "net-hname", h.name), text("span", "net-hvalue", h.value));
  const out = [text("h4", "", title), grid];
  if (more) out.push(text("p", "dim", `${more} more not shown`));
  return out;
}

function body(title: string, b: NetBody): HTMLElement[] {
  const v = bodyView(b);
  const out = [text("h4", "", `${title}: ${v.note}`)];
  if (v.text !== "") out.push(text("pre", "", v.text));
  return out;
}

function renderDetail(d: NetDetail | null, message = ""): void {
  const el = $("net-detail");
  el.replaceChildren();
  if (!d) {
    el.append(text("p", "dim", message || "Select a request."));
    return;
  }
  const v = formatRow(d);
  el.append(
    text("div", "net-url-full", `${d.method} ${d.url}`),
    text("div", "dim", `${v.status} · ${v.time} · ${v.size}${v.mocked ? " · mocked" : ""}${d.seeded ? " · seeded (test)" : ""}${d.error ? " · " + d.error : ""}`),
    ...headers("Request headers", d.requestHeaders, d.moreRequestHeaders),
    ...body("Request body", d.requestBody),
    ...headers("Response headers", d.responseHeaders, d.moreResponseHeaders),
    ...body("Response body", d.responseBody),
  );
}

const api: NetApi = {
  list: async (since, f) => (await HTTPExchanges({ since, method: f.method, status: f.status, text: f.text })) as NetList,
  detail: async (epoch, seq) => (await HTTPExchange(epoch, seq)) as NetDetail,
};

const view: NetView = {
  filters: () => ({ method: $<HTMLSelectElement>("net-method").value, status: $<HTMLSelectElement>("net-status").value, text: $<HTMLInputElement>("net-text").value }),
  renderList,
  renderDetail,
  markSelected: (seq) => {
    for (const li of $("net-list").children) (li as HTMLElement).classList.toggle("selected", (li as HTMLElement).dataset.seq === String(seq));
  },
  setInfo: (t) => {
    $("net-info").textContent = t;
  },
};

// What is shown and what may still be shown is decided in network.ts; this file only draws.
const controller = new NetController(api, view);

export type Tab = "results" | "network" | "problems";

// showTab shows one of the three panels under the editor and marks its tab.
export function showTab(tab: Tab): void {
  const network = tab === "network";
  controller.setActive(network);
  $("results-panel").hidden = tab !== "results";
  $("network-panel").hidden = !network;
  $("problems-panel").hidden = tab !== "problems";
  for (const [id, on] of [["tab-results", tab === "results"], ["tab-network", network], ["tab-problems", tab === "problems"]] as const) {
    $(id).classList.toggle("active", on);
    $(id).setAttribute("aria-selected", String(on));
  }
}

let filterTimer: ReturnType<typeof setTimeout> | null = null;
function filterChanged(): void {
  if (filterTimer) clearTimeout(filterTimer);
  filterTimer = setTimeout(() => void controller.refresh(false), 200);
}

export function initNetworkPanel(): void {
  $("tab-results").addEventListener("click", () => showTab("results"));
  $("tab-network").addEventListener("click", () => showTab("network"));
  $("tab-problems").addEventListener("click", () => showTab("problems"));
  $("btn-net-refresh").addEventListener("click", () => void controller.refresh(false));
  for (const id of ["net-method", "net-status"]) $(id).addEventListener("change", filterChanged);
  $("net-text").addEventListener("input", filterChanged);
  EventsOn("run:started", () => controller.runStarted());
  EventsOn("run:finished", () => controller.runFinished());
  renderList([], 0, emptyMessage(null, false));
}

// A workspace or connection change makes what is on screen stale: it is forgotten, and read again if the tab is on show.
export function resetNetworkPanel(): void {
  controller.reset();
}
