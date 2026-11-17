import * as vscode from 'vscode';
import { buildTestRun, getProbeCommand } from '../probe';
import { runInTerminal } from '../terminal/probeTerminal';
import { getWorkspaceRoot } from '../config';

export function registerTestCommands(context: vscode.ExtensionContext): void {
  context.subscriptions.push(
    vscode.commands.registerCommand('flutterprobe.runTest', runTestAtCursor),
    vscode.commands.registerCommand('flutterprobe.runFile', runCurrentFile),
    vscode.commands.registerCommand('flutterprobe.runByTag', runByTag),
    vscode.commands.registerCommand('flutterprobe.runWithOptions', runWithOptions),
  );
}

// probe test has no per-test filter in this version, so this runs the file that holds the cursor.
async function runTestAtCursor(): Promise<void> {
  const editor = vscode.window.activeTextEditor;
  if (!editor || editor.document.languageId !== 'probescript') {
    vscode.window.showWarningMessage('Open a .probe file to run a test.');
    return;
  }

  const run = buildTestRun(editor.document.uri.fsPath);
  runInTerminal('Probe: Test', getProbeCommand(), run.args, getWorkspaceRoot(), run.env);
}

async function runCurrentFile(): Promise<void> {
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

  const run = buildTestRun(filePath);
  runInTerminal('Probe: Test', getProbeCommand(), run.args, getWorkspaceRoot(), run.env);
}

async function runByTag(): Promise<void> {
  const tag = await vscode.window.showInputBox({
    prompt: 'Enter tag to run (e.g., smoke, critical)',
    placeHolder: 'smoke',
  });
  if (!tag) { return; }

  const ws = getWorkspaceRoot();
  if (!ws) {
    vscode.window.showWarningMessage('No workspace folder open.');
    return;
  }

  const run = buildTestRun(ws, { tags: [tag] });
  runInTerminal('Probe: Test', getProbeCommand(), run.args, ws, run.env);
}

async function runWithOptions(): Promise<void> {
  const editor = vscode.window.activeTextEditor;
  const filePath = editor?.document.uri.fsPath ?? getWorkspaceRoot();
  if (!filePath) {
    vscode.window.showWarningMessage('No workspace folder open.');
    return;
  }

  const device = await vscode.window.showInputBox({
    prompt: 'Device serial (leave empty for default)',
    placeHolder: 'emulator-5554',
  });

  const timeout = await vscode.window.showInputBox({
    prompt: 'Timeout',
    placeHolder: '30s',
    value: '30s',
  });

  const format = await vscode.window.showQuickPick(
    ['terminal', 'json', 'junit'],
    { placeHolder: 'Output format' }
  );

  const videoChoice = await vscode.window.showQuickPick(
    ['No', 'Yes'],
    { placeHolder: 'Record video?' }
  );

  const target = filePath.endsWith('.probe') ? filePath : filePath;
  const run = buildTestRun(target, {
    device: device || undefined,
    timeout: timeout || undefined,
    format: format || undefined,
    video: videoChoice === 'Yes',
  });

  runInTerminal('Probe: Test', getProbeCommand(), run.args, getWorkspaceRoot(), run.env);
}
