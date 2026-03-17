import * as vscode from 'vscode';
import * as path from 'path';
import { getProbeCommand, listDevices } from '../probe';
import { runInTerminal, disposeTerminal } from '../terminal/probeTerminal';
import { getWorkspaceRoot } from '../config';

let recordingStatusItem: vscode.StatusBarItem | undefined;

export function registerRecordCommands(context: vscode.ExtensionContext): void {
  context.subscriptions.push(
    vscode.commands.registerCommand('flutterprobe.record', startRecording),
    vscode.commands.registerCommand('flutterprobe.stopRecord', stopRecording),
  );
}

async function startRecording(): Promise<void> {
  const ws = getWorkspaceRoot();
  if (!ws) {
    vscode.window.showWarningMessage('No workspace folder open.');
    return;
  }

  // Pick device
  const devices = await listDevices();
  let deviceSerial: string | undefined;
  if (devices.length > 1) {
    const picked = await vscode.window.showQuickPick(
      devices.map(d => ({ label: d.name, description: `${d.platform} (${d.serial})`, serial: d.serial })),
      { placeHolder: 'Select device to record on' }
    );
    if (!picked) { return; }
    deviceSerial = picked.serial;
  } else if (devices.length === 1) {
    deviceSerial = devices[0].serial;
  }

  // Output file
  const outputName = await vscode.window.showInputBox({
    prompt: 'Output file name',
    placeHolder: 'recorded-test.probe',
    value: 'recorded-test.probe',
  });
  if (!outputName) { return; }

  const outputPath = path.join(ws, outputName);
  const args = ['record', '-o', outputPath, '--timeout', '5m'];
  if (deviceSerial) {
    args.push('--device', deviceSerial);
  }

  runInTerminal('Probe: Record', getProbeCommand(), args, ws);

  // Show recording status
  recordingStatusItem = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 100);
  recordingStatusItem.text = '$(record) Recording...';
  recordingStatusItem.tooltip = 'Click to stop recording';
  recordingStatusItem.command = 'flutterprobe.stopRecord';
  recordingStatusItem.backgroundColor = new vscode.ThemeColor('statusBarItem.errorBackground');
  recordingStatusItem.show();
}

function stopRecording(): void {
  // Send Ctrl+C to the recording terminal
  disposeTerminal('Probe: Record');

  if (recordingStatusItem) {
    recordingStatusItem.dispose();
    recordingStatusItem = undefined;
  }

  vscode.window.showInformationMessage('Recording stopped.');
}
