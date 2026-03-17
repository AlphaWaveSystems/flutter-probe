export interface Device {
  serial: string;
  name: string;
  platform: 'android' | 'ios';
  state: 'booted' | 'offline' | 'unknown';
}

export interface Session {
  id: string;
  name: string;
  device: Device;
  config?: string;
  port: number;
  devicePort: number;
  status: 'disconnected' | 'connecting' | 'connected' | 'running';
  activeTest?: string;
  lastResult?: TestRunSummary;
}

export interface TestRunSummary {
  passed: number;
  failed: number;
  skipped: number;
  total: number;
  outcome: 'passed' | 'failed';
}

export interface TestDefinition {
  name: string;
  file: string;
  line: number;
  tags: string[];
}

export interface RecipeDefinition {
  name: string;
  file: string;
  line: number;
  params: string[];
}

export interface LintResult {
  file: string;
  line: number;
  column: number;
  severity: 'error' | 'warning' | 'info';
  message: string;
}

export interface ProbeConfig {
  project?: {
    app?: string;
    platform?: string;
  };
  agent?: {
    port?: number;
    device_port?: number;
    dial_timeout?: string;
    ping_interval?: string;
    token_timeout?: string;
    reconnect_delay?: string;
  };
  defaults?: {
    platform?: string;
    timeout?: string;
    screenshots?: boolean;
    video?: boolean;
    retry?: number;
    grant_permissions_on_clear?: boolean;
  };
  device?: {
    emulator_boot_timeout?: string;
    simulator_boot_timeout?: string;
    restart_delay?: string;
  };
  video?: {
    resolution?: string;
    framerate?: number;
  };
  visual?: {
    threshold?: number;
    pixel_delta?: number;
  };
  tools?: {
    adb?: string;
    flutter?: string;
  };
  reports_folder?: string;
}

export interface CommandResult {
  stdout: string;
  stderr: string;
  code: number;
}
