import * as crypto from 'crypto';

/** Random nonce for CSP script/style tags (128 bits, base64url). */
export function generateNonce(): string {
  return crypto.randomBytes(16).toString('base64url');
}

/**
 * Derive the exact WebSocket origin (scheme + host + port, no path) the
 * webview may connect to. Only the loopback hub is accepted.
 * Returns undefined when the URL is not ws://127.0.0.1:<port>/...
 */
export function hubOriginFromUrl(url: string): string | undefined {
  const m = /^ws:\/\/127\.0\.0\.1:(\d{1,5})(\/|$)/.exec(url);
  if (!m) { return undefined; }
  const port = Number(m[1]);
  if (port < 1 || port > 65535) { return undefined; }
  return `ws://127.0.0.1:${port}`;
}

/** Strict Content-Security-Policy for the device view webview. */
export function buildCsp(nonce: string, cspSource: string, hubOrigin: string): string {
  return [
    `default-src 'none'`,
    `script-src 'nonce-${nonce}' ${cspSource}`,
    `style-src ${cspSource} 'nonce-${nonce}'`,
    `connect-src ${hubOrigin}`,
    `img-src data: blob: ${cspSource}`,
    `base-uri 'none'`,
    `form-action 'none'`,
  ].join('; ');
}
