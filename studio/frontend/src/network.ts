// The Network panel's decisions, as plain functions and classes, so they can be tested without a browser. The data comes from Studio's
// HTTPExchanges and HTTPExchange methods, which have already redacted every string; nothing here sees a raw header, query value or body,
// and nothing here stores or sends what it shows.

export interface NetRow {
  seq: number;
  method: string;
  url: string;
  status: number;
  durationMs: number;
  mocked: boolean;
  error?: string;
  requestBytes: number;
  responseBytes: number;
}

export interface NetList {
  epoch: string;
  latest: number;
  total: number;
  shown: number;
  rows: NetRow[];
  seeded?: boolean; // the rows are the test seed of a headless build, not an app's
}

export interface NetHeader {
  name: string;
  value: string;
}

export interface NetBody {
  kind: string; // "empty", "text", "binary", "large" or "limit"
  text?: string;
  size: number;
  truncated?: boolean;
}

export interface NetDetail extends NetRow {
  epoch: string;
  seeded?: boolean;
  requestHeaders: NetHeader[];
  responseHeaders: NetHeader[];
  moreRequestHeaders?: number;
  moreResponseHeaders?: number;
  requestBody: NetBody;
  responseBody: NetBody;
}

/** The rows the list keeps: the newest, as the listing itself does. */
export const MAX_ROWS = 500;

/** The rows on screen, and what is needed to ask for only what is new. */
export class NetState {
  epoch = "";
  latest = 0;
  rows: NetRow[] = [];

  /** The sequence number to ask after: 0 when there is nothing on screen. */
  get since(): number {
    return this.rows.length === 0 ? 0 : this.latest;
  }

  /** Forgets everything, for a new filter or an app that has restarted. */
  clear(): void {
    this.epoch = "";
    this.latest = 0;
    this.rows = [];
  }

  /**
   * Takes an answer. A full answer (the request was for everything) replaces the rows. An incremental one adds the rows after the
   * last one, unless the epoch changed, which means the app restarted and its numbers start over: then it replaces them.
   * Returns "replaced" when what was on screen was replaced, "added" when rows were appended, and "reread" when an incremental answer
   * came from another epoch: the app restarted, and an answer that asked only for what is new may be missing the first rows of the new
   * run, so nothing is taken from it and the caller must read everything again.
   */
  apply(list: NetList, incremental: boolean): "replaced" | "added" | "reread" {
    const restarted = list.epoch !== this.epoch && this.epoch !== "";
    if (incremental && restarted) return "reread";
    if (!incremental || this.rows.length === 0) {
      this.rows = list.rows.slice(-MAX_ROWS);
      this.epoch = list.epoch;
      this.latest = Math.max(list.latest, this.rows.length ? this.rows[this.rows.length - 1].seq : 0);
      return "replaced";
    }
    const added = list.rows.filter((r) => r.seq > this.latest);
    this.rows = this.rows.concat(added).slice(-MAX_ROWS);
    this.epoch = list.epoch;
    this.latest = Math.max(this.latest, list.latest, added.length ? added[added.length - 1].seq : 0);
    return "added";
  }
}

