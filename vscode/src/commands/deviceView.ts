import * as vscode from 'vscode';
import * as cp from 'child_process';
import { getProbeCommand } from '../probe';
import { getSetting, getWorkspaceRoot } from '../config';
import { pickDevice } from '../views/devicePicker';
import { generateNonce, hubOriginFromUrl } from '../deviceView/csp';
import { buildWebviewHtml } from '../deviceView/html';
import { HubProcess, HubStartError, SpawnFn } from '../deviceView/hubProcess';
import { HubInfo } from '../deviceView/hubLine';
import { planInsert, buildTapStep } from '../deviceView/insertStep';
import type { SelectorFields } from '../deviceView/selector';
import { clampViewConfig, statusBarText } from '../deviceView/viewConfig';

const LAST_DEVICE_KEY = 'flutterprobe.deviceView.lastDevice';

const nodeSpawn: SpawnFn = (command, args, cwd) =>
  cp.spawn(command, args, { cwd, env: { ...process.env }, stdio: ['pipe', 'pipe', 'pipe'] });

interface Session {
  panel: vscode.WebviewPanel;
  hub: HubProcess;
  deviceId: string;
  disposed: boolean;
}

let current: Session | undefined;
let output: vscode.OutputChannel | undefined;
let statusItem: vscode.StatusBarItem | undefined;
let lastFields: SelectorFields | undefined;

export function registerDeviceViewCommands(context: vscode.ExtensionContext): void {
  output = vscode.window.createOutputChannel('FlutterProbe Device View');
  statusItem = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 49);
  statusItem.command = 'flutterprobe.openDeviceView';
  context.subscriptions.push(
    output,
    statusItem,
    vscode.commands.registerCommand('flutterprobe.openDeviceView', (deviceId?: string) =>
      openDeviceView(context, typeof deviceId === 'string' ? deviceId : undefined)),
    vscode.commands.registerCommand('flutterprobe.insertTapStep', () => insertLastSelector()),
    { dispose: disposeDeviceView },
  );
}

/** Called from deactivate(): kills the hub child. */
export function disposeDeviceView(): void {
  const s = current;
  current = undefined;
  if (s) {
    s.disposed = true;
    s.hub.stop();
    s.panel.dispose();
  }
  statusItem?.hide();
}

function setStatus(state: string, device: string): void {
  if (!statusItem) { return; }
  const s = statusBarText(state, device);
  statusItem.text = s.text;
  statusItem.tooltip = s.tooltip;
  statusItem.show();
}

async function openDeviceView(context: vscode.ExtensionContext, deviceArg?: string): Promise<void> {
  if (vscode.env.remoteName) {
    vscode.window.showWarningMessage(
      `The device view is not available in remote windows (${vscode.env.remoteName}). ` +
      'The hub listens on 127.0.0.1 of the machine running the extension, while the webview runs on your local ' +
      'machine, so it cannot reach the hub. Use the device view from a local VS Code window, or use FlutterProbe Studio.'
    );
    return;
  }

  let deviceId = deviceArg;
  if (!deviceId) {
    const picked = await pickDevice('Select a device for the device view');
    if (!picked) { return; }
    deviceId = picked.serial;
  }
  await context.workspaceState.update(LAST_DEVICE_KEY, deviceId);

  if (current && !current.disposed) {
    if (current.deviceId === deviceId) {
      current.panel.reveal(undefined, true);
      return;
    }
    disposeDeviceView();
  }

  const mediaRoot = vscode.Uri.joinPath(context.extensionUri, 'media');
  const panel = vscode.window.createWebviewPanel(
    'flutterprobe.deviceView',
    `Device View: ${deviceId}`,
    { viewColumn: vscode.ViewColumn.Beside, preserveFocus: true },
    {
      enableScripts: true,
      retainContextWhenHidden: true,
      localResourceRoots: [mediaRoot],
    },
  );

  const hub = new HubProcess({
    spawn: nodeSpawn,
    onStderr: line => output?.appendLine(line),
    onExit: code => {
      output?.appendLine(`probe device view exited (code ${code ?? 'null'}).`);
      setStatus('stopped', deviceId!);
      panel.webview.postMessage({ type: 'hub-exit' });
      vscode.window.showWarningMessage('The FlutterProbe device view hub stopped. Run "FlutterProbe: Open Device View" to restart it.');
    },
  });
  const session: Session = { panel, hub, deviceId, disposed: false };
  current = session;

  panel.onDidDispose(() => {
    session.disposed = true;
    hub.stop();
    if (current === session) { current = undefined; statusItem?.hide(); }
  });

  setStatus('starting', deviceId);
  panel.webview.html = '<!DOCTYPE html><html><body>Starting device view...</body></html>';

  const command = getProbeCommand();
  const args = ['device', 'view', '--device', deviceId, '--json', '--exit-on-stdin-close'];
  output?.appendLine(`$ ${command} ${args.join(' ')}`);

  let info: HubInfo;
  try {
    info = await hub.start(command, args, getWorkspaceRoot());
  } catch (e) {
    output?.appendLine(String((e as Error).message));
    setStatus('error', deviceId);
    const kind = e instanceof HubStartError ? e.kind : 'spawn';
    const detail = kind === 'not-found'
      ? `The probe binary was not found (${command}). Install FlutterProbe or set "flutterprobe.probePath".`
      : kind === 'exited'
        ? `Device "${deviceId}" is unavailable or the hub could not start. See the "FlutterProbe Device View" output for details.`
        : (e as Error).message;
    if (!session.disposed) { panel.dispose(); }
    const choice = await vscode.window.showErrorMessage(detail, 'Show Output');
    if (choice === 'Show Output') { output?.show(true); }
    return;
  }
  if (session.disposed) { hub.stop(); return; }

  const hubOrigin = hubOriginFromUrl(info.url);
  if (!hubOrigin) {
    hub.stop();
    panel.dispose();
    vscode.window.showErrorMessage('The device view hub returned an unexpected address.');
    return;
  }

  const nonce = generateNonce();
  const w = panel.webview;
  const html = buildWebviewHtml({
    nonce,
    cspSource: w.cspSource,
    hubOrigin,
    bundleUri: w.asWebviewUri(vscode.Uri.joinPath(mediaRoot, 'device-view.js')).toString(),
    hostUri: w.asWebviewUri(vscode.Uri.joinPath(mediaRoot, 'device-view-host.js')).toString(),
  });
  w.html = html;

  const sendInit = () => {
    const cfg = clampViewConfig(getSetting<number>('deviceView.fps'), getSetting<number>('deviceView.quality'));
    // The token is posted to the webview only; it is not stored or logged.
    w.postMessage({ type: 'init', url: info.url, token: info.token, device: info.device || deviceId, config: cfg });
    w.postMessage({ type: 'visibility', visible: panel.visible });
  };

  w.onDidReceiveMessage((msg: unknown) => handleWebviewMessage(msg, deviceId!, sendInit));
  panel.onDidChangeViewState(() => {
    w.postMessage({ type: 'visibility', visible: panel.visible });
  });
  setStatus('connected', deviceId);
}

