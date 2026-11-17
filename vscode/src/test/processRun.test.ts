import { mock, test } from 'node:test';
import * as assert from 'node:assert/strict';
import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import { EventEmitter, PassThrough } from 'node:stream';
import { Child, MAX_LINE, MAX_WAITING, QueueOptions, RunQueue, RunSpec, SpawnFn, makeSignalTree, shownWord, toTerminal, waitAll } from '../terminal/processRun';

// The arguments a tool is run with reach it as they are: no shell parses them. Hostile text is built from code points (no raw format
// characters in the source).
const HOSTILE = [
  "'; touch PWNED; '", '$(touch PWNED)', '`touch PWNED`', 'a\\b', '"x;y|z&w"', '\u202Eevil\u200B', "it's here", '--name=a b',
  '*', '~', '#not a comment', '!!', '{a,b}', 'a  b', '%PATH%', '=ls', '', 'C:\\my tests\\', 'a\nb',
  // U+00D2 (UTF-8 C3 92) and U+2012 (E2 80 92): byte 0x92 is revert-line for bash's readline under LC_ALL=C, which made a typed quoted
  // command line run what followed the opening quote.
  'a\u00D2x;touch PWNED;\'', 'b\u2012x;touch PWNED;\'',
];

// runReal runs node with a script that prints its arguments as JSON, through a RunQueue with the real spawn, and returns the output.
function runReal(args: string[], env: Record<string, string> = {}): Promise<{ out: string; dir: string }> {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'probe-run-'));
  const script = path.join(dir, 'argv.js');
  fs.writeFileSync(script, 'process.stdout.write("ARGV" + JSON.stringify(process.argv.slice(2)) + "\\n")');
  return new Promise(resolve => {
    let out = '';
    const queue = new RunQueue(text => {
      out += text;
      if (/\[(exited|ended|could not)/.test(text)) { resolve({ out, dir }); }
    });
    queue.run({ command: process.execPath, args: [script, ...args], cwd: dir, env });
  });
}

test('every hostile argument reaches the tool as one argument, unchanged, and nothing runs', async () => {
  const { out, dir } = await runReal(HOSTILE);
  const line = out.split('\r\n').find(l => l.startsWith('ARGV'));
  assert.ok(line, out);
  assert.deepEqual(JSON.parse(line.slice(4)), HOSTILE);
  assert.match(out, /\[exited with code 0\]/);
  assert.equal(fs.existsSync(path.join(dir, 'PWNED')), false, 'nothing in an argument may run');
});

// The reviewer's case: a file name with one non-ASCII character, under the C locale, where a typed command line ran code.
test('under LC_ALL=C and LC_ALL=POSIX a non-ASCII file name is an argument, not keystrokes', async () => {
  for (const locale of ['C', 'POSIX']) {
    const payload = 'a\u00D2x;touch PWN8;\'';
    const { out, dir } = await runReal([payload], { LC_ALL: locale, LANG: locale });
    const line = out.split('\r\n').find(l => l.startsWith('ARGV'));
    assert.deepEqual(JSON.parse(line!.slice(4)), [payload], locale);
    assert.equal(fs.existsSync(path.join(dir, 'PWN8')), false, locale);
  }
});

test('a tool that cannot be started says so in the terminal and the next run still starts', async () => {
  const lines: string[] = [];
  await new Promise<void>(resolve => {
    const queue = new RunQueue(text => {
      lines.push(text);
      if (lines.join('').includes('ARGV')) { resolve(); }
    });
    queue.run({ command: path.join(os.tmpdir(), 'no-such-probe-binary'), args: ['test'], env: {} });
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'probe-run-'));
    const script = path.join(dir, 'argv.js');
    fs.writeFileSync(script, 'process.stdout.write("ARGV\\n")');
    queue.run({ command: process.execPath, args: [script], env: {} });
  });
  assert.match(lines.join(''), /could not run .*no-such-probe-binary.*ENOENT/);
});

// A stand-in child process, to check the queue's own behaviour without timing.
class FakeChild extends EventEmitter implements Child {
  stdout = new PassThrough();
  stderr = new PassThrough();
  stdin = new PassThrough();
  signals: (string | undefined)[] = [];
  kill(signal?: NodeJS.Signals): boolean { this.signals.push(signal); return true; }
}

