import * as vscode from 'vscode';
import { getProbeCommand } from '../probe';
import { runInTerminal } from '../terminal/probeTerminal';
import { getWorkspaceRoot } from '../config';

export function registerLintCommands(context: vscode.ExtensionContext): void {
  context.subscriptions.push(
    vscode.commands.registerCommand('flutterprobe.lintFile', lintCurrentFile),
    vscode.commands.registerCommand('flutterprobe.lintWorkspace', lintWorkspace),
  );
}

function lintCurrentFile(): void {
  const editor = vscode.window.activeTextEditor;
  if (!editor) {
    vscode.window.showWarningMessage('No active editor.');
    return;
  }

  const filePath = editor.document.uri.fsPath;
  if (!filePath.endsWith('.probe')) {
    vscode.window.showWarningMessage('Not a .probe file.');
    return;
  }

  runInTerminal('Probe: Lint', getProbeCommand(), ['lint', filePath], getWorkspaceRoot());
}

function lintWorkspace(): void {
  const ws = getWorkspaceRoot();
  if (!ws) {
    vscode.window.showWarningMessage('No workspace folder open.');
    return;
  }

  runInTerminal('Probe: Lint', getProbeCommand(), ['lint', ws], ws);
}
