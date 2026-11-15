// The Send request panel's decisions, as plain functions so they can be tested without a browser. The request is typed by a person and
// sent by Studio's Go code (SendRequest), which checks it again and redacts the answer; nothing here stores what was typed or shown.

export interface SendHeader {
  name: string;
  value: string;
}

export interface HeaderParse {
  headers: SendHeader[];
  error: string; // "" when every line was understood
}

export const MAX_SEND_HEADERS = 50;

/** Reads "Name: value" lines. A blank line is skipped. A line with no colon, or no name, is an error that says which line. */
export function parseHeaders(text: string): HeaderParse {
  const headers: SendHeader[] = [];
  const lines = text.split(/\r?\n/);
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (line.trim() === "") continue;
    const colon = line.indexOf(":");
    const name = colon < 0 ? "" : line.slice(0, colon).trim();
    if (colon < 0 || name === "") return { headers: [], error: `Line ${i + 1} is not "Name: value".` };
    if (headers.length >= MAX_SEND_HEADERS) return { headers: [], error: `At most ${MAX_SEND_HEADERS} headers.` };
    headers.push({ name, value: line.slice(colon + 1).trim() });
  }
  return { headers, error: "" };
}

export interface TargetInfo {
  host: string;
  label?: string; // the name as typed, when it is written in unicode (host is its ASCII form)
  port: string;
  key: string;
  allowed: boolean;
  addresses: string[];
  classes: string[];
  local: boolean;
  refused?: string;
}

/** What the confirmation says. A host on this machine or its network gets the stronger warning. */
export function allowText(t: TargetInfo): { title: string; lines: string[]; warning: string } {
  const where = t.addresses.length ? t.addresses.map((a, i) => `${a} (${t.classes[i] ?? "?"})`).join(", ") : "an address that could not be looked up";
  return {
    title: `Send requests to ${t.key}?`,
    lines: [...(t.label ? [`Written as ${t.label}; Studio uses ${t.host}.`] : []), `The name leads to ${where}.`, "Requests are sent from this machine. Studio asks again for another host or port, and when a redirect leaves this one."],
    warning: t.local ? "This address is on this machine or your own network. A request can reach services that are not meant to be called from here." : "",
  };
}

export interface Hop {
  from: string;
  to: string;
  status: number;
}

export interface ResultSummary {
  line: string;
  hops: string[];
  stopped: string;
}

export interface SendResultLike {
  status: number;
  durationMs: number;
  remoteClass: string;
  hops: Hop[];
  stopped?: Hop | null;
  stoppedHost?: string;
  truncated?: boolean;
}

/** The lines above the headers of an answer. */
export function summarize(r: SendResultLike): ResultSummary {
  const where = r.remoteClass ? ` · from ${r.remoteClass}` : "";
  const cut = r.truncated ? " · the body is longer than 1 MB and was cut" : "";
  return {
    line: `${r.status} · ${r.durationMs} ms${where}${cut}`,
    hops: (r.hops ?? []).map((h) => `${h.status} → ${h.to}`),
    stopped: r.stopped ? `A redirect (${r.stopped.status}) to ${r.stoppedHost || r.stopped.to} was not followed. Send a new request to follow it: Studio will ask before it calls that host.` : "",
  };
}

/** What the status line says after a step was inserted. */
export function savedSummary(fill: string[], headerLines: number): string {
  const set = fill.length ? ` Set ${fill.length === 1 ? "this environment variable" : "these environment variables"} before you run it: ${fill.join(", ")}.` : "";
  const headers = headerLines > 0 ? ` ${headerLines === 1 ? "The header was" : `${headerLines} headers were`} not saved.` : "";
  return `Inserted into the open file.${set}${headers} Save the file to keep it.`;
}

/** The error text for the status line: Studio's sentence, without the wrapper the bridge puts around it. */
export function errorText(err: unknown): string {
  return String(err).replace(/^Error:\s*/, "");
}