function handleWebviewMessage(msg: unknown, deviceId: string, sendInit: () => void): void {
  if (typeof msg !== 'object' || msg === null) { return; }
  const m = msg as { type?: unknown; node?: unknown; state?: unknown };
  switch (m.type) {
    case 'ready':
      sendInit();
      break;
    case 'status':
      if (typeof m.state === 'string') { setStatus(m.state, deviceId); }
      break;
    case 'taken':
      void offerReconnect(sendInit);
      break;
    case 'select': {
      // The webview sends the element's fields only; the step is built here.
      const fields = toSelectorFields(m.node);
      const r = buildTapStep(fields);
      if (!r.ok) {
        lastFields = undefined;
        vscode.window.showWarningMessage(r.error);
        break;
      }
      lastFields = fields;
      void offerInsert(r.selector, r.step);
      break;
    }
  }
}

function toSelectorFields(node: unknown): SelectorFields {
  const n = (typeof node === 'object' && node !== null ? node : {}) as Record<string, unknown>;
  const str = (v: unknown): string | null => (typeof v === 'string' ? v : null);
  return { key: str(n.key), text: str(n.text), label: str(n.label), type: str(n.type) };
}

async function offerReconnect(sendInit: () => void): Promise<void> {
  const choice = await vscode.window.showWarningMessage(
    'The device view was taken over by another window that uses the same hub. It will not reconnect on its own.',
    'Reconnect',
  );
  if (choice === 'Reconnect') { sendInit(); }
}

async function offerInsert(selector: string, step: string): Promise<void> {
  const shown = selector.length > 60 ? `${selector.slice(0, 57)}...` : selector;
  const choice = await vscode.window.showInformationMessage(
    `Selected ${shown}`, 'Insert tap step', 'Copy',
  );
  if (choice === 'Insert tap step') {
    await insertLastSelector();
  } else if (choice === 'Copy') {
    await vscode.env.clipboard.writeText(step);
  }
}

async function insertLastSelector(): Promise<void> {
  if (!lastFields) {
    vscode.window.showInformationMessage('Alt+click an element in the device view first.');
    return;
  }
  const isProbe = (e: vscode.TextEditor) =>
    e.document.languageId === 'probescript' || e.document.fileName.endsWith('.probe');
  const active = vscode.window.activeTextEditor;
  const editor = active && isProbe(active) ? active : vscode.window.visibleTextEditors.find(isProbe);
  const built = buildTapStep(lastFields);
  const step = built.ok ? built.step : undefined;
  if (!editor) {
    if (step) { await vscode.env.clipboard.writeText(step); }
    vscode.window.showInformationMessage('No .probe editor is open; the tap step was copied to the clipboard.');
    return;
  }
  const line = editor.document.lineAt(editor.selection.active.line);
  const plan = planInsert(line.text, lastFields);
  if (!plan) { return; }
  await editor.edit(b => b.insert(line.range.end, plan.text));
}
