import { test } from 'node:test';
import * as assert from 'node:assert/strict';
import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import { EventEmitter, PassThrough } from 'node:stream';
import { Child, RunQueue, RunSpec, SpawnFn, shownWord, toTerminal } from '../terminal/processRun';

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

function fakeQueue(): { queue: RunQueue; out: () => string; children: FakeChild[]; specs: RunSpec[] } {
  let text = '';
  const children: FakeChild[] = [];
  const specs: RunSpec[] = [];
  const spawnFn: SpawnFn = (command, args, options) => {
    specs.push({ command, args, cwd: options.cwd, env: options.env as Record<string, string> });
    const c = new FakeChild();
    children.push(c);
    return c;
  };
  return { queue: new RunQueue(t => { text += t; }, spawnFn), out: () => text, children, specs };
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
  assert.match(text, /const write = \(text: string\): void => \{ if \(opened\) \{ writeEmitter\.fire\(text\); \} else \{ early\.push\(text\); \} \};/);
  assert.match(text, /open: \(\) => \{\s*opened = true;\s*for \(const text of early\.splice\(0\)\) \{ writeEmitter\.fire\(text\); \}/);
  assert.match(text, /close: \(\) => \{\s*queue\.stop\(\);/);
  assert.match(text, /handleInput: data => queue\.input\(data\),/);
  assert.match(text, /createTerminal\(\{ name, pty/);
  assert.doesNotMatch(text, /sendText|shellPath|child_process/);
  const run = fs.readFileSync(path.join(__dirname, '..', '..', 'src', 'terminal', 'processRun.ts'), 'utf8');
  assert.match(run, /spawn\(command, args, \{ \.\.\.options, shell: false,/);
});
