import * as vscode from 'vscode';

const terminals = new Map<string, vscode.Terminal>();

export function getOrCreateTerminal(name: string, cwd?: string): vscode.Terminal {
  const existing = terminals.get(name);
  if (existing) {
    // Check if the terminal is still alive
    const allTerminals = vscode.window.terminals;
    if (allTerminals.includes(existing)) {
      existing.show();
      return existing;
    }
    terminals.delete(name);
  }

  const terminal = vscode.window.createTerminal({
    name,
    cwd,
  });
  terminals.set(name, terminal);
  terminal.show();
  return terminal;
}

export function runInTerminal(
  name: string,
  command: string,
  args: string[],
  cwd?: string
): vscode.Terminal {
  const terminal = getOrCreateTerminal(name, cwd);
  const fullCommand = [command, ...args.map(shellEscape)].join(' ');
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
  const terminal = terminals.get(name);
  if (terminal) {
    terminal.dispose();
    terminals.delete(name);
  }
}

export function disposeAllTerminals(): void {
  for (const [name, terminal] of terminals) {
    terminal.dispose();
    terminals.delete(name);
  }
}
