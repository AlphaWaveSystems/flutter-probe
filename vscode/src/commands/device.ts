import * as vscode from 'vscode';
import { getProbeCommand, listDevices } from '../probe';
import { runInTerminal } from '../terminal/probeTerminal';
import { getWorkspaceRoot } from '../config';

export function registerDeviceCommands(context: vscode.ExtensionContext): void {
  context.subscriptions.push(
    vscode.commands.registerCommand('flutterprobe.listDevices', showDevices),
    vscode.commands.registerCommand('flutterprobe.startDevice', startDevice),
  );
}

async function showDevices(): Promise<void> {
  const devices = await listDevices();
  if (devices.length === 0) {
    vscode.window.showInformationMessage('No devices found. Run "probe device list" for details.');
    runInTerminal('Probe: Devices', getProbeCommand(), ['device', 'list'], getWorkspaceRoot());
    return;
  }

  const items = devices.map(d => ({
    label: `$(device-mobile) ${d.name}`,
    description: `${d.platform} — ${d.serial}`,
    detail: `State: ${d.state}`,
  }));

  await vscode.window.showQuickPick(items, {
    placeHolder: 'Connected devices',
  });
}

async function startDevice(): Promise<void> {
  const platform = await vscode.window.showQuickPick(
    [
      { label: '$(device-mobile) Android Emulator', value: 'android' },
      { label: '$(device-mobile) iOS Simulator', value: 'ios' },
    ],
    { placeHolder: 'Select platform' }
  );
  if (!platform) { return; }

  runInTerminal('Probe: Device', getProbeCommand(), ['device', 'start', '--platform', platform.value], getWorkspaceRoot());
}
