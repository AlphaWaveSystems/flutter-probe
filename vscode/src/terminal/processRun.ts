import { spawn } from 'node:child_process';
import * as path from 'node:path';

// One run of a tool: the binary by its absolute path and its arguments as a list. It is started with no shell (shell: false), so no
// shell parses, expands or line-edits any argument: a file name with a quote, a $, a % or a non-ASCII character reaches the tool as it
// is, on every system and under any terminal shell or locale. (Typing a quoted command line into a shell could not be made safe: under a
// single-byte locale bash's readline reads a byte of a UTF-8 character, such as 0x92 in Ò, as a key binding that discards the line.)
export interface RunSpec {
  command: string;
  args: string[];
  cwd?: string;
  env: Record<string, string>;
}

// The parts of a child process the queue uses (a ChildProcess, or a stand-in in tests).
export interface Child {
  pid?: number;
  stdout: NodeJS.ReadableStream | null;
  stderr: NodeJS.ReadableStream | null;
  stdin: NodeJS.WritableStream | null;
  kill(signal?: NodeJS.Signals): boolean;
  on(event: 'error', listener: (err: Error) => void): this;
  on(event: 'close', listener: (code: number | null, signal: NodeJS.Signals | null) => void): this;
}

export type SpawnFn = (command: string, args: string[], options: { cwd?: string; env: NodeJS.ProcessEnv }) => Child;

// On macOS and Linux the tool starts in a process group of its own (detached), so a signal reaches the processes it starts too (adb, flutter,
// xcrun), as Ctrl+C in a shell reaches the whole foreground job.
export const spawnNoShell: SpawnFn = (command, args, options) =>
  spawn(command, args, { ...options, shell: false, windowsHide: true, stdio: 'pipe', detached: process.platform !== 'win32' });

// Signaller sends a signal to a tool and the processes it started.
export type Signaller = (child: Child, signal: 'SIGINT' | 'SIGKILL') => void;

// signalTree: on macOS and Linux, the signal goes to the tool's process group. Windows has no Ctrl+C for another process (Node's kill
// ends it at once, whatever the signal), so there the tool and every process it started are ended with taskkill /T /F, run by its full
// path (a bare name could be found in the current folder first).
export function makeSignalTree(
  platform: string,
  spawnKill: (command: string, args: string[]) => { on(event: 'error', listener: (err: Error) => void): unknown },
  systemRoot: string | undefined,
): Signaller {
  return (child, signal) => {
    if (child.pid === undefined) { child.kill(signal); return; }
    if (platform === 'win32') {
      const taskkill = path.win32.join(systemRoot ?? 'C:\\Windows', 'System32', 'taskkill.exe');
      spawnKill(taskkill, ['/T', '/F', '/PID', String(child.pid)]).on('error', () => child.kill(signal));
      return;
    }
    try {
      process.kill(-child.pid, signal);
    } catch {
      child.kill(signal);
    }
  };
}

export const signalTree: Signaller = makeSignalTree(
  process.platform,
  (command, args) => spawn(command, args, { shell: false, windowsHide: true, stdio: 'ignore' }),
  process.env.SystemRoot,
);

// The limits of what a terminal keeps for a person: runs waiting, and the characters of one typed line.
export const MAX_WAITING = 100;
export const MAX_LINE = 64 * 1024;

// toTerminal is text for a terminal: a bare \n only moves the cursor down, so every line break becomes \r\n.
export function toTerminal(text: string): string {
  return text.replace(/\r?\n/g, '\r\n');
}

// shownWord is one word of the command as shown above its output: for reading only, never parsed. A control character, the line and
// paragraph separators, and the bidirectional controls are written as \uXXXX, so a file name cannot move the cursor, recolor the
// terminal or reorder what is shown; a word with a space or a quote is shown in JSON quotes.
const SHOWN_ESCAPED = /[\u0000-\u001f\u007f-\u009f\u2028\u2029\u200e\u200f\u202a-\u202e\u2066-\u2069]/g;

