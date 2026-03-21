import * as cp from 'child_process';
import * as vscode from 'vscode';
import { getProbePath, getConvertPath, getSetting, getWorkspaceRoot } from './config';
import { CommandResult, Device, LintResult } from './types';

export function runCommand(
  command: string,
  args: string[],
  cwd?: string
): Promise<CommandResult> {
  return new Promise((resolve) => {
    const proc = cp.spawn(command, args, {
      cwd: cwd ?? getWorkspaceRoot(),
      env: { ...process.env },
    });

    let stdout = '';
    let stderr = '';

    proc.stdout.on('data', (data: Buffer) => { stdout += data.toString(); });
    proc.stderr.on('data', (data: Buffer) => { stderr += data.toString(); });

    proc.on('close', (code) => {
      resolve({ stdout, stderr, code: code ?? 1 });
    });

    proc.on('error', (err) => {
      resolve({ stdout, stderr: err.message, code: 1 });
    });
  });
}

export async function listDevices(): Promise<Device[]> {
  const probePath = getProbePath();
  const result = await runCommand(probePath, ['device', 'list']);
  if (result.code !== 0) {
    return [];
  }

  // Parse table output:
  //   SERIAL                 STATE        NAME
  //   ------                 -----        ----
  //   emulator-5554          device       sdk gphone64 x86 64
  //   A1B2C3D4-...           booted       iPhone 16 Pro
  const devices: Device[] = [];
  const lines = result.stdout.split('\n');

  for (const line of lines) {
    const trimmed = line.trim();
    // Skip header, separator, and empty lines
    if (!trimmed || trimmed.startsWith('SERIAL') || trimmed.startsWith('---')) {
      continue;
    }

    // Split into: serial, state, name (columns are whitespace-separated)
    const match = trimmed.match(/^(\S+)\s+(device|booted|offline|shutdown|unknown)\s+(.+)$/i);
    if (!match) { continue; }

    const serial = match[1];
    const rawState = match[2].toLowerCase();
    const name = match[3].trim();

    // Infer platform: UUIDs (8-4-4-4-12) = iOS, otherwise Android
    const isIOS = /^[0-9A-F]{8}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{12}$/i.test(serial);
    const platform: Device['platform'] = isIOS ? 'ios' : 'android';

    // Normalize state: "device" (adb term) = "booted"
    const state: Device['state'] = (rawState === 'device' || rawState === 'booted') ? 'booted' :
                                    rawState === 'offline' ? 'offline' : 'unknown';

    // Skip watches
    if (name.toLowerCase().includes('watch')) { continue; }

    devices.push({ serial, name, platform, state });
  }

  return devices;
}

export async function runLint(filePath: string): Promise<LintResult[]> {
  const probePath = getProbePath();
  const result = await runCommand(probePath, ['lint', filePath]);
  const results: LintResult[] = [];
  const output = result.stdout + result.stderr;

  for (const line of output.split('\n')) {
    // Parse error lines like: "file.probe:10:5: error message"
    const errorMatch = line.match(/^(.+?):(\d+):(\d+):\s*(.+)$/);
    if (errorMatch) {
      results.push({
        file: errorMatch[1],
        line: parseInt(errorMatch[2], 10),
        column: parseInt(errorMatch[3], 10),
        severity: line.toLowerCase().includes('warning') ? 'warning' : 'error',
        message: errorMatch[4],
      });
      continue;
    }

    // Parse warning lines like: "⚠ file.probe\n  warning message"
    const warnMatch = line.match(/^\s*[⚠!]\s*(.+\.probe)/);
    if (warnMatch) {
      results.push({
        file: warnMatch[1],
        line: 1,
        column: 1,
        severity: 'warning',
        message: line.trim(),
      });
    }

    // Parse general error without position
    if (result.code !== 0 && line.includes('error') && !line.includes('✓')) {
      const genMatch = line.match(/^\s*(?:error|Error):\s*(.+)/);
      if (genMatch) {
        results.push({
          file: filePath,
          line: 1,
          column: 1,
          severity: 'error',
          message: genMatch[1],
        });
      }
    }
  }

  return results;
}

