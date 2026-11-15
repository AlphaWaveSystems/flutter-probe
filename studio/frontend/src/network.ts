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
   * Returns true when what was on screen was replaced.
   */
  apply(list: NetList, incremental: boolean): boolean {
    const restarted = list.epoch !== this.epoch && this.epoch !== "";
    if (!incremental || restarted || this.rows.length === 0) {
      this.rows = list.rows.slice(-MAX_ROWS);
      this.epoch = list.epoch;
      this.latest = Math.max(list.latest, this.rows.length ? this.rows[this.rows.length - 1].seq : 0);
      return true;
    }
    const added = list.rows.filter((r) => r.seq > this.latest);
    this.rows = this.rows.concat(added).slice(-MAX_ROWS);
    this.epoch = list.epoch;
    this.latest = Math.max(this.latest, list.latest, added.length ? added[added.length - 1].seq : 0);
    return false;
  }
}

export function formatSize(n: number): string {
  if (!Number.isFinite(n) || n < 0) return "?";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

export type StatusClass = "ok" | "redirect" | "client" | "server" | "error";

export function statusClass(status: number, error?: string): StatusClass {
  if (error || status === 0) return "error";
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
