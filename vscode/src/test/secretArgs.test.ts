import { test } from 'node:test';
import * as assert from 'node:assert/strict';
import * as fs from 'node:fs';
import * as path from 'node:path';
import { SECRET_ENV, envFingerprint, secretEnv, secretFlagIn } from '../secretArgs';

test('each secret goes to the environment variable the CLI reads', () => {
  assert.deepEqual(
    secretEnv({ cloudKey: 'user-1', cloudSecret: 's3cr3t', relayToken: 'tok' }),
    { PROBE_CLOUD_KEY: 'user-1', PROBE_CLOUD_SECRET: 's3cr3t', PROBE_RELAY_CLI_TOKEN: 'tok' },
  );
  assert.deepEqual(
    Object.fromEntries(Object.values(SECRET_ENV).map(s => [s.flag, s.env])),
    { '--cloud-key': 'PROBE_CLOUD_KEY', '--cloud-secret': 'PROBE_CLOUD_SECRET', '--relay-token': 'PROBE_RELAY_CLI_TOKEN' },
  );
});

test('a secret that is not set, or set empty, is left out', () => {
  assert.deepEqual(secretEnv({}), {});
  assert.deepEqual(secretEnv({ cloudKey: '', cloudSecret: 'x' }), { PROBE_CLOUD_SECRET: 'x' });
});

test('a secret flag is found on a command line, in either form', () => {
  assert.equal(secretFlagIn(['test', 'a.probe', '--cloud-secret', 'x']), '--cloud-secret');
  assert.equal(secretFlagIn(['test', '--relay-token=abc']), '--relay-token');
  assert.equal(secretFlagIn(['test', '--cloud-key', 'u']), '--cloud-key');
  assert.equal(secretFlagIn(['test', 'a.probe', '--cloud-provider', 'aws', '--relay-url', 'wss://r', '--device', 'x']), undefined);
});

test('the fingerprint tells two environments apart and holds no value', () => {
  const a = envFingerprint({ PROBE_CLOUD_SECRET: 'first-secret' });
  const b = envFingerprint({ PROBE_CLOUD_SECRET: 'other-secret' });
  assert.notEqual(a, b);
  assert.equal(a, envFingerprint({ PROBE_CLOUD_SECRET: 'first-secret' }));
  assert.equal(envFingerprint({ A: '1', B: '2' }), envFingerprint({ B: '2', A: '1' }));
  assert.ok(!a.includes('first-secret'));
  assert.equal(envFingerprint({}), '');
});

// runInTerminal imports vscode, so a unit test cannot call it: its source must refuse a secret flag and create the terminal with the env.
test('runInTerminal refuses a secret flag and gives the terminal its environment', () => {
  const file = path.join(__dirname, '..', '..', 'src', 'terminal', 'probeTerminal.ts');
  const text = fs.readFileSync(file, 'utf8');
  assert.match(text, /const secret = secretFlagIn\(args\);\s*if \(secret\) \{\s*throw new Error/);
  assert.match(text, /createTerminal\(\{[^}]*\benv,?\s*\}\)/);
  assert.match(text, /getOrCreateTerminal\(name, cwd, env\)/);
});

// The extension's sources name a secret flag only in secretArgs.ts: anything else that writes one would put a secret on a command line.
test('no source file outside secretArgs.ts names a secret flag', () => {
  const src = path.join(__dirname, '..', '..', 'src');
  const flags = Object.values(SECRET_ENV).map(s => s.flag);
  const offenders: string[] = [];
  const walk = (dir: string): void => {
    for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
      const p = path.join(dir, e.name);
      if (e.isDirectory()) {
        if (e.name !== 'test') { walk(p); }
      } else if (p.endsWith('.ts') && e.name !== 'secretArgs.ts') {
        const text = fs.readFileSync(p, 'utf8');
        for (const f of flags) {
          if (text.includes(`'${f}'`) || text.includes(`"${f}"`) || text.includes('`' + f)) { offenders.push(`${path.relative(src, p)}: ${f}`); }
        }
      }
    }
  };
  walk(src);
  assert.ok(fs.existsSync(path.join(src, 'probe.ts')), 'the source folder was not found');
  assert.deepEqual(offenders, []);
});