export function formatSize(n: number): string {
  if (!Number.isFinite(n) || n < 0) return "?";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`;
  if (n < 1024 ** 4) return `${(n / 1024 ** 3).toFixed(1)} GB`;
  return "over 1 TB";
}

export type StatusClass = "ok" | "redirect" | "client" | "server" | "error";

export function statusClass(status: number, error?: string): StatusClass {
  if (error || status <= 0 || !Number.isFinite(status)) return "error";
  if (status >= 500) return "server";
  if (status >= 400) return "client";
  if (status >= 300) return "redirect";
  return "ok";
}

export interface RowView {
  method: string;
  url: string;
  status: string;
  statusClass: StatusClass;
  time: string;
  size: string;
  mocked: boolean;
}

export function formatRow(r: NetRow): RowView {
  const cls = statusClass(r.status, r.error);
  return {
    method: r.method,
    url: r.url,
    status: r.status === 0 ? (r.error ? "error" : "-") : String(r.status),
    statusClass: cls,
    time: `${r.durationMs} ms`,
    size: formatSize(r.responseBytes),
    mocked: r.mocked === true,
  };
}

/** What a body shows: its text, or a line that says why there is none. */
export function bodyView(b: NetBody): { text: string; note: string } {
  switch (b.kind) {
    case "empty":
      return { text: "", note: "empty" };
    case "binary":
      return { text: "", note: `binary, ${formatSize(b.size)}, not shown` };
    case "large":
      return { text: "", note: `${formatSize(b.size)}, over 1 MB, not shown` };
    case "limit":
      return { text: "", note: `${formatSize(b.size)}, not shown: the size limit for one request was used` };
    default:
      return { text: b.text ?? "", note: b.truncated ? `first part of ${formatSize(b.size)}` : formatSize(b.size) };
  }
}

/** What the list says when it has no rows. */
export function emptyMessage(error: string | null, filtered: boolean): string {
  if (error) return error;
  return filtered ? "No request matches the filter." : "No request was recorded yet. Run a test to see the app's requests.";
}

/**
 * Asks for a refresh every interval while a run is going, and not otherwise. The scheduler is a parameter so a test can drive it.
 * Stopping asks once more, so what the run ended with is shown.
 */
export class RunPoller {
  private timer: unknown = null;
  private readonly refresh: () => void;
  private readonly schedule: (fn: () => void, ms: number) => unknown;
  private readonly cancel: (t: unknown) => void;
  private readonly ms: number;

  constructor(
    refresh: () => void,
    schedule: (fn: () => void, ms: number) => unknown = (fn, ms) => setInterval(fn, ms),
    cancel: (t: unknown) => void = (t) => clearInterval(t as ReturnType<typeof setInterval>),
    ms = 1000,
  ) {
    this.refresh = refresh;
    this.schedule = schedule;
    this.cancel = cancel;
    this.ms = ms;
  }

  get running(): boolean {
    return this.timer !== null;
  }

  start(): void {
    if (this.timer !== null) return;
    this.timer = this.schedule(this.refresh, this.ms);
  }

  stop(): void {
    if (this.timer === null) return;
    this.cancel(this.timer);
    this.timer = null;
    this.refresh();
  }
}

export interface NetFilterArgs {
  method: string;
  status: string;
  text: string;
}

/** What the controller needs from Studio and from the page. The page's side only draws what it is given. */
export interface NetApi {
  list(since: number, filter: NetFilterArgs): Promise<NetList>;
  detail(epoch: string, seq: number): Promise<NetDetail>;
}

export interface NetView {
  filters(): NetFilterArgs;
  renderList(rows: NetRow[], selected: number, emptyText: string): void;
  renderDetail(d: NetDetail | null, message?: string): void;
  markSelected(seq: number): void;
  setInfo(text: string): void;
}

/**
 * The Network tab's behaviour without the page: what is read, when, and which answers may still be shown. Every reset (a new run, a
 * connection or workspace change, a refused read) starts a new generation, and an answer that was asked in an earlier one is dropped,
 * so a slow answer cannot put a row or a detail of the old run or the old app under what is shown now. A detail is also dropped when
 * its epoch is not the list's or its row is no longer the selected one.
 */
export class NetController {
  readonly state = new NetState();
  selected = 0;
  lastError: string | null = null;
  active = false;
  private generation = 0;
  private listCalls = 0;
  private detailCalls = 0;
  private readonly poller: RunPoller;
  private readonly api: NetApi;
  private readonly view: NetView;

  constructor(api: NetApi, view: NetView, schedule?: (fn: () => void, ms: number) => unknown, cancel?: (t: unknown) => void) {
    this.api = api;
    this.view = view;
    this.poller = new RunPoller(
      () => {
        if (this.active) void this.refresh(true);
      },
      schedule,
      cancel,
    );
  }

  get polling(): boolean {
    return this.poller.running;
  }

  private forget(): void {
    this.generation++;
    this.state.clear();
    this.selected = 0;
    this.view.renderDetail(null);
  }

  private draw(): void {
    this.view.renderList(this.state.rows, this.selected, emptyMessage(this.lastError, filteredBy(this.view.filters())));
  }

  setActive(on: boolean): void {
    this.active = on;
    if (on) void this.refresh(false);
  }

  /** A new run starts a new log. */
  runStarted(): void {
    this.forget();
    if (this.active) this.draw();
    this.poller.start();
  }

  runFinished(): void {
    this.poller.stop();
  }

  /** A workspace or connection change makes what is on screen stale. */
  reset(): void {
    this.forget();
    this.draw();
    if (this.active) void this.refresh(false);
  }

  /** A new filter or a click on Refresh reads all of the log; the poll during a run reads what is new. */
  async refresh(incremental: boolean): Promise<void> {
    const wantIncremental = incremental && this.state.since > 0;
    const mine = ++this.listCalls;
    const generation = this.generation;
    let list: NetList;
    try {
      list = await this.api.list(wantIncremental ? this.state.since : 0, this.view.filters());
    } catch (err) {
      if (mine !== this.listCalls || generation !== this.generation) return;
      // Studio said no (for example, there is no app connected): its sentence is the list's empty text.
      this.lastError = String(err instanceof Error ? err.message : err).replace(/^Error: /, "");
      this.forget();
      this.view.setInfo("");
      this.draw();
      return;
    }
    if (mine !== this.listCalls || generation !== this.generation) return;
    this.lastError = null;
    const result = this.state.apply(list, wantIncremental);
    if (result === "reread") {
      this.forget(); // the app restarted: what is on screen is of the old run
      this.draw();
      await this.refresh(false);
      return;
    }
    if (result === "replaced" && this.selected !== 0 && !this.state.rows.some((r) => r.seq === this.selected)) {
      this.selected = 0;
      this.generation++; // a detail that is still on its way was asked for a row that is gone
      this.view.renderDetail(null);
    }
    const count = list.total > list.shown ? `${list.shown} of ${list.total} shown` : this.state.rows.length ? `${this.state.rows.length}` : "";
    this.view.setInfo(list.seeded ? (count ? `seeded (test) · ${count}` : "seeded (test)") : count);
    this.draw();
  }

  async select(seq: number): Promise<void> {
    this.selected = seq;
    this.view.markSelected(seq);
    const mine = ++this.detailCalls;
    const generation = this.generation;
    const epoch = this.state.epoch;
    try {
      const d = await this.api.detail(epoch, seq);
      if (mine !== this.detailCalls || generation !== this.generation || d.epoch !== this.state.epoch || this.selected !== seq || d.seq !== seq) return;
      this.view.renderDetail(d);
    } catch {
      if (mine !== this.detailCalls || generation !== this.generation) return;
      this.view.renderDetail(null, "That request cannot be shown now. Refresh the list.");
    }
  }
}

export function filteredBy(f: NetFilterArgs): boolean {
  return f.method !== "" || f.status !== "" || f.text.trim() !== "";
}
