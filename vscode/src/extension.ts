import * as vscode from 'vscode';
import { registerTestCommands } from './commands/test';
import { registerLintCommands } from './commands/lint';
import { registerRecordCommands } from './commands/record';
import { registerDeviceCommands } from './commands/device';
import { registerReportCommands } from './commands/report';
import { registerStudioCommands } from './commands/studio';
import { registerInitCommands } from './commands/init';
import { registerConvertCommands } from './commands/convert';
import { registerDeviceViewCommands, disposeDeviceView } from './commands/deviceView';
import { SessionManagerProvider } from './providers/sessionManager';
import { TestExplorerProvider } from './providers/testExplorer';
import { registerTestingController } from './testing/testController';
import { ProbeCodeLensProvider } from './providers/codelens';
import { DiagnosticsProvider } from './providers/diagnostics';
import { ProbeCompletionProvider } from './providers/completions';
import { RunProfilePanel } from './views/runProfile';
import { buildTestRun, getProbeCommand } from './probe';
import { runInTerminal } from './terminal/probeTerminal';
import { getWorkspaceRoot } from './config';
import { stopAllTerminals } from './terminal/probeTerminal';
import { Session } from './types';
import { SECRET_SETTINGS, staleWorkspaceSettings } from './secretArgs';
import { TOOL_SETTINGS } from './toolPath';
import * as fs from 'fs';
import * as path from 'path';

// noticeStaleWorkspaceSettings says once per workspace that machine-scoped settings in its .vscode/settings.json are ignored. VS Code
// drops them without a word; a user who had them there would otherwise see cloud runs fail with no reason.
function noticeStaleWorkspaceSettings(context: vscode.ExtensionContext): void {
  const ws = getWorkspaceRoot();
  if (!ws || context.workspaceState.get<boolean>('flutterprobe.staleSettingsNoticed')) { return; }
  let text = '';
  try {
    text = fs.readFileSync(path.join(ws, '.vscode', 'settings.json'), 'utf8');
  } catch {
    return;
  }
  const stale = staleWorkspaceSettings(text, [...SECRET_SETTINGS, ...TOOL_SETTINGS]);
  if (stale.length === 0) { return; }
  void context.workspaceState.update('flutterprobe.staleSettingsNoticed', true);
  void vscode.window.showWarningMessage(
    `FlutterProbe ignores ${stale.map(k => 'flutterprobe.' + k).join(', ')} in this workspace's .vscode/settings.json: ` +
    'they can be set only in your user (or remote) settings now: the secrets, the relay URL and the cloud provider that decide where they are sent, ' +
    'and the paths of the binaries the extension runs cannot come from a workspace. Move them to your user settings.',
  );
}

export function activate(context: vscode.ExtensionContext): void {
  noticeStaleWorkspaceSettings(context);

  // Register all command groups
  registerTestCommands(context);
  registerLintCommands(context);
  registerRecordCommands(context);
  registerDeviceCommands(context);
  registerReportCommands(context);
  registerStudioCommands(context);
  registerInitCommands(context);
  registerConvertCommands(context);
  registerDeviceViewCommands(context);

  // Session Manager sidebar
  const sessionManager = new SessionManagerProvider(context);
  vscode.window.registerTreeDataProvider('flutterprobe.sessions', sessionManager);

  // Test Explorer sidebar
  const testExplorer = new TestExplorerProvider(context);
  vscode.window.registerTreeDataProvider('flutterprobe.tests', testExplorer);

  // Native Testing view (VS Code Testing API), over the same discovery of .probe tests
  registerTestingController(context);

  // CodeLens
  const codeLensProvider = new ProbeCodeLensProvider(sessionManager);
  context.subscriptions.push(
    vscode.languages.registerCodeLensProvider(
      { language: 'probescript' },
      codeLensProvider
    )
  );

  // Diagnostics (lint-on-save)
  new DiagnosticsProvider(context);

  // Completions
  const completionProvider = new ProbeCompletionProvider();
  context.subscriptions.push(
    vscode.languages.registerCompletionItemProvider(
      { language: 'probescript' },
      completionProvider,
      '"', '@', ' '
    )
  );

  // Run Profile webview
  new RunProfilePanel(context);

  // Additional commands that need the session manager
  context.subscriptions.push(
    vscode.commands.registerCommand('flutterprobe.runTestOnDevice', runTestOnDevice),
    vscode.commands.registerCommand('flutterprobe.runTestVerbose', runTestVerbose),
  );

  // Status bar — active device
  const deviceStatusItem = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 50);
  deviceStatusItem.text = '$(device-mobile) FlutterProbe';
  deviceStatusItem.tooltip = 'FlutterProbe — click to select device';
  deviceStatusItem.command = 'flutterprobe.listDevices';
  deviceStatusItem.show();
  context.subscriptions.push(deviceStatusItem);
}

// probe test has no per-test filter in this version: these run the file that holds the test.
function runTestOnDevice(filePath: string, session: Session): void {
  const run = buildTestRun(filePath, {
    device: session.device.serial,
    port: session.port,
    config: session.config,
    autoConfirm: true,
  });
  runInTerminal(`Probe: ${session.name}`, getProbeCommand(), run.args, getWorkspaceRoot(), run.env);
}

function runTestVerbose(filePath: string): void {
  const run = buildTestRun(filePath, { verbose: true });
  runInTerminal('Probe: Test', getProbeCommand(), run.args, getWorkspaceRoot(), run.env);
}

// deactivate returns when the tools the extension started have ended (stopAllTerminals), so none outlives the extension host.
export function deactivate(): Promise<void> {
  disposeDeviceView();
  return stopAllTerminals();
}
