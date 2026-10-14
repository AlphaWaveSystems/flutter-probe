import { HubInfo, LineBuffer, parseHubLine } from './hubLine';

/** Minimal child-process surface so tests can inject a fake. */
export interface ChildLike {
  stdout: { on(ev: 'data', cb: (d: Buffer | string) => void): unknown } | null;
  stderr: { on(ev: 'data', cb: (d: Buffer | string) => void): unknown } | null;
  on(ev: 'error', cb: (err: Error) => void): unknown;
  on(ev: 'close', cb: (code: number | null, signal: string | null) => void): unknown;
  kill(): boolean;
}

export type SpawnFn = (command: string, args: string[], cwd?: string) => ChildLike;

export class HubStartError extends Error {
  constructor(public readonly kind: 'not-found' | 'exited' | 'timeout' | 'spawn', message: string) {
    super(message);
  }
}

export interface HubProcessOptions {
  spawn: SpawnFn;
  /** Receives stderr lines with the token redacted. */
  onStderr?: (line: string) => void;
  /** Called when the process ends on its own after a successful start. */
  onExit?: (code: number | null) => void;
  startTimeoutMs?: number;
}

/** Owns one `probe device view --json` child. The token only leaves this object via start()'s result. */
export class HubProcess {
  private child: ChildLike | undefined;
  private token = '';
  private ended = false;
  private killed = false;

  constructor(private readonly opts: HubProcessOptions) {}

  get running(): boolean {
    return !!this.child && !this.ended;
  }

  start(command: string, args: string[], cwd?: string): Promise<HubInfo> {
    return new Promise<HubInfo>((resolve, reject) => {
      let settled = false;
      const stderrTail: string[] = [];
      let timer: ReturnType<typeof setTimeout> | undefined;
      const finish = (fn: () => void) => {
        if (settled) { return; }
        settled = true;
        if (timer) { clearTimeout(timer); }
        fn();
      };

      let child: ChildLike;
      try {
        child = this.opts.spawn(command, args, cwd);
      } catch (e) {
        reject(new HubStartError('spawn', `Could not start ${command}: ${(e as Error).message}`));
        return;
      }
      this.child = child;

      timer = setTimeout(() => {
        finish(() => {
          this.stop();
          reject(new HubStartError('timeout', 'Timed out waiting for the device view hub to start.'));
        });
      }, this.opts.startTimeoutMs ?? 20000);

      const out = new LineBuffer();
      const err = new LineBuffer();

      const handleStdout = (lines: string[]) => {
        for (const line of lines) {
          if (settled) { return; }
          const info = parseHubLine(line);
          if (info) {
            this.token = info.token;
            finish(() => resolve(info));
          }
        }
      };
      const handleStderr = (lines: string[]) => {
        for (const raw of lines) {
          const line = this.redact(raw);
          stderrTail.push(line);
          if (stderrTail.length > 20) { stderrTail.shift(); }
          this.opts.onStderr?.(line);
        }
      };

      child.stdout?.on('data', d => handleStdout(out.push(d.toString())));
      child.stderr?.on('data', d => handleStderr(err.push(d.toString())));

      child.on('error', e => {
        this.ended = true;
        const code = (e as NodeJS.ErrnoException).code;
        if (code === 'ENOENT') {
          finish(() => reject(new HubStartError('not-found', `probe binary not found: ${command}`)));
        } else {
          finish(() => reject(new HubStartError('spawn', `Could not start ${command}: ${e.message}`)));
        }
      });

      child.on('close', code => {
        handleStdout(out.flush());
        handleStderr(err.flush());
        this.ended = true;
        if (!settled) {
          finish(() => reject(new HubStartError(
            'exited',
            `probe exited with code ${code ?? 'null'} before the hub was ready.` +
              (stderrTail.length ? `\n${stderrTail.slice(-5).join('\n')}` : ''),
          )));
        } else if (!this.killed) {
          this.opts.onExit?.(code);
        }
      });
    });
  }

  /** Terminate the child (idempotent). */
  stop(): void {
    if (this.killed) { return; }
    this.killed = true;
    if (this.child && !this.ended) {
      try { this.child.kill(); } catch { /* already gone */ }
    }
  }

  private redact(line: string): string {
    return this.token ? line.split(this.token).join('***') : line;
  }
}