function fakeQueue(opts: QueueOptions = {}): { queue: RunQueue; out: () => string; children: FakeChild[]; specs: RunSpec[] } {
  let text = '';
  const children: FakeChild[] = [];
  const specs: RunSpec[] = [];
  const spawnFn: SpawnFn = (command, args, options) => {
    specs.push({ command, args, cwd: options.cwd, env: options.env as Record<string, string> });
    const c = new FakeChild();
    children.push(c);
    return c;
  };
  return { queue: new RunQueue(t => { text += t; }, spawnFn, { graceMs: 20, ...opts }), out: () => text, children, specs };
}

const tick = (): Promise<void> => new Promise(r => setImmediate(r));

test('a second run waits for the first to end, then starts', async () => {
  const { queue, out, children, specs } = fakeQueue();
  queue.run({ command: '/bin/probe', args: ['test', 'a.probe'], env: {} });
  queue.run({ command: '/bin/probe', args: ['test', 'b.probe'], env: {} });
  assert.equal(children.length, 1);
  assert.match(out(), /waiting for the run above to end: \/bin\/probe test b\.probe/);
  children[0].emit('close', 1, null);
  assert.equal(children.length, 2);
  assert.deepEqual(specs[1].args, ['test', 'b.probe']);
  assert.match(out(), /\[exited with code 1\]/);
  assert.equal(queue.busy, true);
  children[1].emit('close', null, 'SIGINT');
  assert.match(out(), /\[ended by SIGINT\]/);
  assert.equal(queue.busy, false);
});

test('the run gets the cwd, and its env on top of the extension environment', () => {
  const { queue, specs } = fakeQueue();
  queue.run({ command: '/bin/probe', args: ['test'], cwd: '/ws', env: { PROBE_CLOUD_KEY: 'k' } });
  assert.equal(specs[0].cwd, '/ws');
  assert.equal(specs[0].env.PROBE_CLOUD_KEY, 'k');
  assert.equal(specs[0].env.PATH, process.env.PATH);
});