export function shownWord(word: string): string {
  SHOWN_ESCAPED.lastIndex = 0;
  if (word !== '' && !/[\s"'\\]/.test(word) && !SHOWN_ESCAPED.test(word)) { return word; }
  const escaped = word
    .replace(/[\\"]/g, c => '\\' + c)
    .replace(SHOWN_ESCAPED, c => '\\u' + c.charCodeAt(0).toString(16).padStart(4, '0'));
  return `"${escaped}"`;
}

export interface QueueOptions {
  signal?: Signaller;
  // How long a stopped tool has to end after SIGINT before it and its processes get SIGKILL.
  graceMs?: number;
  platform?: string;
}

// RunQueue runs the tools of one terminal, one after another, and writes their output to it. A run asked for while another is running
// starts when that one ends, as a command typed into a busy shell did. What is typed into the terminal is echoed and sent to the running
// tool's input line by line; Ctrl+C interrupts it (on Windows it ends it, see signalTree).
export class RunQueue {
  private running?: Child;
  private readonly waiting: RunSpec[] = [];
  private line = '';
  private killTimer?: ReturnType<typeof setTimeout>;
  private killAt = Infinity;
  private readonly endWaiters: (() => void)[] = [];
  private readonly signal: Signaller;
  private readonly graceMs: number;
  private readonly platform: string;

  constructor(private readonly write: (text: string) => void, private readonly spawnFn: SpawnFn = spawnNoShell, opts: QueueOptions = {}) {
    this.signal = opts.signal ?? signalTree;
    // probe record, on SIGINT, waits up to 5 s for the agent to stop recording and then writes the file: 15 s leaves it room to save.
    this.graceMs = opts.graceMs ?? 15000;
    this.platform = opts.platform ?? process.platform;
  }

  get busy(): boolean { return this.running !== undefined; }

  run(spec: RunSpec): void {
    if (this.running) {
      if (this.waiting.length >= MAX_WAITING) {
        this.write(toTerminal(`\n(not started: ${MAX_WAITING} runs are already waiting in this terminal)\n`));
        return;
      }
      this.write(toTerminal(`\n(waiting for the run above to end: ${[spec.command, ...spec.args].map(shownWord).join(' ')})\n`));
      this.waiting.push(spec);
      return;
    }
    this.start(spec);
  }

  private start(spec: RunSpec): void {
    this.write(toTerminal(`> ${[spec.command, ...spec.args].map(shownWord).join(' ')}\n`));
    // Windows runs a .bat or .cmd file only through cmd.exe, which would parse the arguments: refused rather than run through a shell.
    if (this.platform === 'win32' && /\.(bat|cmd)$/i.test(spec.command)) {
      this.write(toTerminal(`not started: ${shownWord(spec.command)} is a batch file, which Windows runs only through cmd.exe; the extension runs tools with no shell. Set the path to the .exe instead.\n`));
      this.next();
      return;
    }
    let child: Child;
    try {
      child = this.spawnFn(spec.command, spec.args, { cwd: spec.cwd, env: { ...process.env, ...spec.env } });
    } catch (err) {
      this.write(toTerminal(`could not start ${shownWord(spec.command)}: ${(err as Error).message}\n`));
      this.next();
      return;
    }
    this.running = child;
    let ended = false;
    const end = (status: string): void => {
      if (ended) { return; }
      ended = true;
      clearTimeout(this.killTimer);
      this.killTimer = undefined;
      this.killAt = Infinity;
      for (const resolve of this.endWaiters.splice(0)) { resolve(); }
      this.write(toTerminal(`\n[${status}]\n`));
      this.running = undefined;
      this.line = '';
      this.next();
    };
    for (const stream of [child.stdout, child.stderr]) {
      stream?.setEncoding('utf8');
      stream?.on('data', (chunk: string) => this.write(toTerminal(chunk)));
    }
    // A tool that closes its input while it runs: what is typed then is dropped, not an error of the extension (EPIPE).
    child.stdin?.on('error', () => undefined);
    child.on('error', err => end(`could not run ${shownWord(spec.command)}: ${err.message}`));
    child.on('close', (code, signal) => end(signal ? `ended by ${signal}` : `exited with code ${code}`));
  }

  private next(): void {
    const spec = this.waiting.shift();
    if (spec) { this.start(spec); }
  }

  // input is what the person typed into the terminal.
  input(data: string): void {
    // An arrow, Home or function key arrives as an escape sequence: it is dropped, not typed as its letters.
    for (const c of data.replace(/\u001b(\[[0-9;?]*[ -/]*[@-~]|O.|.)?/g, '')) {
      if (c === '\u0003') { this.interrupt(); continue; }
      if (!this.running) { continue; }
      if (c === '\r') {
        this.write('\r\n');
        this.running.stdin?.write(this.line + '\n');
        this.line = '';
      } else if (c === '\u007f') {
        if (this.line) { this.line = [...this.line].slice(0, -1).join(''); this.write('\b \b'); }
      } else if (c >= ' ' && this.line.length < MAX_LINE) {
        this.line += c;
        this.write(c);
      }
    }
  }

  // interrupt sends the running tool and the processes it started SIGINT, as Ctrl+C in a shell does (probe record saves what it recorded).
  // On Windows it ends them (see signalTree).
  interrupt(): void {
    if (this.running) { this.signal(this.running, 'SIGINT'); }
  }

  // stop drops the runs that are waiting and interrupts the running one: the terminal is being closed. A tool that has not ended after
  // graceMs (it ignores SIGINT, or hangs) gets SIGKILL, with the processes it started, so nothing is left running.
  stop(): void {
    void this.shutdown(this.graceMs);
  }

  // shutdown is stop with its own grace time, and a promise that resolves when the running tool has ended (at once when none runs). The
  // extension's deactivate uses a short grace: the window is closing, VS Code waits only briefly, and the extension host exits soon after,
  // taking the SIGKILL timer with it. A later stop never moves the SIGKILL later than one already set.
  shutdown(graceMs: number): Promise<void> {
    this.waiting.length = 0;
    const child = this.running;
    if (!child) { return Promise.resolve(); }
    const ended = new Promise<void>(resolve => this.endWaiters.push(resolve));
    this.signal(child, 'SIGINT');
    const at = Date.now() + graceMs;
    if (at < this.killAt) {
      this.killAt = at;
      clearTimeout(this.killTimer);
      this.killTimer = setTimeout(() => { if (this.running === child) { this.signal(child, 'SIGKILL'); } }, graceMs);
    }
    return ended;
  }
}
