import * as vscode from 'vscode';
import { envFingerprint, secretFlagIn, terminalAction } from '../secretArgs';

// Each terminal with the fingerprint of the environment it was made with: a terminal's environment is fixed when it is created, so a run
// whose secrets differ gets a new terminal.
const terminals = new Map<string, { terminal: vscode.Terminal; env: string }>();

export function getOrCreateTerminal(name: string, cwd?: string, env: Record<string, string> = {}): vscode.Terminal {
  const fingerprint = envFingerprint(env);
  const existing = terminals.get(name);
  const action = terminalAction(
    existing && { alive: vscode.window.terminals.includes(existing.terminal), fingerprint: existing.env },
    env,
  );
  if (existing && action === 'reuse') {
    existing.terminal.show();
    return existing.terminal;
  }
  if (existing && action === 'replace') { existing.terminal.dispose(); }
  terminals.delete(name);

  const terminal = vscode.window.createTerminal({
    name,
    cwd,
    env,
  });
  terminals.set(name, { terminal, env: fingerprint });
  terminal.show();
  return terminal;
}

// runInTerminal types the command into a terminal, so the command line ends up in shell history: it must hold no secret. The secrets go
// in env, the environment the terminal is created with (see secretArgs.ts).
export function runInTerminal(
  name: string,
  command: string,
  args: string[],
  cwd: string | undefined,
  env: Record<string, string>
): vscode.Terminal {
  const secret = secretFlagIn(args);
  if (secret) {
    throw new Error(`${secret} must not be on the command line: pass it in the terminal's environment`);
  }
  const terminal = getOrCreateTerminal(name, cwd, env);
  // The command is quoted like its arguments (a path with a space stays one word). On Windows it is left as it is: the terminal's shell
  // is not known (PowerShell takes a quoted path as a string, cmd.exe does not read single quotes), and the path is absolute (toolPath.ts).
  const shown = process.platform === 'win32' ? command : shellEscape(command);
  const fullCommand = [shown, ...args.map(shellEscape)].join(' ');
  terminal.sendText(fullCommand);
  return terminal;
}

function shellEscape(arg: string): string {
  if (/^[a-zA-Z0-9_./:=-]+$/.test(arg)) {
    return arg;
  }
  return `'${arg.replace(/'/g, "'\\''")}'`;
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
