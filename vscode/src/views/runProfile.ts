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
          cloudProvider: msg.cloudProvider || undefined,
          cloudDevice: msg.cloudDevice || undefined,
          cloudApp: msg.cloudApp || undefined,
          cloudKey: msg.cloudKey || undefined,
          cloudSecret: msg.cloudSecret || undefined,
          relayUrl: msg.relayUrl || undefined,
          relayToken: msg.relayToken || undefined,
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
    .section { margin-top: 20px; padding-top: 16px; border-top: 1px solid var(--vscode-panel-border); }
    .section h3 { margin: 0 0 12px; }
    .cloud-fields { display: none; }
    .cloud-fields.visible { display: block; }
    .hint { font-size: 11px; opacity: 0.7; margin-top: -4px; margin-bottom: 8px; }
  </style>
</head>
<body>
  <h2>Run Test Options</h2>

  <label>File</label>
  <input id="file" value="${filePath}" />

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

  <div class="section">
    <h3>Target</h3>
    <label>Cloud Provider</label>
    <select id="cloudProvider" onchange="toggleCloud()">
      <option value="">Local Device</option>
      <option value="browserstack">BrowserStack</option>
      <option value="saucelabs">Sauce Labs</option>
      <option value="lambdatest">LambdaTest</option>
      <option value="aws">AWS Device Farm</option>
      <option value="firebase">Firebase Test Lab</option>
    </select>

    <div id="localFields">
      <label>Device Serial</label>
      <input id="device" placeholder="emulator-5554 (leave empty for default)" />
    </div>

    <div id="cloudFields" class="cloud-fields">
      <label>Cloud Device</label>
      <input id="cloudDevice" placeholder="Google Pixel 7-13.0" />
      <p class="hint">Format: "Device Name-OS Version" (e.g., "iPhone 14-16.0")</p>

      <label>App Binary</label>
      <input id="cloudApp" placeholder="/path/to/app.apk or /path/to/app.ipa" />

      <label>Username / Access Key ID</label>
      <input id="cloudKey" placeholder="From your provider account" />

      <label>Access Key / Secret</label>
      <input id="cloudSecret" type="password" placeholder="From your provider account" />

      <label>Relay URL</label>
      <input id="relayUrl" placeholder="wss://relay.flutterprobe.com (required for cloud E2E)" />

      <label>Relay Token</label>
      <input id="relayToken" placeholder="Relay authentication token" />
    </div>
  </div>

  <button onclick="run()">Run</button>

  <script>
    const vscode = acquireVsCodeApi();

    function toggleCloud() {
      const provider = document.getElementById('cloudProvider').value;
      const cloudFields = document.getElementById('cloudFields');
      const localFields = document.getElementById('localFields');
      if (provider) {
        cloudFields.classList.add('visible');
        localFields.style.display = 'none';
      } else {
        cloudFields.classList.remove('visible');
        localFields.style.display = 'block';
      }
    }

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
        cloudProvider: document.getElementById('cloudProvider').value,
        cloudDevice: document.getElementById('cloudDevice').value,
        cloudApp: document.getElementById('cloudApp').value,
        cloudKey: document.getElementById('cloudKey').value,
        cloudSecret: document.getElementById('cloudSecret').value,
        relayUrl: document.getElementById('relayUrl').value,
        relayToken: document.getElementById('relayToken').value,
      });
    }
  </script>
</body>
</html>`;
  }
}
