import * as vscode from 'vscode';
import { SessionManagerProvider } from './sessionManager';

export class ProbeCodeLensProvider implements vscode.CodeLensProvider {
  private _onDidChangeCodeLenses = new vscode.EventEmitter<void>();
  readonly onDidChangeCodeLenses = this._onDidChangeCodeLenses.event;

  constructor(private sessionManager?: SessionManagerProvider) {
    vscode.workspace.onDidChangeTextDocument(() => this._onDidChangeCodeLenses.fire());
  }

  provideCodeLenses(document: vscode.TextDocument): vscode.CodeLens[] {
    if (document.languageId !== 'probescript') {
      return [];
    }

    const lenses: vscode.CodeLens[] = [];
    const sessions = this.sessionManager?.getSessions() ?? [];

    for (let i = 0; i < document.lineCount; i++) {
      const line = document.lineAt(i);
      const text = line.text;

      // Test definitions
      const testMatch = text.match(/^test\s+"([^"]+)"/);
      if (testMatch) {
        const range = new vscode.Range(i, 0, i, text.length);

        // Run button
        lenses.push(new vscode.CodeLens(range, {
          title: '▶ Run',
          command: 'flutterprobe.runTest',
          tooltip: 'Run this test',
        }));

        // Per-session run buttons
        for (const session of sessions) {
          lenses.push(new vscode.CodeLens(range, {
            title: `▶ ${session.name}`,
            command: 'flutterprobe.runTestOnDevice',
            arguments: [document.uri.fsPath, testMatch[1], session],
            tooltip: `Run on ${session.name}`,
          }));
        }

        // Verbose run
        lenses.push(new vscode.CodeLens(range, {
          title: 'Debug',
          command: 'flutterprobe.runTestVerbose',
          arguments: [document.uri.fsPath, testMatch[1]],
          tooltip: 'Run with verbose output',
        }));

        continue;
      }

      // Recipe definitions
      const recipeMatch = text.match(/^recipe\s+"([^"]+)"/);
      if (recipeMatch) {
        const range = new vscode.Range(i, 0, i, text.length);
        lenses.push(new vscode.CodeLens(range, {
          title: '⚙ Recipe',
          command: '',
          tooltip: `Recipe: ${recipeMatch[1]}`,
        }));
        continue;
      }

      // Hook definitions
      const hookMatch = text.match(/^(before each test|after each test|on failure)/);
      if (hookMatch) {
        const range = new vscode.Range(i, 0, i, text.length);
        const hookType = hookMatch[1] === 'before each test' ? 'runs before each test' :
                         hookMatch[1] === 'after each test' ? 'runs after each test' :
                         'runs when a test fails';
        lenses.push(new vscode.CodeLens(range, {
          title: `⚙ Hook (${hookType})`,
          command: '',
          tooltip: hookType,
        }));
      }
    }

    // "Run All" at top of file if it contains tests
    if (lenses.length > 0) {
      lenses.unshift(new vscode.CodeLens(new vscode.Range(0, 0, 0, 0), {
        title: '▶ Run All',
        command: 'flutterprobe.runFile',
        tooltip: 'Run all tests in this file',
      }));
    }

    return lenses;
  }
}
