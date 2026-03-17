import * as vscode from 'vscode';
import { getConvertCommand } from '../probe';
import { runInTerminal } from '../terminal/probeTerminal';
import { getWorkspaceRoot } from '../config';

export function registerConvertCommands(context: vscode.ExtensionContext): void {
  context.subscriptions.push(
    vscode.commands.registerCommand('flutterprobe.convert', convertTests),
  );
}

async function convertTests(): Promise<void> {
  const inputFiles = await vscode.window.showOpenDialog({
    canSelectFiles: true,
    canSelectFolders: true,
    canSelectMany: true,
    title: 'Select test files or folder to convert',
    filters: {
      'Test files': ['yaml', 'yml', 'feature', 'robot', 'js', 'ts', 'py', 'java', 'kt'],
    },
  });
  if (!inputFiles?.length) { return; }

  const format = await vscode.window.showQuickPick(
    [
      { label: 'Auto-detect', value: '' },
      { label: 'Maestro (YAML)', value: 'maestro' },
      { label: 'Gherkin/Cucumber (.feature)', value: 'gherkin' },
      { label: 'Robot Framework (.robot)', value: 'robot' },
      { label: 'Detox (JS/TS)', value: 'detox' },
      { label: 'Appium (Python/Java/JS)', value: 'appium' },
    ],
    { placeHolder: 'Source format' }
  );
  if (!format) { return; }

  const outputDir = await vscode.window.showOpenDialog({
    canSelectFiles: false,
    canSelectFolders: true,
    title: 'Select output directory',
  });

  const args: string[] = [];
  for (const f of inputFiles) {
    args.push(f.fsPath);
  }

  if (format.value) {
    args.push('--from', format.value);
  }
  if (outputDir?.length) {
    args.push('-o', outputDir[0].fsPath);
  }
  args.push('--lint');

  runInTerminal('Probe: Convert', getConvertCommand(), args, getWorkspaceRoot());
}
