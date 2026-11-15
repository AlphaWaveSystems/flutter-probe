// The Network tab of the Results pane: the requests and responses the app made, read from Studio's HTTPExchanges and HTTPExchange, which
// redact every string before it gets here. Everything is set with textContent, nothing is stored in the browser, and nothing is sent
// anywhere but to the two Studio methods that read the app's own request log.

import { EventsOn } from "../wailsjs/runtime/runtime";
import { HTTPExchange, HTTPExchanges } from "../wailsjs/go/main/App";
import { latestWins } from "./latest-wins";
import { NetState, RunPoller, bodyView, emptyMessage, formatRow, type NetBody, type NetDetail, type NetHeader, type NetList } from "./network";

const $ = <T extends HTMLElement = HTMLElement>(id: string): T => document.getElementById(id) as T;

const state = new NetState();
let selected = 0; // the sequence number whose detail is shown
let lastError: string | null = null;
let active = false; // the Network tab is the one on show

function filtered(): boolean {
  return $<HTMLSelectElement>("net-method").value !== "" || $<HTMLSelectElement>("net-status").value !== "" || $<HTMLInputElement>("net-text").value.trim() !== "";
}

function text(tag: string, className: string, value: string): HTMLElement {
  const el = document.createElement(tag);
  if (className) el.className = className;
  el.textContent = value;
  return el;
}

function renderList(): void {
  const list = $("net-list");
  list.replaceChildren();
  if (state.rows.length === 0) {
    list.append(text("li", "empty", emptyMessage(lastError, filtered())));
    return;
  }
  for (const r of state.rows) {
    const v = formatRow(r);
    const li = document.createElement("li");
    li.dataset.seq = String(r.seq);
    if (r.seq === selected) li.className = "selected";
    li.append(text("span", "net-method", v.method), text("span", `net-status-${v.statusClass}`, v.status), text("span", "", v.time), text("span", "", v.size));
    const url = text("span", "net-url", v.url);
    if (v.mocked) url.append(" ", text("span", "net-mocked", "mocked"));
    url.title = v.url;
    li.append(url);
    li.addEventListener("click", () => void select(r.seq));
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
    text("div", "dim", `${v.status} · ${v.time} · ${v.size}${v.mocked ? " · mocked" : ""}${d.error ? " · " + d.error : ""}`),
    ...headers("Request headers", d.requestHeaders, d.moreRequestHeaders),
    ...body("Request body", d.requestBody),
    ...headers("Response headers", d.responseHeaders, d.moreResponseHeaders),
    ...body("Response body", d.responseBody),
  );
}

// The latest detail wins: a slow answer for a row that is no longer selected is dropped.
let wanted = 0;
const loadDetail = latestWins<NetDetail>(
  () => HTTPExchange(state.epoch, wanted) as Promise<NetDetail>,
  (d) => renderDetail(d),
  () => renderDetail(null, "That request cannot be shown now. Refresh the list."),
);

async function select(seq: number): Promise<void> {
  selected = seq;
  wanted = seq;
  for (const li of $("net-list").children) (li as HTMLElement).classList.toggle("selected", (li as HTMLElement).dataset.seq === String(seq));
  await loadDetail();
}

let wantIncremental = false;
let pendingError = "";
const refreshOnce = latestWins<NetList>(
  async () => {
    try {
      return (await HTTPExchanges({ since: wantIncremental ? state.since : 0, method: $<HTMLSelectElement>("net-method").value, status: $<HTMLSelectElement>("net-status").value, text: $<HTMLInputElement>("net-text").value })) as NetList;
    } catch (err) {
      pendingError = String(err).replace(/^Error: /, "");
      throw err;
    }
  },
  (list) => {
    lastError = null;
    const replaced = state.apply(list, wantIncremental);
    if (replaced && !state.rows.some((r) => r.seq === selected)) {
      selected = 0;
      renderDetail(null);
    }
    $("net-info").textContent = list.total > list.shown ? `${list.shown} of ${list.total} shown` : state.rows.length ? `${state.rows.length}` : "";
    renderList();
  },
  () => {
    // Studio said no (for example, there is no app connected): its sentence is the list's empty text.
    lastError = pendingError;
    state.clear();
    selected = 0;
    renderDetail(null);
    $("net-info").textContent = "";
    renderList();
  },
);

// refresh reads the request log. A new filter or a click on Refresh reads all of it again; the poll during a run reads what is new.
// The latest answer wins, so a slow answer cannot replace a newer one.
async function refresh(incremental: boolean): Promise<void> {
  wantIncremental = incremental && state.since > 0;
  await refreshOnce();
}

const poller = new RunPoller(() => {
  if (active) void refresh(true);
});

function show(network: boolean): void {
  active = network;
  $("results-panel").hidden = network;
  $("network-panel").hidden = !network;
  for (const [id, on] of [["tab-results", !network], ["tab-network", network]] as const) {
    $(id).classList.toggle("active", on);
    $(id).setAttribute("aria-selected", String(on));
  }
  if (network) void refresh(false);
}

let filterTimer: ReturnType<typeof setTimeout> | null = null;
function filterChanged(): void {
  if (filterTimer) clearTimeout(filterTimer);
  filterTimer = setTimeout(() => void refresh(false), 200);
}

export function initNetworkPanel(): void {
  $("tab-results").addEventListener("click", () => show(false));
  $("tab-network").addEventListener("click", () => show(true));
  $("btn-net-refresh").addEventListener("click", () => void refresh(false));
  for (const id of ["net-method", "net-status"]) $(id).addEventListener("change", filterChanged);
  $("net-text").addEventListener("input", filterChanged);
  EventsOn("run:started", () => {
    state.clear(); // a new run starts a new log
    selected = 0;
    renderDetail(null);
    if (active) renderList();
    poller.start();
  });
  EventsOn("run:finished", () => poller.stop());
  renderList();
}

// A workspace or connection change makes what is on screen stale: it is forgotten, and read again if the tab is on show.
export function resetNetworkPanel(): void {
  state.clear();
  selected = 0;
  renderDetail(null);
  renderList();
  if (active) void refresh(false);
}

