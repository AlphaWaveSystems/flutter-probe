import * as vscode from 'vscode';
import * as fs from 'fs';
import { getWorkspaceRoot } from '../config';

const PROBESCRIPT_KEYWORDS = [
  'test', 'recipe', 'use', 'before each test', 'after each test', 'on failure',
  'if', 'else', 'repeat', 'Examples:', 'run dart:',
  'open the app', 'tap on', 'tap', 'type', 'see', "don't see",
  'wait until', 'wait for', 'wait',
  'swipe up', 'swipe down', 'swipe left', 'swipe right',
  'scroll up', 'scroll down', 'scroll up until', 'scroll down until',
  'go back', 'long press', 'double tap',
  'take a screenshot called', 'dump the widget tree',
  'clear app data', 'restart the app',
  'allow permission', 'deny permission',
  'grant all permissions', 'revoke all permissions',
  'toggle wifi', 'toggle airplane mode',
  'when the app calls', 'respond with',
  'pause for', 'log',
];

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
    const trimmed = textBefore.trimStart();
    return PROBESCRIPT_KEYWORDS
      .filter(kw => kw.startsWith(trimmed.toLowerCase()) || trimmed === '')
      .map(kw => {
        const item = new vscode.CompletionItem(kw, vscode.CompletionItemKind.Keyword);
        item.detail = 'ProbeScript';
        return item;
      });
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
