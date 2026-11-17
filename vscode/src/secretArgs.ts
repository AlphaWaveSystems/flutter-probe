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
