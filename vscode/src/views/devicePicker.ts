import * as vscode from 'vscode';
import { listDevices } from '../probe';
import { Device } from '../types';

export async function pickDevice(prompt?: string): Promise<Device | undefined> {
  const devices = await listDevices();

  if (devices.length === 0) {
    vscode.window.showWarningMessage('No devices found. Start a device or emulator first.');
    return undefined;
  }

  if (devices.length === 1) {
    return devices[0];
  }

  const picked = await vscode.window.showQuickPick(
    devices.map(d => ({
      label: `$(device-mobile) ${d.name}`,
      description: `${d.platform} — ${d.serial}`,
      detail: `State: ${d.state}`,
      device: d,
    })),
    { placeHolder: prompt ?? 'Select a device' }
  );

  return picked?.device;
}