export function buildTestArgs(
  filePath: string,
  options?: {
    device?: string;
    timeout?: string;
    port?: number;
    tags?: string[];
    format?: string;
    outputPath?: string;
    video?: boolean;
    videoResolution?: string;
    videoFramerate?: number;
    visualThreshold?: number;
    visualPixelDelta?: number;
    autoConfirm?: boolean;
    verbose?: boolean;
    dialTimeout?: string;
    tokenTimeout?: string;
    reconnectDelay?: string;
    adbPath?: string;
    flutterPath?: string;
    config?: string;
    cloudProvider?: string;
    cloudDevice?: string;
    cloudApp?: string;
    cloudKey?: string;
    cloudSecret?: string;
    relayUrl?: string;
    relayToken?: string;
  }
): string[] {
  const args = ['test', filePath];
  const o = options ?? {};

  // Cloud provider flags
  const cloudProvider = o.cloudProvider ?? getSetting<string>('cloudProvider');
  if (cloudProvider) { args.push('--cloud-provider', cloudProvider); }

  const cloudDevice = o.cloudDevice ?? getSetting<string>('cloudDevice');
  if (cloudDevice) { args.push('--cloud-device', cloudDevice); }

  const cloudApp = o.cloudApp ?? getSetting<string>('cloudApp');
  if (cloudApp) { args.push('--cloud-app', cloudApp); }

  const cloudKey = o.cloudKey ?? getSetting<string>('cloudKey');
  if (cloudKey) { args.push('--cloud-key', cloudKey); }

  const cloudSecret = o.cloudSecret ?? getSetting<string>('cloudSecret');
  if (cloudSecret) { args.push('--cloud-secret', cloudSecret); }

  const relayUrl = o.relayUrl ?? getSetting<string>('relayUrl');
  if (relayUrl) { args.push('--relay-url', relayUrl); }

  const relayToken = o.relayToken ?? getSetting<string>('relayToken');
  if (relayToken) { args.push('--relay-token', relayToken); }

  // Local device flags (only when not using cloud provider)
  if (!cloudProvider) {
    const device = o.device ?? getSetting<string>('defaultDevice');
    if (device) { args.push('--device', device); }
  }

  const timeout = o.timeout ?? getSetting<string>('defaultTimeout');
  if (timeout) { args.push('--timeout', timeout); }

  const port = o.port ?? getSetting<number>('agentPort');
  if (port && port !== 48686) { args.push('--port', String(port)); }

  if (o.tags?.length) {
    for (const tag of o.tags) { args.push('--tag', tag); }
  }

  const format = o.format ?? getSetting<string>('outputFormat');
  if (format && format !== 'terminal') { args.push('--format', format); }

  const outputPath = o.outputPath ?? getSetting<string>('outputPath');
  if (outputPath) { args.push('-o', outputPath); }

  const video = o.video ?? getSetting<boolean>('video');
  if (video) { args.push('--video'); }

  if (o.videoResolution) { args.push('--video-resolution', o.videoResolution); }
  if (o.videoFramerate) { args.push('--video-framerate', String(o.videoFramerate)); }

  if (o.visualThreshold !== undefined) { args.push('--visual-threshold', String(o.visualThreshold)); }
  if (o.visualPixelDelta !== undefined) { args.push('--visual-pixel-delta', String(o.visualPixelDelta)); }

  const autoConfirm = o.autoConfirm ?? getSetting<boolean>('autoConfirm');
  if (autoConfirm) { args.push('-y'); }

  if (o.verbose !== false) { args.push('-v'); }

  const dialTimeout = o.dialTimeout ?? getSetting<string>('dialTimeout');
  if (dialTimeout && dialTimeout !== '30s') { args.push('--dial-timeout', dialTimeout); }

  const tokenTimeout = o.tokenTimeout ?? getSetting<string>('tokenTimeout');
  if (tokenTimeout && tokenTimeout !== '30s') { args.push('--token-timeout', tokenTimeout); }

  const reconnectDelay = o.reconnectDelay ?? getSetting<string>('reconnectDelay');
  if (reconnectDelay && reconnectDelay !== '2s') { args.push('--reconnect-delay', reconnectDelay); }

  const adbPath = o.adbPath ?? getSetting<string>('adbPath');
  if (adbPath) { args.push('--adb', adbPath); }

  const flutterPath = o.flutterPath ?? getSetting<string>('flutterPath');
  if (flutterPath) { args.push('--flutter', flutterPath); }

  if (o.config) { args.push('--config', o.config); }

  return args;
}

export function getProbeCommand(): string {
  return getProbePath();
}

export function getConvertCommand(): string {
  return getConvertPath();
}
