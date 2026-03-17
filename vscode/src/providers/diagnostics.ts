import * as vscode from 'vscode';
import { runLint } from '../probe';
import { getSetting } from '../config';

export class DiagnosticsProvider {
  private diagnosticCollection: vscode.DiagnosticCollection;
  private pending = new Map<string, NodeJS.Timeout>();

  constructor(context: vscode.ExtensionContext) {
    this.diagnosticCollection = vscode.languages.createDiagnosticCollection('flutterprobe');
    context.subscriptions.push(this.diagnosticCollection);

    // Lint on save
    context.subscriptions.push(
      vscode.workspace.onDidSaveTextDocument((doc) => {
        if (doc.languageId === 'probescript' && getSetting<boolean>('autoLint')) {
          this.lintDocument(doc);
        }
      })
    );

    // Lint on open
    context.subscriptions.push(
      vscode.workspace.onDidOpenTextDocument((doc) => {
        if (doc.languageId === 'probescript' && getSetting<boolean>('autoLint')) {
          this.lintDocument(doc);
        }
      })
    );

    // Clear diagnostics when file is closed
    context.subscriptions.push(
      vscode.workspace.onDidCloseTextDocument((doc) => {
        this.diagnosticCollection.delete(doc.uri);
      })
    );

    // Lint already-open probe files
    for (const doc of vscode.workspace.textDocuments) {
      if (doc.languageId === 'probescript' && getSetting<boolean>('autoLint')) {
        this.lintDocument(doc);
      }
    }
  }

  async lintDocument(document: vscode.TextDocument): Promise<void> {
    const filePath = document.uri.fsPath;

    // Debounce
    const existing = this.pending.get(filePath);
    if (existing) { clearTimeout(existing); }

    this.pending.set(filePath, setTimeout(async () => {
      this.pending.delete(filePath);

      const results = await runLint(filePath);
      const diagnostics: vscode.Diagnostic[] = [];

      for (const r of results) {
        const line = Math.max(0, r.line - 1);
        const col = Math.max(0, r.column - 1);
        const range = new vscode.Range(line, col, line, Number.MAX_SAFE_INTEGER);
        const severity = r.severity === 'error' ? vscode.DiagnosticSeverity.Error :
                         r.severity === 'warning' ? vscode.DiagnosticSeverity.Warning :
                         vscode.DiagnosticSeverity.Information;

        const diag = new vscode.Diagnostic(range, r.message, severity);
        diag.source = 'FlutterProbe';
        diagnostics.push(diag);
      }

      this.diagnosticCollection.set(document.uri, diagnostics);
    }, 500));
  }
}
