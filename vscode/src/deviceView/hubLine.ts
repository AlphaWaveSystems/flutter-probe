import { hubOriginFromUrl } from './csp';

export interface HubInfo {
  url: string;
  token: string;
  device: string;
}

/**
 * Parse one stdout line of `probe device view --device <id> --json`.
 * Returns undefined for anything that is not the expected JSON object
 * (garbage, partial lines, non-loopback URLs, missing token).
 */
export function parseHubLine(line: string): HubInfo | undefined {
  const trimmed = line.trim();
  if (!trimmed.startsWith('{')) { return undefined; }
  let obj: unknown;
  try {
    obj = JSON.parse(trimmed);
  } catch {
    return undefined;
  }
  if (typeof obj !== 'object' || obj === null) { return undefined; }
  const o = obj as Record<string, unknown>;
  if (typeof o.url !== 'string' || typeof o.token !== 'string' || !o.token) { return undefined; }
  if (!/^[A-Za-z0-9_\-.~]+$/.test(o.token)) { return undefined; }
  if (!hubOriginFromUrl(o.url)) { return undefined; }
  return { url: o.url, token: o.token, device: typeof o.device === 'string' ? o.device : '' };
}

/** Accumulates chunks and yields complete lines; a trailing partial line is held back. */
export class LineBuffer {
  private pending = '';

  push(chunk: string): string[] {
    this.pending += chunk;
    const parts = this.pending.split('\n');
    this.pending = parts.pop() ?? '';
    return parts.map(p => p.replace(/\r$/, ''));
  }

  /** Flush a final unterminated line (process ended). */
  flush(): string[] {
    const rest = this.pending;
    this.pending = '';
    return rest ? [rest.replace(/\r$/, '')] : [];
  }
}