test('output is written with \\r\\n line ends; an error and a close end the run once', async () => {
  const { queue, out, children } = fakeQueue();
  queue.run({ command: '/bin/probe', args: [], env: {} });
  children[0].stdout.write('one\ntwo\r\nthree');
  children[0].stderr.write('err\n');
  await tick();
  assert.match(out(), /one\r\ntwo\r\nthree/);
  assert.match(out(), /err\r\n/);
  children[0].emit('error', new Error('boom'));
  children[0].emit('close', 1, null);
  assert.equal((out().match(/\[/g) ?? []).length, 1, out());
});

test('typed text is echoed and sent to the tool a line at a time; backspace edits; Ctrl+C interrupts', async () => {
  const { queue, out, children } = fakeQueue();
  queue.run({ command: '/bin/probe', args: ['init'], env: {} });
  let sent = '';
  children[0].stdin.on('data', d => { sent += d; });
  queue.input('yex');
  queue.input('\u007f');
  queue.input('s\r');
  await tick();
  assert.equal(sent, 'yes\n');
  assert.ok(out().includes('yex\b \bs\r\n'), out());
  queue.input('\u001b[A\u001bOPok\r'); // an arrow key and F1 are not typed as their letters
  await tick();
  assert.equal(sent, 'yes\nok\n');
  queue.input('\u0003');
  assert.deepEqual(children[0].signals, ['SIGINT']);
});

test('closing the terminal drops waiting runs and interrupts the running one', () => {
  const { queue, children } = fakeQueue();
  queue.run({ command: '/bin/probe', args: ['record'], env: {} });
  queue.run({ command: '/bin/probe', args: ['test'], env: {} });
  queue.stop();
  assert.deepEqual(children[0].signals, ['SIGINT']);
  children[0].emit('close', 0, null);
  assert.equal(children.length, 1, 'the waiting run is not started');
});

test('the command shown above the output cannot move the cursor or reorder the text', () => {
  assert.equal(shownWord('test'), 'test');
  assert.equal(shownWord('a b'), '"a b"');
  assert.equal(shownWord(''), '""');
  assert.equal(shownWord('C:\\users\\a.probe'), '"C:\\\\users\\\\a.probe"');
  assert.equal(shownWord('x\u001b[2Jy'), '"x\\u001b[2Jy"');
  assert.equal(shownWord('\u202Eevil'), '"\\u202eevil"');
  assert.equal(shownWord('a\u0085b'), '"a\\u0085b"');
  assert.equal(shownWord('say "hi"'), '"say \\"hi\\""');
  assert.equal(shownWord('\u00D2'), '\u00D2');
  assert.equal(toTerminal('a\nb\r\nc'), 'a\r\nb\r\nc');
});

// probeTerminal.ts imports vscode, so its wiring is checked in its source: output is kept until VS Code opens the terminal (events fired
// before Pseudoterminal.open are dropped), closing the terminal stops the queue, typed text goes to the queue, and nothing is typed into
// a shell.
test('the terminal keeps output until it opens, and closing it stops the run', () => {
  const text = fs.readFileSync(path.join(__dirname, '..', '..', 'src', 'terminal', 'probeTerminal.ts'), 'utf8');
  assert.match(text, /if \(opened\) \{ writeEmitter\.fire\(text\); return; \}\s*if \(earlySize >= MAX_EARLY\) \{ return; \}/);
  assert.match(text, /early\.push\(/);
  assert.match(text, /open: \(\) => \{\s*opened = true;\s*for \(const text of early\.splice\(0\)\) \{ writeEmitter\.fire\(text\); \}/);
  assert.match(text, /close: \(\) => \{\s*queue\.stop\(\);/);
  assert.match(text, /handleInput: data => queue\.input\(data\),/);
  assert.match(text, /createTerminal\(\{ name, pty/);
  assert.doesNotMatch(text, /sendText|shellPath|child_process/);
  const run = fs.readFileSync(path.join(__dirname, '..', '..', 'src', 'terminal', 'processRun.ts'), 'utf8');
  assert.match(run, /spawn\(command, args, \{ \.\.\.options, shell: false, windowsHide: true, stdio: 'pipe', detached: process\.platform !== 'win32' \}\)/);
});

const alive = (pid: number): boolean => { try { process.kill(pid, 0); return true; } catch { return false; } };
const sleep = (ms: number): Promise<void> => new Promise(r => setTimeout(r, ms));

// The review's case: a tool that ignores SIGINT and started a child of its own. Closing the terminal must leave neither running.
test('closing the terminal ends a tool that ignores SIGINT, and the process it started', { skip: process.platform === 'win32' }, async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'probe-orphan-'));
  const pids = path.join(dir, 'pids');
  const queue = new RunQueue(() => undefined, undefined, { graceMs: 300 });
  queue.run({ command: process.execPath, env: {}, cwd: dir, args: ['-e',
    `process.on('SIGINT', () => {}); const c = require('child_process').spawn('/bin/sleep', ['300'], { stdio: 'ignore' }); ` +
    `require('fs').writeFileSync(${JSON.stringify(pids)}, process.pid + ' ' + c.pid); setInterval(() => {}, 1000)`] });
  queue.run({ command: process.execPath, env: {}, cwd: dir, args: ['-e', `require('fs').writeFileSync('second-ran', '1')`] });
  for (let i = 0; i < 50 && !fs.existsSync(pids); i++) { await sleep(50); }
  const [child, grandchild] = fs.readFileSync(pids, 'utf8').split(' ').map(Number);
  queue.stop();
  await sleep(1200);
  const left = [alive(child), alive(grandchild)];
  for (const p of [child, grandchild]) { try { process.kill(p, 'SIGKILL'); } catch { /* ended */ } }
  assert.deepEqual(left, [false, false], 'the tool and its child must be ended');
  assert.equal(fs.existsSync(path.join(dir, 'second-ran')), false, 'a waiting run is dropped when the terminal closes');
});

test('Ctrl+C reaches the processes the tool started, as in a shell', { skip: process.platform === 'win32' }, async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'probe-group-'));
  const pids = path.join(dir, 'pids');
  const queue = new RunQueue(() => undefined);
  queue.run({ command: process.execPath, env: {}, cwd: dir, args: ['-e',
    `process.on('SIGINT', () => {}); const c = require('child_process').spawn('/bin/sleep', ['300'], { stdio: 'ignore' }); ` +
    `require('fs').writeFileSync(${JSON.stringify(pids)}, process.pid + ' ' + c.pid); setInterval(() => {}, 1000)`] });
  for (let i = 0; i < 50 && !fs.existsSync(pids); i++) { await sleep(50); }
  const [child, grandchild] = fs.readFileSync(pids, 'utf8').split(' ').map(Number);
  queue.input('\u0003');
  await sleep(500);
  const grandchildAlive = alive(grandchild);
  for (const p of [child, grandchild]) { try { process.kill(p, 'SIGKILL'); } catch { /* ended */ } }
  assert.equal(grandchildAlive, false, 'the SIGINT goes to the process group');
});

