import * as vscode from 'vscode';
import { getProbeCommand, listDevices } from '../probe';
import { runInTerminal } from '../terminal/probeTerminal';
import { getSetting, getWorkspaceRoot } from '../config';

export function registerStudioCommands(context: vscode.ExtensionContext): void {
  context.subscriptions.push(
    vscode.commands.registerCommand('flutterprobe.openStudio', openStudio),
  );
}

async function openStudio(): Promise<void> {
  const port = getSetting<number>('studioPort') ?? 9191;

  // Pick device if multiple
  const devices = await listDevices();
  let deviceSerial: string | undefined;
  if (devices.length > 1) {
    const picked = await vscode.window.showQuickPick(
      devices.map(d => ({ label: d.name, description: d.serial, serial: d.serial })),
      { placeHolder: 'Select device for Studio' }
    );
    if (!picked) { return; }
    deviceSerial = picked.serial;
  } else if (devices.length === 1) {
    deviceSerial = devices[0].serial;
  }

  const args = ['studio', '--port', String(port)];
  if (deviceSerial) {
    args.push('--device', deviceSerial);
  }

  runInTerminal('Probe: Studio', getProbeCommand(), args, getWorkspaceRoot());

  // Open in browser after a short delay to let the server start
  setTimeout(() => {
    vscode.env.openExternal(vscode.Uri.parse(`http://127.0.0.1:${port}`));
  }, 2000);
}
