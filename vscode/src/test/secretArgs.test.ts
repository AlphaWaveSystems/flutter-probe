import { test } from 'node:test';
import * as assert from 'node:assert/strict';
import * as fs from 'node:fs';
import * as path from 'node:path';
import { SECRET_ENV, SECRET_SETTINGS, envFingerprint, secretEnv, secretFlagIn, staleWorkspaceSettings, terminalAction, withoutSecrets } from '../secretArgs';

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
  assert.match(text, /if \(existing && action === 'reuse'\) \{/);
  assert.match(text, /if \(existing && action === 'replace'\) \{ existing\.terminal\.dispose\(\); \}/);
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
          // A prefix match: it also catches '--cloud-key=' + value and `--cloud-key=${v}`.
          if (text.includes(`'${f}`) || text.includes(`"${f}`) || text.includes('`' + f)) { offenders.push(`${path.relative(src, p)}: ${f}`); }
        }
      }
    }
  };
  walk(src);
  assert.ok(fs.existsSync(path.join(src, 'probe.ts')), 'the source folder was not found');
  assert.deepEqual(offenders, []);
});

test('the options lose their secrets before the argument builder sees them', () => {
  assert.deepEqual(withoutSecrets({ cloudKey: 'k', cloudSecret: 's', relayToken: 't', device: 'emulator-5554' }), { device: 'emulator-5554' });
});

// Every setting that is a secret, or decides where one is sent, is machine-scoped: a workspace's settings.json cannot set it.
test('the secret settings and the settings that decide where secrets go are machine-scoped', () => {
  const pkg = JSON.parse(fs.readFileSync(path.join(__dirname, '..', '..', 'package.json'), 'utf8'));
  const conf = pkg.contributes.configuration;
  const props = Array.isArray(conf) ? Object.assign({}, ...conf.map((c: { properties: object }) => c.properties)) : conf.properties;
  assert.deepEqual([...SECRET_SETTINGS].sort(), ['cloudKey', 'cloudProvider', 'cloudSecret', 'relayToken', 'relayUrl']);
  for (const key of SECRET_SETTINGS) {
    assert.equal(props[`flutterprobe.${key}`]?.scope, 'machine', `flutterprobe.${key} must be machine-scoped`);
  }
});

test('a terminal is reused only when it is alive and was made with the same secrets', () => {
  const env = { PROBE_CLOUD_SECRET: 'one' };
  assert.equal(terminalAction(undefined, env), 'create');
  assert.equal(terminalAction({ alive: true, fingerprint: envFingerprint(env) }, env), 'reuse');
  assert.equal(terminalAction({ alive: true, fingerprint: envFingerprint({ PROBE_CLOUD_SECRET: 'two' }) }, env), 'replace');
  assert.equal(terminalAction({ alive: true, fingerprint: envFingerprint({}) }, env), 'replace');
  assert.equal(terminalAction({ alive: false, fingerprint: envFingerprint(env) }, env), 'create');
});

test('a workspace settings file that still names a machine-scoped setting is found', () => {
  const text = '{\n  // old\n  "flutterprobe.cloudSecret" : "x",\n  "flutterprobe.relayUrl": "wss://r",\n  "flutterprobe.defaultDevice": "d"\n}';
  assert.deepEqual(staleWorkspaceSettings(text, SECRET_SETTINGS), ['cloudSecret', 'relayUrl']);
  assert.deepEqual(staleWorkspaceSettings('{ "flutterprobe.cloudSecrets": 1, "other.cloudKey": 2 }', SECRET_SETTINGS), []);
});

// The callers import vscode, so their hand-off is checked in their source: every run that builds a TestRun passes its env to the
// terminal, and the run profile hands the secrets of its form to buildTestRun.
test('every test run hands its env to the terminal', () => {
  const src = path.join(__dirname, '..', '..', 'src');
  const files = ['extension.ts', 'commands/test.ts', 'providers/testExplorer.ts', 'providers/sessionManager.ts', 'views/runProfile.ts'];
  let runs = 0;
  for (const f of files) {
    const text = fs.readFileSync(path.join(src, f), 'utf8');
    assert.doesNotMatch(text, /buildTestArgs\(/, `${f} must build runs with buildTestRun`);
    for (const call of text.match(/runInTerminal\([^;]*\);/g) ?? []) {
      if (call.includes('run.args')) {
        runs++;
        assert.match(call, /run\.args, [^;]*, run\.env\);$/, `${f}: ${call}`);
      }
    }
  }
  assert.equal(runs, 13, 'the 13 test-run call sites');
  const profile = fs.readFileSync(path.join(src, 'views/runProfile.ts'), 'utf8');
  for (const k of ['cloudKey', 'cloudSecret', 'relayToken']) {
    assert.match(profile, new RegExp(`${k}: msg\\.${k} \\|\\| undefined`), `runProfile must hand ${k} to buildTestRun`);
  }
});

test('the extension says once when a workspace still sets a machine-scoped setting', () => {
  const text = fs.readFileSync(path.join(__dirname, '..', '..', 'src', 'extension.ts'), 'utf8');
  assert.match(text, /staleWorkspaceSettings\(text, SECRET_SETTINGS\)/);
  assert.match(text, /workspaceState\.update\('flutterprobe\.staleSettingsNoticed', true\)/);
  assert.match(text, /export function activate\(context: vscode\.ExtensionContext\): void \{\n  noticeStaleWorkspaceSettings\(context\);/);
});
