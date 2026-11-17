import * as vscode from 'vscode';
import { secretFlagIn } from '../secretArgs';
import { RunQueue } from './processRun';

// What a terminal keeps of the output written before VS Code opens it.
const MAX_EARLY = 1024 * 1024;

// Each named terminal runs the tools itself (RunQueue in processRun.ts): the binary is started with its arguments as a list and no shell,
// so no shell parses, expands or line-edits a file name, under any terminal shell or locale. The output stays in the terminal after the
// run ends, and the next run of the same name goes to the same terminal.
interface Entry { terminal: vscode.Terminal; queue: RunQueue }
const terminals = new Map<string, Entry>();

function getOrCreateTerminal(name: string): Entry {
  const existing = terminals.get(name);
  if (existing && vscode.window.terminals.includes(existing.terminal)) {
    existing.terminal.show();
    return existing;
  }
  terminals.delete(name);

  const writeEmitter = new vscode.EventEmitter<string>();
  // What is written before VS Code opens the terminal would be dropped (Pseudoterminal.onDidWrite), so it is kept until then.
  let opened = false;
  const early: string[] = [];
  let earlySize = 0;
  const write = (text: string): void => {
    if (opened) { writeEmitter.fire(text); return; }
    if (earlySize >= MAX_EARLY) { return; } // a terminal that is never opened keeps at most MAX_EARLY characters
    earlySize += text.length;
    early.push(earlySize >= MAX_EARLY ? text + '\r\n(output before the terminal opened was cut here)\r\n' : text);
  };
  const queue = new RunQueue(write);
  const pty: vscode.Pseudoterminal = {
    onDidWrite: writeEmitter.event,
    open: () => {
      opened = true;
      for (const text of early.splice(0)) { writeEmitter.fire(text); }
    },
    close: () => {
      queue.stop();
      terminals.delete(name);
    },
    handleInput: data => queue.input(data),
  };
  const terminal = vscode.window.createTerminal({ name, pty, iconPath: new vscode.ThemeIcon('beaker') });
  const entry = { terminal, queue };
  terminals.set(name, entry);
  terminal.show();
  return entry;
}

// runInTerminal runs the tool in the terminal of that name. The secrets go in env, the environment of the process, never in args: the
// command line of a process can be read by other processes of the same user (see secretArgs.ts).
export function runInTerminal(
  name: string,
  command: string,
  args: string[],
  cwd: string | undefined,
  env: Record<string, string>
): vscode.Terminal {
  const secret = secretFlagIn(args);
  if (secret) {
    throw new Error(`${secret} must not be on the command line: pass it in the environment`);
  }
  const entry = getOrCreateTerminal(name);
  entry.queue.run({ command, args, cwd, env });
  return entry.terminal;
}

export function disposeTerminal(name: string): void {
  const entry = terminals.get(name);
  if (entry) {
    entry.terminal.dispose();
    terminals.delete(name);
  }
}

export function disposeAllTerminals(): void {
  for (const [name, { terminal }] of terminals) {
    terminal.dispose();
    terminals.delete(name);
  }
}