// The review's case: a tool that closes its input and keeps running, while the person types.
test('typing into a tool that closed its input is not an error of the extension', async () => {
  const errors: string[] = [];
  const onUncaught = (e: Error): void => { errors.push(String((e as NodeJS.ErrnoException).code ?? e.message)); };
  process.on('uncaughtException', onUncaught);
  try {
    let out = '';
    const done = new Promise<void>(resolve => {
      const queue = new RunQueue(t => { out += t; if (out.includes('[exited')) { resolve(); } });
      queue.run({ command: process.execPath, env: {}, args: ['-e', `process.stdin.destroy(); require('fs').closeSync(0); setTimeout(() => {}, 1200)`] });
      setTimeout(() => { queue.input('hello\r'); queue.input('again\r'); }, 500);
    });
    await done;
    await sleep(100);
  } finally {
    process.off('uncaughtException', onUncaught);
  }
  assert.deepEqual(errors, []);
});

test('the run\'s own env wins over the same name in VS Code\'s environment', () => {
  const old = process.env.PROBE_CLOUD_KEY;
  process.env.PROBE_CLOUD_KEY = 'from-vscode';
  try {
    const { queue, specs } = fakeQueue();
    queue.run({ command: '/bin/probe', args: ['test'], env: { PROBE_CLOUD_KEY: 'from-setting' } });
    assert.equal(specs[0].env.PROBE_CLOUD_KEY, 'from-setting');
  } finally {
    if (old === undefined) { delete process.env.PROBE_CLOUD_KEY; } else { process.env.PROBE_CLOUD_KEY = old; }
  }
});

test('a spawn that throws says so, and the next waiting run still starts', () => {
  let text = '';
  let calls = 0;
  const spawnFn: SpawnFn = () => {
    calls++;
    if (calls === 1) { throw new Error('spawn EINVAL'); }
    return new FakeChild();
  };
  const queue = new RunQueue(t => { text += t; }, spawnFn);
  queue.run({ command: '/bin/probe', args: ['a'], env: {} });
  assert.match(text, /could not start \/bin\/probe: spawn EINVAL/);
  assert.equal(calls, 1);
  queue.run({ command: '/bin/probe', args: ['b'], env: {} });
  assert.equal(calls, 2);
  // a throw while another run waits: the waiting one starts
  let n = 0;
  const kids: FakeChild[] = [];
  const q2 = new RunQueue(() => undefined, () => { n++; if (n === 2) { throw new Error('boom'); } const c = new FakeChild(); kids.push(c); return c; });
  q2.run({ command: '/bin/probe', args: ['1'], env: {} });
  q2.run({ command: '/bin/probe', args: ['2'], env: {} });
  q2.run({ command: '/bin/probe', args: ['3'], env: {} });
  kids[0].emit('close', 0, null);
  assert.equal(n, 3, 'run 2 failed to start and run 3 started');
});

test('stopping escalates to SIGKILL after the grace time, and not when the tool ended in time', async () => {
  const { queue, children } = fakeQueue({ graceMs: 30 });
  queue.run({ command: '/bin/probe', args: [], env: {} });
  queue.stop();
  assert.deepEqual(children[0].signals, ['SIGINT']);
  await sleep(80);
  assert.deepEqual(children[0].signals, ['SIGINT', 'SIGKILL']);
  const b = fakeQueue({ graceMs: 30 });
  b.queue.run({ command: '/bin/probe', args: [], env: {} });
  b.queue.stop();
  b.children[0].emit('close', null, 'SIGINT');
  await sleep(80);
  assert.deepEqual(b.children[0].signals, ['SIGINT']);
});

