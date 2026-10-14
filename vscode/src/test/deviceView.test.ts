import { test } from 'node:test';
import * as assert from 'node:assert/strict';
import { EventEmitter } from 'events';
import { buildCsp, generateNonce, hubOriginFromUrl } from '../deviceView/csp';
import { LineBuffer, parseHubLine } from '../deviceView/hubLine';
import { ChildLike, HubProcess, HubStartError } from '../deviceView/hubProcess';
import { buildTapStep, planInsert } from '../deviceView/insertStep';
import { clampViewConfig, statusBarText } from '../deviceView/viewConfig';
import { buildWebviewHtml } from '../deviceView/html';

const GOOD = '{"url":"ws://127.0.0.1:41234/device/v1","token":"abcDEF123_-","device":"emulator-5554"}';

// ---- CSP ----

test('buildCsp produces the exact policy', () => {
  const csp = buildCsp('N0nce', 'vscode-resource.test', 'ws://127.0.0.1:41234');
  assert.equal(
    csp,
    "default-src 'none'; script-src 'nonce-N0nce' vscode-resource.test; " +
    "style-src vscode-resource.test 'nonce-N0nce'; connect-src ws://127.0.0.1:41234; " +
    'img-src data: blob: vscode-resource.test',
  );
});

test('buildCsp has no wildcard and exactly one connect-src origin', () => {
  const csp = buildCsp('n', 'src.test', 'ws://127.0.0.1:9');
  assert.ok(!csp.includes('*'));
  const connect = csp.split(';').map(s => s.trim()).filter(s => s.startsWith('connect-src'));
  assert.equal(connect.length, 1);
  assert.deepEqual(connect[0].split(/\s+/).slice(1), ['ws://127.0.0.1:9']);
  assert.ok(!csp.includes('unsafe-inline') && !csp.includes('unsafe-eval'));
});

test('hubOriginFromUrl accepts only loopback ws', () => {
  assert.equal(hubOriginFromUrl('ws://127.0.0.1:41234/device/v1'), 'ws://127.0.0.1:41234');
  assert.equal(hubOriginFromUrl('ws://localhost:1/device/v1'), undefined);
  assert.equal(hubOriginFromUrl('ws://0.0.0.0:1/x'), undefined);
  assert.equal(hubOriginFromUrl('wss://127.0.0.1:1/x'), undefined);
  assert.equal(hubOriginFromUrl('ws://127.0.0.1:99999/x'), undefined);
  assert.equal(hubOriginFromUrl('ws://127.0.0.1.evil.test:1/x'), undefined);
  assert.equal(hubOriginFromUrl('ws://127.0.0.1/x'), undefined);
});

test('nonces are unique and url-safe', () => {
  const a = generateNonce();
  assert.notEqual(a, generateNonce());
  assert.match(a, /^[A-Za-z0-9_-]{22}$/);
});

test('webview html contains the policy, nonce on scripts and no token', () => {
  const html = buildWebviewHtml({
    nonce: 'abc', cspSource: 'src.test', hubOrigin: 'ws://127.0.0.1:5',
    bundleUri: 'https://src.test/device-view.js', hostUri: 'https://src.test/device-view-host.js',
  });
  assert.ok(html.includes('Content-Security-Policy'));
  assert.equal((html.match(/<script nonce="abc" src=/g) ?? []).length, 2);
  assert.ok(!/<script(?![^>]*src=)/.test(html), 'no inline script');
  assert.ok(!html.toLowerCase().includes('token'));
});

// ---- JSON line parser ----

test('parseHubLine accepts the hub line', () => {
  assert.deepEqual(parseHubLine(GOOD), {
    url: 'ws://127.0.0.1:41234/device/v1', token: 'abcDEF123_-', device: 'emulator-5554',
  });
  assert.ok(parseHubLine(`  ${GOOD}\r`));
});

test('parseHubLine rejects garbage and partial lines', () => {
  for (const bad of [
    '', 'hello', 'Listening...', '{', GOOD.slice(0, 30), '[]', 'null', '{}',
    '{"url":"ws://127.0.0.1:1/device/v1"}',
    '{"url":"ws://127.0.0.1:1/device/v1","token":""}',
    '{"url":"http://127.0.0.1:1/","token":"t"}',
    '{"url":"ws://evil.test:1/device/v1","token":"t"}',
    '{"url":"ws://127.0.0.1:1/device/v1","token":"has space"}',
    '{"url":1,"token":"t"}',
  ]) {
    assert.equal(parseHubLine(bad), undefined, bad);
  }
});

test('LineBuffer joins partial chunks', () => {
  const b = new LineBuffer();
  assert.deepEqual(b.push('{"url":'), []);
  assert.deepEqual(b.push('1}\nsecond'), ['{"url":1}']);
  assert.deepEqual(b.push(' line\r\nthird'), ['second line']);
  assert.deepEqual(b.flush(), ['third']);
  assert.deepEqual(b.flush(), []);
});

// ---- Process lifecycle with a fake spawn ----

class FakeChild extends EventEmitter {
  stdout = new EventEmitter();
  stderr = new EventEmitter();
  killed = 0;
  kill(): boolean { this.killed++; return true; }
}

function mk(opts: { onStderr?: (l: string) => void; onExit?: (c: number | null) => void; startTimeoutMs?: number } = {}) {
  const child = new FakeChild();
  const calls: Array<[string, string[], string | undefined]> = [];
  const hub = new HubProcess({
    spawn: (c, a, d) => { calls.push([c, a, d]); return child as unknown as ChildLike; },
    ...opts,
  });
  return { child, hub, calls };
}

