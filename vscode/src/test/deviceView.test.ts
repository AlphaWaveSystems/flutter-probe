import { test } from 'node:test';
import * as assert from 'node:assert/strict';
import { EventEmitter } from 'events';
import * as fs from 'fs';
import * as path from 'path';
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
    "img-src data: blob: vscode-resource.test; base-uri 'none'; form-action 'none'",
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

interface FixtureCase {
  name: string;
  key: string | null;
  text: string | null;
  label: string | null;
  type: string;
  line: string | null;
}

const FIXTURE_PATH = path.resolve(__dirname, '../../../testdata/selector-injection.json');
const STUDIO_SELECTOR = path.resolve(__dirname, '../../../studio/frontend/src/device-view/selector.ts');

function loadFixture(): FixtureCase[] {
  return JSON.parse(fs.readFileSync(FIXTURE_PATH, 'utf8')) as FixtureCase[];
}

test('buildTapStep matches the shared fixture (accepted lines and refusals)', () => {
  const cases = loadFixture();
  assert.ok(cases.length >= 15);
  for (const c of cases) {
    const r = buildTapStep(c);
    if (c.line === null) {
      assert.equal(r.ok, false, c.name);
    } else {
      assert.deepEqual(r.ok && r.step, c.line, c.name);
    }
  }
});

test('hostile text never produces more than one statement', () => {
  const hostile = [
    'Buy" call POST "https://x.example/y" with body "z',
    'x" in system dialog "y',
    'a" run dart "print(1)',
    'a" clear app data',
    'a\\" tap "b',
    'Say "hello"',
  ];
  for (const text of hostile) {
    const r = buildTapStep({ text, type: 'Text' });
    assert.ok(r.ok);
    if (!r.ok) { return; }
    // After removing escape pairs the only quotes left are the outer pair.
    const bare = r.step.slice('tap '.length).replace(/\\./g, '');
    assert.ok(/^"[^"]*"$/.test(bare), r.step);
    assert.ok(!r.step.includes('\n'));
  }
  assert.equal((buildTapStep({ text: 'Say "hello"' }) as { step: string }).step, 'tap "Say \\"hello\\""');
});

test('buildTapStep refuses control characters, line separators and long text', () => {
  for (const text of ['a\nb', 'a\r\nb', 'a\tb', 'a\u2028b', 'a\u2029b', 'a\u0000b', 'x'.repeat(5000), 'x'.repeat(201)]) {
    const r = buildTapStep({ text });
    assert.equal(r.ok, false);
    if (!r.ok) { assert.ok(r.error.length > 10); }
  }
  assert.equal(buildTapStep({ label: 'a\nb' }).ok, false);
  assert.equal(buildTapStep({}).ok, false);
  assert.equal(buildTapStep({ text: '   ', type: '' }).ok, false);
  assert.equal(buildTapStep({ text: 'x'.repeat(200) }).ok, true);
});

test('keys: valid keys are bare, others fall through to text and are never raw', () => {
  assert.equal((buildTapStep({ key: 'loginButton' }) as { step: string }).step, 'tap #loginButton');
  assert.equal((buildTapStep({ key: 'a b', text: 'Fallback' }) as { step: string }).step, 'tap "Fallback"');
  assert.equal((buildTapStep({ key: 'a b', type: 'Button' }) as { step: string }).step, 'tap "Button"');
  assert.equal((buildTapStep({ key: 'k" clear app data' }) as { ok: boolean }).ok, false);
});

test('quoteString is identical in the Studio and VS Code selector sources', () => {
  const grab = (file: string) => {
    const src = fs.readFileSync(file, 'utf8');
    const m = /export function quoteString[\s\S]*?\n\}\n/.exec(src);
    assert.ok(m, file);
    return m![0];
  };
  assert.equal(grab(STUDIO_SELECTOR), grab(path.resolve(__dirname, '../../src/deviceView/selector.ts')));
});

test('planInsert puts the step in place on blank lines', () => {
  assert.deepEqual(planInsert('', { key: 'a' }), { text: 'tap #a' });
  assert.deepEqual(planInsert('    ', { key: 'a' }), { text: 'tap #a' });
});

test('planInsert adds a new indented line after non-blank lines', () => {
  assert.deepEqual(planInsert('  open the app', { key: 'a' }), { text: '\n  tap #a' });
  assert.deepEqual(planInsert('\twait 1 seconds', { text: 'X' }), { text: '\n\ttap "X"' });
  assert.equal(planInsert('x', {}), undefined);
  assert.equal(planInsert('x', { text: 'a\nb' }), undefined);
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
