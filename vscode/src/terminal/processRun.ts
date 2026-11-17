import { spawn } from 'node:child_process';

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
  stdout: NodeJS.ReadableStream | null;
  stderr: NodeJS.ReadableStream | null;
  stdin: NodeJS.WritableStream | null;
  kill(signal?: NodeJS.Signals): boolean;
  on(event: 'error', listener: (err: Error) => void): this;
  on(event: 'close', listener: (code: number | null, signal: NodeJS.Signals | null) => void): this;
}

export type SpawnFn = (command: string, args: string[], options: { cwd?: string; env: NodeJS.ProcessEnv }) => Child;

export const spawnNoShell: SpawnFn = (command, args, options) =>
  spawn(command, args, { ...options, shell: false, windowsHide: true, stdio: 'pipe' });

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

// RunQueue runs the tools of one terminal, one after another, and writes their output to it. A run asked for while another is running
// starts when that one ends, as a command typed into a busy shell did. What is typed into the terminal is echoed and sent to the running
// tool's input line by line; Ctrl+C interrupts it.
export class RunQueue {
  private running?: Child;
  private readonly waiting: RunSpec[] = [];
  private line = '';

  constructor(private readonly write: (text: string) => void, private readonly spawnFn: SpawnFn = spawnNoShell) {}

  get busy(): boolean { return this.running !== undefined; }

  run(spec: RunSpec): void {
    if (this.running) {
      this.write(toTerminal(`\n(waiting for the run above to end: ${[spec.command, ...spec.args].map(shownWord).join(' ')})\n`));
      this.waiting.push(spec);
      return;
    }
    this.start(spec);
  }

  private start(spec: RunSpec): void {
    this.write(toTerminal(`> ${[spec.command, ...spec.args].map(shownWord).join(' ')}\n`));
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
      this.write(toTerminal(`\n[${status}]\n`));
      this.running = undefined;
      this.line = '';
      this.next();
    };
    for (const stream of [child.stdout, child.stderr]) {
      stream?.setEncoding('utf8');
      stream?.on('data', (chunk: string) => this.write(toTerminal(chunk)));
    }
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
      } else if (c >= ' ') {
        this.line += c;
        this.write(c);
      }
    }
  }

  // interrupt sends the running tool SIGINT, as Ctrl+C in a shell does (probe record saves what it recorded).
  interrupt(): void {
    this.running?.kill('SIGINT');
  }

  // stop drops the runs that are waiting and interrupts the running one: the terminal is being closed.
  stop(): void {
    this.waiting.length = 0;
    this.interrupt();
  }
}