test('on Windows a .bat or .cmd tool is refused with a clear message', () => {
  const { queue, out, children } = fakeQueue({ platform: 'win32' });
  queue.run({ command: 'C:\\tools\\probe.CMD', args: ['test'], env: {} });
  assert.equal(children.length, 0);
  assert.match(out(), /is a batch file, which Windows runs only through cmd\.exe/);
  queue.run({ command: 'C:\\tools\\probe.exe', args: ['test'], env: {} });
  assert.equal(children.length, 1);
  const posix = fakeQueue({ platform: 'linux' });
  posix.queue.run({ command: '/opt/probe.cmd', args: [], env: {} });
  assert.equal(posix.children.length, 1, 'only Windows treats .cmd as a batch file');
});

test('the waiting runs and a typed line are capped', async () => {
  const { queue, out, children } = fakeQueue();
  for (let i = 0; i <= MAX_WAITING + 1; i++) { queue.run({ command: '/bin/probe', args: [String(i)], env: {} }); }
  assert.match(out(), new RegExp(`not started: ${MAX_WAITING} runs are already waiting`));
  let sent = '';
  children[0].stdin.on('data', d => { sent += d; });
  queue.input('x'.repeat(MAX_LINE + 10) + '\r');
  await tick();
  assert.equal(sent.length, MAX_LINE + 1);
});

// The review's deactivate case: the extension host exits soon after deactivate, taking any timer with it, so the tool must be gone by the
// time the promise resolves.
test('shutdown resolves only once a tool that ignores SIGINT has ended', { skip: process.platform === 'win32' }, async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'probe-deact-'));
  const pidFile = path.join(dir, 'pid');
  const queue = new RunQueue(() => undefined);
  queue.run({ command: process.execPath, env: {}, args: ['-e',
    `process.on('SIGINT', () => {}); require('fs').writeFileSync(${JSON.stringify(pidFile)}, String(process.pid)); setInterval(() => {}, 1000)`] });
  for (let i = 0; i < 50 && !fs.existsSync(pidFile); i++) { await sleep(50); }
  const pid = Number(fs.readFileSync(pidFile, 'utf8'));
  const started = Date.now();
  await queue.shutdown(300);
  const waited = Date.now() - started;
  const stillAlive = alive(pid);
  if (stillAlive) { process.kill(pid, 'SIGKILL'); }
  assert.equal(stillAlive, false, 'the tool must have ended when shutdown resolves');
  assert.ok(waited >= 250 && waited < 3000, `waited ${waited} ms`);
  assert.equal(queue.busy, false);
});

test('shutdown with nothing running resolves at once and signals nothing', async () => {
  const { queue, children } = fakeQueue();
  queue.run({ command: '/bin/probe', args: [], env: {} });
  children[0].emit('close', 0, null);
  await queue.shutdown(10);
  queue.stop();
  await sleep(40);
  assert.deepEqual(children[0].signals, [], 'a run that already ended is not signalled');
});

test('a later stop does not move the SIGKILL of a shutdown later', async () => {
  const { queue, children } = fakeQueue({ graceMs: 10_000 });
  queue.run({ command: '/bin/probe', args: [], env: {} });
  void queue.shutdown(30);
  queue.stop(); // the terminal's own close, after deactivate
  await sleep(80);
  assert.deepEqual(children[0].signals, ['SIGINT', 'SIGINT', 'SIGKILL']);
});

test('the default grace before SIGKILL is 15 s', () => {
  mock.timers.enable({ apis: ['setTimeout', 'Date'] });
  try {
    const children: FakeChild[] = [];
    const queue = new RunQueue(() => undefined, () => { const c = new FakeChild(); children.push(c); return c; });
    queue.run({ command: '/bin/probe', args: [], env: {} });
    queue.stop();
    mock.timers.tick(14_999);
    assert.deepEqual(children[0].signals, ['SIGINT']);
    mock.timers.tick(1);
    assert.deepEqual(children[0].signals, ['SIGINT', 'SIGKILL']);
  } finally {
    mock.timers.reset();
  }
});

