import { buildCsp } from './csp';

export interface HtmlInput {
  nonce: string;
  cspSource: string;
  hubOrigin: string;
  bundleUri: string;
  hostUri: string;
}

/** Webview document. Contains no token and no inline script. */
export function buildWebviewHtml(i: HtmlInput): string {
  const csp = buildCsp(i.nonce, i.cspSource, i.hubOrigin);
  return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta http-equiv="Content-Security-Policy" content="${csp}">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Device View</title>
<style nonce="${i.nonce}">
  html, body { height: 100%; margin: 0; padding: 0; overflow: hidden;
    background: var(--vscode-editor-background); color: var(--vscode-foreground);
    font-family: var(--vscode-font-family); }
  #root { position: absolute; inset: 0; }
</style>
</head>
<body>
<div id="root"></div>
<script nonce="${i.nonce}" src="${i.bundleUri}"></script>
<script nonce="${i.nonce}" src="${i.hostUri}"></script>
</body>
</html>`;
}
