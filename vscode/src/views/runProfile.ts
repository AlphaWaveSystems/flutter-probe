import * as vscode from 'vscode';
import { buildTestArgs, getProbeCommand } from '../probe';
import { runInTerminal } from '../terminal/probeTerminal';
import { getWorkspaceRoot } from '../config';

export class RunProfilePanel {
  private panel: vscode.WebviewPanel | undefined;

  constructor(private context: vscode.ExtensionContext) {
    context.subscriptions.push(
      vscode.commands.registerCommand('flutterprobe.runProfile', () => this.show()),
    );
  }

  show(): void {
    const editor = vscode.window.activeTextEditor;
    const filePath = editor?.document.uri.fsPath ?? '';

    if (this.panel) {
      this.panel.reveal();
      return;
    }

    this.panel = vscode.window.createWebviewPanel(
      'flutterprobe.runProfile',
      'FlutterProbe: Run Options',
      vscode.ViewColumn.Beside,
      { enableScripts: true },
    );

    this.panel.webview.html = this.getHtml(filePath);

    this.panel.webview.onDidReceiveMessage((msg) => {
      if (msg.command === 'run') {
        const args = buildTestArgs(msg.file || filePath, {
          device: msg.device || undefined,
          timeout: msg.timeout || undefined,
          format: msg.format || undefined,
          video: msg.video,
          autoConfirm: msg.autoConfirm,
          verbose: msg.verbose,
        });
        runInTerminal('Probe: Test', getProbeCommand(), args, getWorkspaceRoot());
        this.panel?.dispose();
      }
    });

    this.panel.onDidDispose(() => { this.panel = undefined; });
  }

  private getHtml(filePath: string): string {
    return `<!DOCTYPE html>
<html>
<head>
  <style>
    body { font-family: var(--vscode-font-family); padding: 16px; color: var(--vscode-foreground); }
    label { display: block; margin: 8px 0 4px; font-weight: bold; }
    input, select { width: 100%; padding: 6px; margin-bottom: 8px; background: var(--vscode-input-background); color: var(--vscode-input-foreground); border: 1px solid var(--vscode-input-border); }
    input[type="checkbox"] { width: auto; margin-right: 8px; }
    button { padding: 8px 24px; margin-top: 16px; background: var(--vscode-button-background); color: var(--vscode-button-foreground); border: none; cursor: pointer; }
    button:hover { background: var(--vscode-button-hoverBackground); }
    .row { display: flex; align-items: center; }
  </style>
</head>
<body>
  <h2>Run Test Options</h2>

  <label>File</label>
  <input id="file" value="${filePath}" />

  <label>Device Serial</label>
  <input id="device" placeholder="emulator-5554 (leave empty for default)" />

  <label>Timeout</label>
  <input id="timeout" value="30s" />

  <label>Output Format</label>
  <select id="format">
    <option value="terminal">Terminal</option>
    <option value="json">JSON</option>
    <option value="junit">JUnit</option>
  </select>

  <div class="row"><input type="checkbox" id="video" /><label for="video">Record Video</label></div>
  <div class="row"><input type="checkbox" id="autoConfirm" /><label for="autoConfirm">Auto-confirm (-y)</label></div>
  <div class="row"><input type="checkbox" id="verbose" checked /><label for="verbose">Verbose (-v)</label></div>

  <button onclick="run()">Run</button>

  <script>
    const vscode = acquireVsCodeApi();
    function run() {
      vscode.postMessage({
        command: 'run',
        file: document.getElementById('file').value,
        device: document.getElementById('device').value,
        timeout: document.getElementById('timeout').value,
        format: document.getElementById('format').value,
        video: document.getElementById('video').checked,
        autoConfirm: document.getElementById('autoConfirm').checked,
        verbose: document.getElementById('verbose').checked,
      });
    }
  </script>
</body>
</html>`;
  }
}
