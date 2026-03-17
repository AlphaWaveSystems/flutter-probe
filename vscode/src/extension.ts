import * as vscode from 'vscode';
import { registerTestCommands } from './commands/test';
import { registerLintCommands } from './commands/lint';
import { registerRecordCommands } from './commands/record';
import { registerDeviceCommands } from './commands/device';
import { registerReportCommands } from './commands/report';
import { registerStudioCommands } from './commands/studio';
import { registerInitCommands } from './commands/init';
import { registerConvertCommands } from './commands/convert';
import { SessionManagerProvider } from './providers/sessionManager';
import { TestExplorerProvider } from './providers/testExplorer';
import { ProbeCodeLensProvider } from './providers/codelens';
import { DiagnosticsProvider } from './providers/diagnostics';
import { ProbeCompletionProvider } from './providers/completions';
import { RunProfilePanel } from './views/runProfile';
import { buildTestArgs, getProbeCommand } from './probe';
import { runInTerminal } from './terminal/probeTerminal';
import { getWorkspaceRoot } from './config';
import { disposeAllTerminals } from './terminal/probeTerminal';
import { Session } from './types';

export function activate(context: vscode.ExtensionContext): void {
  // Register all command groups
  registerTestCommands(context);
  registerLintCommands(context);
  registerRecordCommands(context);
  registerDeviceCommands(context);
  registerReportCommands(context);
  registerStudioCommands(context);
  registerInitCommands(context);
  registerConvertCommands(context);

  // Session Manager sidebar
  const sessionManager = new SessionManagerProvider(context);
  vscode.window.registerTreeDataProvider('flutterprobe.sessions', sessionManager);

  // Test Explorer sidebar
  const testExplorer = new TestExplorerProvider(context);
  vscode.window.registerTreeDataProvider('flutterprobe.tests', testExplorer);

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

function runTestOnDevice(filePath: string, testName: string, session: Session): void {
  const args = buildTestArgs(filePath, {
    device: session.device.serial,
    port: session.port,
    config: session.config,
    autoConfirm: true,
  });
  args.push('--name', testName);
  runInTerminal(`Probe: ${session.name}`, getProbeCommand(), args, getWorkspaceRoot());
}

function runTestVerbose(filePath: string, testName: string): void {
  const args = buildTestArgs(filePath, { verbose: true });
  args.push('--name', testName);
  runInTerminal('Probe: Test', getProbeCommand(), args, getWorkspaceRoot());
}

export function deactivate(): void {
  disposeAllTerminals();
}