test('on Windows the tree is ended with taskkill /T /F by its full path', () => {
  const calls: { command: string; args: string[] }[] = [];
  const fakeSpawn = (command: string, args: string[]) => { calls.push({ command, args }); return new EventEmitter(); };
  const child = new FakeChild();
  (child as unknown as { pid: number }).pid = 4242;
  makeSignalTree('win32', fakeSpawn, 'D:\\Win')(child, 'SIGINT');
  assert.deepEqual(calls, [{ command: 'D:\\Win\\System32\\taskkill.exe', args: ['/T', '/F', '/PID', '4242'] }]);
  assert.deepEqual(child.signals, [], 'Node\'s kill is not used on Windows');
  makeSignalTree('win32', fakeSpawn, undefined)(child, 'SIGKILL');
  assert.equal(calls[1].command, 'C:\\Windows\\System32\\taskkill.exe');
  // taskkill cannot be started: the tool is ended by Node's kill
  const failing = (): EventEmitter => { const e = new EventEmitter(); setImmediate(() => e.emit('error', new Error('ENOENT'))); return e; };
  makeSignalTree('win32', failing, 'C:\\Windows')(child, 'SIGINT');
  return new Promise<void>(resolve => setImmediate(() => { assert.deepEqual(child.signals, ['SIGINT']); resolve(); }));
});

test('a refused .cmd tool lets the next waiting run start', () => {
  const { queue, out, children } = fakeQueue({ platform: 'win32' });
  queue.run({ command: 'C:\\tools\\probe.exe', args: ['1'], env: {} });
  queue.run({ command: 'C:\\tools\\probe.cmd', args: ['2'], env: {} });
  queue.run({ command: 'C:\\tools\\probe.exe', args: ['3'], env: {} });
  children[0].emit('close', 0, null);
  assert.match(out(), /is a batch file/);
  assert.equal(children.length, 2, 'run 3 started after run 2 was refused');
});

test('deactivate waits for the tools to end, with a short grace and a cap', () => {
  const ext = fs.readFileSync(path.join(__dirname, '..', '..', 'src', 'extension.ts'), 'utf8');
  assert.match(ext, /export function deactivate\(\): Promise<void> \{\s*disposeDeviceView\(\);\s*return stopAllTerminals\(\);\s*\}/);
  const term = fs.readFileSync(path.join(__dirname, '..', '..', 'src', 'terminal', 'probeTerminal.ts'), 'utf8');
  assert.match(term, /export const DEACTIVATE_GRACE_MS = 1000;/);
  assert.match(term, /ended\.push\(queue\.shutdown\(DEACTIVATE_GRACE_MS\)\);/);
  assert.match(term, /return waitAll\(ended, DEACTIVATE_GRACE_MS \+ 2000\);/);
});

test('waitAll ends at the cap when a process never reports its exit, and at once when all have ended', async () => {
  mock.timers.enable({ apis: ['setTimeout'] });
  try {
    let done = false;
    void waitAll([new Promise<void>(() => undefined)], 3000).then(() => { done = true; });
    mock.timers.tick(2999);
    await Promise.resolve(); await Promise.resolve();
    assert.equal(done, false, 'not before the cap');
    mock.timers.tick(1);
    for (let i = 0; i < 5; i++) { await Promise.resolve(); }
    assert.equal(done, true, 'at the cap');
  } finally {
    mock.timers.reset();
  }
  let early = false;
  await waitAll([Promise.resolve(), Promise.resolve()], 60_000).then(() => { early = true; });
  assert.equal(early, true);
});

test('a run asked for after the terminal started closing is not started', () => {
  const { queue, out, children } = fakeQueue();
  queue.run({ command: '/bin/probe', args: ['1'], env: {} });
  void queue.shutdown(10);
  queue.run({ command: '/bin/probe', args: ['2'], env: {} });
  children[0].emit('close', null, 'SIGINT');
  assert.equal(children.length, 1);
  assert.match(out(), /not started: this terminal is closing/);
});
