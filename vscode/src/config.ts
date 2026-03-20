import * as vscode from 'vscode';
import * as fs from 'fs';
import * as path from 'path';
import { ProbeConfig } from './types';

export function getConfig(): vscode.WorkspaceConfiguration {
  return vscode.workspace.getConfiguration('flutterprobe');
}

export function getSetting<T>(key: string): T | undefined {
  return getConfig().get<T>(key);
}

export function getProbePath(): string {
  const configured = getSetting<string>('probePath');
  if (configured && configured !== 'probe') {
    return configured;
  }
  const ws = getWorkspaceRoot();
  if (ws) {
    const localBin = path.join(ws, 'bin', 'probe');
    if (fs.existsSync(localBin)) {
      return localBin;
    }
  }
  return 'probe';
}

export function getConvertPath(): string {
  const configured = getSetting<string>('convertPath');
  if (configured && configured !== 'probe-convert') {
    return configured;
  }
  const ws = getWorkspaceRoot();
  if (ws) {
    const localBin = path.join(ws, 'bin', 'probe-convert');
    if (fs.existsSync(localBin)) {
      return localBin;
    }
  }
  return 'probe-convert';
}

export function getWorkspaceRoot(): string | undefined {
  return vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
}

export function readProbeYaml(): ProbeConfig | undefined {
  const ws = getWorkspaceRoot();
  if (!ws) { return undefined; }
  const yamlPath = path.join(ws, 'probe.yaml');
  if (!fs.existsSync(yamlPath)) { return undefined; }
  try {
    const content = fs.readFileSync(yamlPath, 'utf-8');
    // Simple YAML parsing for key: value pairs (flat sections)
    // For full parsing, would need a YAML library, but VS Code extensions
    // should minimize dependencies. We parse the common flat keys.
    const config: ProbeConfig = {};
    const lines = content.split('\n');
    let currentSection = '';
    for (const line of lines) {
      const trimmed = line.trimEnd();
      if (!trimmed || trimmed.startsWith('#')) { continue; }
      const sectionMatch = trimmed.match(/^(\w+):$/);
      if (sectionMatch) {
        currentSection = sectionMatch[1];
        continue;
      }
      const kvMatch = trimmed.match(/^\s+(\w+):\s*(.+)$/);
      if (kvMatch) {
        const [, key, rawValue] = kvMatch;
        const value = rawValue.replace(/^["']|["']$/g, '');
        setConfigValue(config, currentSection, key, value);
      }
    }
    return config;
  } catch {
    return undefined;
  }
}

function setConfigValue(config: ProbeConfig, section: string, key: string, value: string): void {
  const numValue = Number(value);
  const boolValue = value === 'true' ? true : value === 'false' ? false : undefined;

  switch (section) {
    case 'project':
      if (!config.project) { config.project = {}; }
      if (key === 'app') { config.project.app = value; }
      if (key === 'platform') { config.project.platform = value; }
      break;
    case 'agent':
      if (!config.agent) { config.agent = {}; }
      if (key === 'port' && !isNaN(numValue)) { config.agent.port = numValue; }
      if (key === 'device_port' && !isNaN(numValue)) { config.agent.device_port = numValue; }
      if (key === 'dial_timeout') { config.agent.dial_timeout = value; }
      if (key === 'token_timeout') { config.agent.token_timeout = value; }
      if (key === 'reconnect_delay') { config.agent.reconnect_delay = value; }
      break;
    case 'defaults':
      if (!config.defaults) { config.defaults = {}; }
      if (key === 'timeout') { config.defaults.timeout = value; }
      if (key === 'platform') { config.defaults.platform = value; }
      if (key === 'screenshots' && boolValue !== undefined) { config.defaults.screenshots = boolValue; }
      if (key === 'video' && boolValue !== undefined) { config.defaults.video = boolValue; }
      if (key === 'retry' && !isNaN(numValue)) { config.defaults.retry = numValue; }
      break;
    case 'video':
      if (!config.video) { config.video = {}; }
      if (key === 'resolution') { config.video.resolution = value; }
      if (key === 'framerate' && !isNaN(numValue)) { config.video.framerate = numValue; }
      break;
    case 'visual':
      if (!config.visual) { config.visual = {}; }
      if (key === 'threshold' && !isNaN(numValue)) { config.visual.threshold = numValue; }
      if (key === 'pixel_delta' && !isNaN(numValue)) { config.visual.pixel_delta = numValue; }
      break;
    case 'tools':
      if (!config.tools) { config.tools = {}; }
      if (key === 'adb') { config.tools.adb = value; }
      if (key === 'flutter') { config.tools.flutter = value; }
      break;
    case 'cloud':
      if (!config.cloud) { config.cloud = {}; }
      if (key === 'provider') { config.cloud.provider = value; }
      if (key === 'device') { config.cloud.device = value; }
      if (key === 'app') { config.cloud.app = value; }
      break;
    case 'credentials':
      // credentials is a nested section under cloud
      if (!config.cloud) { config.cloud = {}; }
      if (!config.cloud.credentials) { config.cloud.credentials = {}; }
      if (key === 'username') { config.cloud.credentials.username = value; }
      if (key === 'access_key') { config.cloud.credentials.access_key = value; }
      if (key === 'access_key_id') { config.cloud.credentials.access_key_id = value; }
      if (key === 'secret_access_key') { config.cloud.credentials.secret_access_key = value; }
      if (key === 'project_id') { config.cloud.credentials.project_id = value; }
      if (key === 'service_account') { config.cloud.credentials.service_account = value; }
      if (key === 'region') { config.cloud.credentials.region = value; }
      break;
  }
  if (!section && key === 'reports_folder') {
    config.reports_folder = value;
  }
}
