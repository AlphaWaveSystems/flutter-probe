export interface ViewConfig { fps: number; quality: number }

function clamp(v: unknown, min: number, max: number, dflt: number): number {
  if (typeof v !== 'number' || !Number.isFinite(v)) { return dflt; }
  return Math.min(max, Math.max(min, Math.round(v)));
}

export function clampViewConfig(fps: unknown, quality: unknown): ViewConfig {
  return { fps: clamp(fps, 1, 30, 10), quality: clamp(quality, 1, 100, 60) };
}

const ICONS: Record<string, string> = {
  connected: '$(vm-running)',
  reconnecting: '$(sync~spin)',
  error: '$(error)',
  paused: '$(debug-pause)',
  starting: '$(sync~spin)',
  stopped: '$(circle-slash)',
};

export function statusBarText(state: string, device: string): { text: string; tooltip: string } {
  const known = state in ICONS ? state : 'error';
  return {
    text: `${ICONS[known]} Device view: ${known}`,
    tooltip: `FlutterProbe device view (${device}): ${known}`,
  };
}
