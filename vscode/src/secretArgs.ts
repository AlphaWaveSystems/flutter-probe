// Secrets never go on the command line. A command line is visible to other users in the process list and stays in shell history, so
// the cloud key, the cloud secret and the relay token reach `probe test` as environment variables of its terminal, which the CLI reads
// when the flag is not given. This module has no vscode import, so the tests can load it.

export interface SecretValues {
  cloudKey?: string;
  cloudSecret?: string;
  relayToken?: string;
}

// The environment variable of each secret, and the flag it replaces. The CLI reads the same names (internal/cli/secret_flags.go).
export const SECRET_ENV: Record<keyof SecretValues, { env: string; flag: string }> = {
  cloudKey: { env: 'PROBE_CLOUD_KEY', flag: '--cloud-key' },
  cloudSecret: { env: 'PROBE_CLOUD_SECRET', flag: '--cloud-secret' },
  relayToken: { env: 'PROBE_RELAY_CLI_TOKEN', flag: '--relay-token' },
};

// secretEnv returns the environment variables for the secrets that are set; an empty value is left out.
export function secretEnv(values: SecretValues): Record<string, string> {
  const env: Record<string, string> = {};
  for (const key of Object.keys(SECRET_ENV) as (keyof SecretValues)[]) {
    const v = values[key];
    if (v) { env[SECRET_ENV[key].env] = v; }
  }
  return env;
}

// withoutSecrets is the options with the three secrets taken out, so they cannot reach the argument builder.
export function withoutSecrets<T extends SecretValues>(o: T): Omit<T, keyof SecretValues> {
  const { cloudKey: _k, cloudSecret: _s, relayToken: _t, ...rest } = o;
  return rest;
}

// The settings that are a secret, or that decide where a secret is sent: the relay URL receives the relay CLI token, and the provider
// receives the cloud key and secret. All are machine-scoped (package.json), so a workspace's .vscode/settings.json cannot set them.
export const SECRET_SETTINGS = ['cloudKey', 'cloudSecret', 'relayToken', 'relayUrl', 'cloudProvider'] as const;

// staleWorkspaceSettings returns the machine-scoped settings that a workspace's settings.json still names. VS Code ignores them there, so
// the extension says so once instead of dropping them silently. A textual match is enough for a notice: the file is JSON with comments.
export function staleWorkspaceSettings(settingsText: string, keys: readonly string[]): string[] {
  return keys.filter(k => new RegExp(`"flutterprobe\\.${k}"\\s*:`).test(settingsText));
}

// terminalAction decides whether a named terminal can be reused for a run: only when it is alive and was made with the same secrets,
// since a terminal's environment is fixed when it is created.
export function terminalAction(existing: { alive: boolean; fingerprint: string } | undefined, env: Record<string, string>): 'reuse' | 'replace' | 'create' {
  if (!existing) { return 'create'; }
  if (existing.alive && existing.fingerprint === envFingerprint(env)) { return 'reuse'; }
  return existing.alive ? 'replace' : 'create';
}

// childEnv is the environment of a program the extension starts: VS Code's own environment, as every child inherits it (a ${NAME} in a
// test reads any variable by design, so nothing else is removed), without the three secret variables unless the child is one meant to
// receive them (allow), and with add on top. Only `probe test` is meant to: the device list, lint and the device view are not.
export function childEnv(
  base: Record<string, string | undefined>,
  opts: { allow?: readonly (keyof SecretValues)[]; add?: Record<string, string> } = {},
): Record<string, string | undefined> {
  const env = { ...base };
  for (const key of Object.keys(SECRET_ENV) as (keyof SecretValues)[]) {
    if (!opts.allow?.includes(key)) { delete env[SECRET_ENV[key].env]; }
  }
  return { ...env, ...(opts.add ?? {}) };
}

// ALL_SECRETS is the allow list of a child that runs tests.
export const ALL_SECRETS = Object.keys(SECRET_ENV) as (keyof SecretValues)[];

// secretFlagIn returns the first secret flag in a command line (`--cloud-key` or `--cloud-key=...`), or undefined when there is none.
export function secretFlagIn(args: readonly string[]): string | undefined {
  const flags = Object.values(SECRET_ENV).map(s => s.flag);
  for (const a of args) {
    const name = a.split('=', 1)[0];
    if (flags.includes(name)) { return name; }
  }
  return undefined;
}

// envFingerprint is a stable text for an environment, so a terminal made with one set of secrets is not reused for another. It holds
// each name, the value's length and a 32-bit hash of the value, not the value. A short secret could be found again from its hash by trying
// every candidate, so the fingerprint stays in memory as a map key and is never printed, logged or stored.
export function envFingerprint(env: Record<string, string>): string {
  return Object.keys(env).sort().map(k => `${k}:${env[k].length}:${hash(env[k])}`).join(',');
}

// hash is a small non-cryptographic hash (FNV-1a), enough to tell two values apart without keeping either.
function hash(s: string): string {
  let h = 0x811c9dc5;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 0x01000193) >>> 0;
  }
  return h.toString(16);
}
