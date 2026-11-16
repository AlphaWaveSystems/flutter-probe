import * as vscode from 'vscode';
import * as fs from 'fs';
import { getWorkspaceRoot } from '../config';
// The completion words come from the ProbeScript docs (see scripts/gen-keywords.mjs): the lead of each command and the
// reserved words. The examples with arguments are in the snippets.
import { PROBESCRIPT_COMMANDS, PROBESCRIPT_KEYWORDS } from './probescriptKeywords.generated';

const PERMISSION_NAMES = [
  'notifications', 'camera', 'location', 'microphone',
  'storage', 'contacts', 'phone', 'calendar', 'sms', 'bluetooth',
];

export class ProbeCompletionProvider implements vscode.CompletionItemProvider {
  provideCompletionItems(
    document: vscode.TextDocument,
    position: vscode.Position,
  ): vscode.CompletionItem[] {
    const lineText = document.lineAt(position).text;
    const textBefore = lineText.substring(0, position.character);

    // Permission completions
    if (/(?:allow|deny)\s+permission\s+"?$/.test(textBefore)) {
      return PERMISSION_NAMES.map(p => {
        const item = new vscode.CompletionItem(p, vscode.CompletionItemKind.EnumMember);
        item.detail = 'Permission name';
        return item;
      });
    }

    // Tag completions (after @)
    if (textBefore.trimStart().startsWith('@')) {
      return this.getWorkspaceTags(document);
    }

    // Recipe call completions
    const recipes = this.getWorkspaceRecipes(document);
    const keywordCompletions = this.getKeywordCompletions(textBefore);

    return [...keywordCompletions, ...recipes];
  }

  private getKeywordCompletions(textBefore: string): vscode.CompletionItem[] {
    const trimmed = textBefore.trimStart().toLowerCase();
    const words = PROBESCRIPT_KEYWORDS
      .filter(kw => kw.startsWith(trimmed) || trimmed === '')
      .map(kw => {
        const item = new vscode.CompletionItem(kw, vscode.CompletionItemKind.Keyword);
        item.detail = 'ProbeScript';
        return item;
      });
    // A command form with its arguments as tab stops (`press <key>`, `see no requests`). A form that is only a keyword word is
    // already offered above.
    const commands = PROBESCRIPT_COMMANDS
      .filter(cmd => !PROBESCRIPT_KEYWORDS.includes(cmd.label))
      .filter(cmd => cmd.label.toLowerCase().startsWith(trimmed) || trimmed === '')
      .map(cmd => {
        const item = new vscode.CompletionItem(cmd.label, vscode.CompletionItemKind.Snippet);
        item.insertText = new vscode.SnippetString(cmd.insert);
        item.detail = 'ProbeScript command';
        return item;
      });
    return [...words, ...commands];
  }

  private getWorkspaceRecipes(document: vscode.TextDocument): vscode.CompletionItem[] {
    const items: vscode.CompletionItem[] = [];
    const ws = getWorkspaceRoot();
    if (!ws) { return items; }

    // Parse the current document for `use` statements
    const content = document.getText();
    const useMatches = content.matchAll(/^use\s+"([^"]+)"/gm);
    const usedFiles = new Set<string>();

    for (const m of useMatches) {
      const usePath = m[1];
      const fullPath = usePath.startsWith('/') ? usePath : require('path').join(ws, usePath);
      usedFiles.add(fullPath);
    }

    // Also scan the current document for recipes
    this.extractRecipesFromContent(content, items);

    // Scan used files
    for (const filePath of usedFiles) {
      try {
        const fileContent = fs.readFileSync(filePath, 'utf-8');
        this.extractRecipesFromContent(fileContent, items);
      } catch {
        // File not found
      }
    }

    return items;
  }

  private extractRecipesFromContent(content: string, items: vscode.CompletionItem[]): void {
    const recipeMatches = content.matchAll(/^recipe\s+"([^"]+)"(?:\s*\(([^)]*)\))?/gm);
    for (const m of recipeMatches) {
      const name = m[1];
      const params = m[2] ? m[2].split(',').map(p => p.trim()) : [];

      const item = new vscode.CompletionItem(name, vscode.CompletionItemKind.Function);
      item.detail = params.length > 0 ? `recipe(${params.join(', ')})` : 'recipe';
      item.documentation = `Call recipe "${name}"`;

      if (params.length > 0) {
        const snippetArgs = params.map((p, i) => `"\${${i + 1}:${p}}"`).join(' and ');
        item.insertText = new vscode.SnippetString(`${name} ${snippetArgs}`);
      }

      items.push(item);
    }
  }

  private getWorkspaceTags(document: vscode.TextDocument): vscode.CompletionItem[] {
    const tags = new Set<string>();

    // Scan all open documents and the current document
    for (const doc of vscode.workspace.textDocuments) {
      if (doc.languageId === 'probescript') {
        const tagMatches = doc.getText().matchAll(/@(\w+)/g);
        for (const m of tagMatches) {
          tags.add(m[1]);
        }
      }
    }

    // Also scan current document
    const tagMatches = document.getText().matchAll(/@(\w+)/g);
    for (const m of tagMatches) {
      tags.add(m[1]);
    }

    return [...tags].map(tag => {
      const item = new vscode.CompletionItem(tag, vscode.CompletionItemKind.Constant);
      item.detail = 'Tag';
      return item;
    });
  }
}
