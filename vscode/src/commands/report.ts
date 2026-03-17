import * as vscode from 'vscode';
import { getProbeCommand } from '../probe';
import { runInTerminal } from '../terminal/probeTerminal';
import { getWorkspaceRoot } from '../config';

export function registerReportCommands(context: vscode.ExtensionContext): void {
  context.subscriptions.push(
    vscode.commands.registerCommand('flutterprobe.generateReport', generateReport),
    vscode.commands.registerCommand('flutterprobe.openReport', openReport),
  );
}

async function generateReport(): Promise<void> {
  const inputFile = await vscode.window.showOpenDialog({
    canSelectFiles: true,
    canSelectFolders: false,
    filters: { 'JSON Results': ['json'] },
    title: 'Select JSON results file',
  });
  if (!inputFile?.length) { return; }

  const outputFile = await vscode.window.showSaveDialog({
    filters: { 'HTML Report': ['html'] },
    title: 'Save report as',
    defaultUri: vscode.Uri.file(inputFile[0].fsPath.replace('.json', '.html')),
  });
  if (!outputFile) { return; }

  const args = ['report', '--input', inputFile[0].fsPath, '-o', outputFile.fsPath, '--open'];
  runInTerminal('Probe: Report', getProbeCommand(), args, getWorkspaceRoot());
}

async function openReport(): Promise<void> {
  const reportFile = await vscode.window.showOpenDialog({
    canSelectFiles: true,
    canSelectFolders: false,
    filters: { 'HTML Report': ['html'] },
    title: 'Select HTML report',
  });
  if (!reportFile?.length) { return; }

  vscode.env.openExternal(vscode.Uri.file(reportFile[0].fsPath));
}
