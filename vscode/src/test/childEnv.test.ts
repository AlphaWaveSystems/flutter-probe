import { test } from 'node:test';
import * as assert from 'node:assert/strict';
import * as fs from 'node:fs';
import * as path from 'node:path';
import { ALL_SECRETS, childEnv } from '../secretArgs';

const base = {
  PATH: '/usr/bin',
  HOME: '/home/a',
  USER_EMAIL: 'a@example.com', // a ${USER_EMAIL} in a test reads it: kept
  PROBE_CLOUD_URL: 'https://c', // not a secret: kept
  PROBE_CLOUD_KEY: 'key',
  PROBE_CLOUD_SECRET: 'secret',
  PROBE_RELAY_CLI_TOKEN: 'token',
};

test('a child that is not a test run gets the environment without the three secrets', () => {
  const env = childEnv(base);
  assert.deepEqual(env, { PATH: '/usr/bin', HOME: '/home/a', USER_EMAIL: 'a@example.com', PROBE_CLOUD_URL: 'https://c' });
  assert.equal(base.PROBE_CLOUD_SECRET, 'secret', 'the base is not changed');
});

test('a test run keeps the secrets it is allowed, and the settings come on top', () => {
  assert.deepEqual(childEnv(base, { allow: ALL_SECRETS }), base);
  assert.deepEqual(childEnv(base, { allow: ['cloudKey'] }).PROBE_CLOUD_SECRET, undefined);
  assert.equal(childEnv(base, { allow: ['cloudKey'] }).PROBE_CLOUD_KEY, 'key');
  const env = childEnv(base, { allow: ALL_SECRETS, add: { PROBE_CLOUD_SECRET: 'from-settings' } });
  assert.equal(env.PROBE_CLOUD_SECRET, 'from-settings');
  assert.equal(env.PROBE_CLOUD_KEY, 'key');
});

test('a secret variable is removed in any casing, and the settings replace any casing of their name', () => {
  const mixed = { Path: 'C:\\Windows', Probe_Cloud_Key: 'k', probe_cloud_secret: 's', PROBE_relay_CLI_token: 't', Probe_Cloud_Url: 'u' };
  assert.deepEqual(childEnv(mixed), { Path: 'C:\\Windows', Probe_Cloud_Url: 'u' });
  const env = childEnv(mixed, { allow: ALL_SECRETS, add: { PROBE_CLOUD_SECRET: 'from-settings' } });
  assert.equal(env.PROBE_CLOUD_SECRET, 'from-settings');
  assert.equal(env.probe_cloud_secret, undefined);
  assert.equal(env.Probe_Cloud_Key, 'k');
});

// The spawn sites import vscode, so their use of childEnv is checked in their source.
test('each spawned child gets the environment its role allows', () => {
  const src = path.join(__dirname, '..', '..', 'src');
  const read = (f: string) => fs.readFileSync(path.join(src, f), 'utf8');
  assert.match(read('probe.ts'), /env: childEnv\(process\.env\), \/\/ device list and lint/);
  assert.match(read('commands/deviceView.ts'), /env: childEnv\(process\.env\), stdio/);
  assert.match(read('testing/testController.ts'), /env: childEnv\(process\.env, \{ allow: ALL_SECRETS, add: settingsSecretEnv\(\) \}\)/);
  // No spawn passes the whole environment on its own any more.
  for (const f of ['probe.ts', 'commands/deviceView.ts', 'testing/testController.ts']) {
    assert.doesNotMatch(read(f), /env: \{ \.\.\.process\.env \}/, f);
  }
});
