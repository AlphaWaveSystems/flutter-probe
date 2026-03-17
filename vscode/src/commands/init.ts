import * as vscode from 'vscode';
import { getProbeCommand } from '../probe';
import { runInTerminal } from '../terminal/probeTerminal';
import { getWorkspaceRoot } from '../config';

export function registerInitCommands(context: vscode.ExtensionContext): void {
  context.subscriptions.push(
    vscode.commands.registerCommand('flutterprobe.init', initProject),
  );
}

async function initProject(): Promise<void> {
  const ws = getWorkspaceRoot();
  if (!ws) {
    vscode.window.showWarningMessage('No workspace folder open.');
    return;
  }

  runInTerminal('Probe: Init', getProbeCommand(), ['init'], ws);
}