test('start resolves on the hub line split across chunks and spawns the right command', async () => {
  const { child, hub, calls } = mk();
  const p = hub.start('probe', ['device', 'view', '--device', 'X', '--json'], '/ws');
  child.stdout.emit('data', Buffer.from('starting\n' + GOOD.slice(0, 25)));
  child.stdout.emit('data', Buffer.from(GOOD.slice(25) + '\n'));
  const info = await p;
  assert.equal(info.token, 'abcDEF123_-');
  assert.deepEqual(calls[0], ['probe', ['device', 'view', '--device', 'X', '--json'], '/ws']);
  assert.ok(hub.running);
  hub.stop();
});

test('start rejects with not-found on ENOENT', async () => {
  const { child, hub } = mk();
  const p = hub.start('probe', []);
  const err = Object.assign(new Error('spawn probe ENOENT'), { code: 'ENOENT' });
  child.emit('error', err);
  await assert.rejects(p, (e: unknown) => e instanceof HubStartError && e.kind === 'not-found');
});

test('start rejects with exited and includes stderr when the process dies early', async () => {
  const lines: string[] = [];
  const { child, hub } = mk({ onStderr: l => lines.push(l) });
  const p = hub.start('probe', []);
  child.stderr.emit('data', Buffer.from('device X not found\n'));
  child.emit('close', 1, null);
  await assert.rejects(p, (e: unknown) =>
    e instanceof HubStartError && e.kind === 'exited' && e.message.includes('device X not found'));
  assert.deepEqual(lines, ['device X not found']);
});

test('start rejects when spawn throws', async () => {
  const hub = new HubProcess({ spawn: () => { throw new Error('boom'); } });
  await assert.rejects(hub.start('probe', []), (e: unknown) => e instanceof HubStartError && e.kind === 'spawn');
});

test('start times out and kills the child', async () => {
  const { child, hub } = mk({ startTimeoutMs: 10 });
  await assert.rejects(hub.start('probe', []), (e: unknown) => e instanceof HubStartError && e.kind === 'timeout');
  assert.equal(child.killed, 1);
});

test('stderr never exposes the token', async () => {
  const lines: string[] = [];
  const { child, hub } = mk({ onStderr: l => lines.push(l) });
  const p = hub.start('probe', []);
  child.stdout.emit('data', GOOD + '\n');
  await p;
  child.stderr.emit('data', 'connect ?token=abcDEF123_- ok\n');
  assert.deepEqual(lines, ['connect ?token=*** ok']);
  hub.stop();
});

test('onExit fires on unexpected exit, not after stop()', async () => {
  const exits: Array<number | null> = [];
  const a = mk({ onExit: c => { exits.push(c); } });
  const pa = a.hub.start('probe', []);
  a.child.stdout.emit('data', GOOD + '\n');
  await pa;
  a.child.emit('close', 2, null);
  assert.deepEqual([...exits], [2]);
  assert.ok(!a.hub.running);

  const b = mk({ onExit: c => { exits.push(c); } });
  const pb = b.hub.start('probe', []);
  b.child.stdout.emit('data', GOOD + '\n');
  await pb;
  b.hub.stop();
  b.hub.stop();
  assert.equal(b.child.killed, 1);
  b.child.emit('close', null, 'SIGTERM');
  assert.deepEqual([...exits], [2]);
});

// ---- Insert step ----

test('buildTapStep formats selector chips', () => {
  assert.equal(buildTapStep('#loginButton'), 'tap #loginButton');
  assert.equal(buildTapStep('"Sign in"'), 'tap "Sign in"');
  assert.equal(buildTapStep('Submit label'), 'tap "Submit label"');
  assert.equal(buildTapStep('say "hi"'), 'tap "say \\"hi\\""');
  assert.equal(buildTapStep('  #k  '), 'tap #k');
});

test('buildTapStep rejects empty, multi-line and oversized selectors', () => {
  assert.equal(buildTapStep(''), undefined);
  assert.equal(buildTapStep('   '), undefined);
  assert.equal(buildTapStep('a\nb'), undefined);
  assert.equal(buildTapStep('x'.repeat(201)), undefined);
});

test('planInsert puts the step in place on blank lines', () => {
  assert.deepEqual(planInsert('', '#a'), { text: 'tap #a' });
  assert.deepEqual(planInsert('    ', '#a'), { text: 'tap #a' });
});

test('planInsert adds a new indented line after non-blank lines', () => {
  assert.deepEqual(planInsert('  open the app', '#a'), { text: '\n  tap #a' });
  assert.deepEqual(planInsert('\twait 1 seconds', '"X"'), { text: '\n\ttap "X"' });
  assert.equal(planInsert('x', ''), undefined);
});

// ---- Config / status ----

test('clampViewConfig applies defaults and bounds', () => {
  assert.deepEqual(clampViewConfig(undefined, undefined), { fps: 10, quality: 60 });
  assert.deepEqual(clampViewConfig(0, 500), { fps: 1, quality: 100 });
  assert.deepEqual(clampViewConfig(99, -3), { fps: 30, quality: 1 });
  assert.deepEqual(clampViewConfig(NaN, '5'), { fps: 10, quality: 60 });
  assert.deepEqual(clampViewConfig(7.6, 55.2), { fps: 8, quality: 55 });
});

test('statusBarText maps hub states and unknown states to error', () => {
  assert.match(statusBarText('connected', 'd').text, /connected/);
  assert.match(statusBarText('weird', 'd').text, /error/);
});
