import * as vscode from 'vscode';
import * as fs from 'fs';
import * as path from 'path';
import { TestDefinition, RecipeDefinition } from '../types';
import { buildTestArgs, getProbeCommand } from '../probe';
import { runInTerminal } from '../terminal/probeTerminal';
import { getWorkspaceRoot } from '../config';

interface ProbeFile {
  uri: vscode.Uri;
  tests: TestDefinition[];
  recipes: RecipeDefinition[];
}

export class TestExplorerProvider implements vscode.TreeDataProvider<TestTreeItem> {
  private _onDidChangeTreeData = new vscode.EventEmitter<TestTreeItem | undefined>();
  readonly onDidChangeTreeData = this._onDidChangeTreeData.event;

  private probeFiles: ProbeFile[] = [];

  constructor(context: vscode.ExtensionContext) {
    this.scanWorkspace();

    const watcher = vscode.workspace.createFileSystemWatcher('**/*.probe');
    context.subscriptions.push(
      watcher,
      watcher.onDidChange(() => this.scanWorkspace()),
      watcher.onDidCreate(() => this.scanWorkspace()),
      watcher.onDidDelete(() => this.scanWorkspace()),
      vscode.commands.registerCommand('flutterprobe.refreshTests', () => this.scanWorkspace()),
      vscode.commands.registerCommand('flutterprobe.runTestByName', (test: TestDefinition) => this.runTest(test)),
      vscode.commands.registerCommand('flutterprobe.runTestFile', (filePath: string) => this.runFile(filePath)),
      vscode.commands.registerCommand('flutterprobe.runTestFolder', (folderPath: string) => this.runFolder(folderPath)),
    );
  }

  refresh(): void {
    this._onDidChangeTreeData.fire(undefined);
  }

  getTreeItem(element: TestTreeItem): vscode.TreeItem {
    return element;
  }

  getChildren(element?: TestTreeItem): TestTreeItem[] {
    if (!element) {
      return this.buildFolderTree();
    }

    if (element.type === 'folder') {
      return this.getChildrenForFolder(element.folderPath!);
    }

    if (element.type === 'file') {
      return this.getChildrenForFile(element.filePath!);
    }

    return [];
  }

  private buildFolderTree(): TestTreeItem[] {
    const ws = getWorkspaceRoot();
    if (!ws) { return []; }

    // Group files by their immediate parent folder relative to workspace
    const folders = new Map<string, vscode.Uri[]>();

    for (const pf of this.probeFiles) {
      const rel = path.relative(ws, path.dirname(pf.uri.fsPath));
      const folder = rel || '.';
      if (!folders.has(folder)) { folders.set(folder, []); }
      folders.get(folder)!.push(pf.uri);
    }

    const items: TestTreeItem[] = [];
    const sortedFolders = [...folders.keys()].sort();

    for (const folder of sortedFolders) {
      if (folders.get(folder)!.length === 1 && sortedFolders.length === 1) {
        // Single folder → show files directly
        return this.getChildrenForFolder(path.join(ws, folder));
      }
      const item = new TestTreeItem(
        `📁 ${folder}/`,
        vscode.TreeItemCollapsibleState.Expanded,
      );
      item.type = 'folder';
      item.folderPath = path.join(ws, folder);
      item.contextValue = 'testFolder';
      return items;
    }

    for (const folder of sortedFolders) {
      const item = new TestTreeItem(
        `📁 ${folder}/`,
        vscode.TreeItemCollapsibleState.Expanded,
      );
      item.type = 'folder';
      item.folderPath = path.join(ws, folder);
      item.contextValue = 'testFolder';
      items.push(item);
    }

    return items;
  }

  private getChildrenForFolder(folderPath: string): TestTreeItem[] {
    const items: TestTreeItem[] = [];

    for (const pf of this.probeFiles) {
      if (path.dirname(pf.uri.fsPath) === folderPath) {
        const hasRecipes = pf.recipes.length > 0 && pf.tests.length === 0;
        const label = hasRecipes
          ? `📄 ${path.basename(pf.uri.fsPath)} (${pf.recipes.length} recipe${pf.recipes.length > 1 ? 's' : ''})`
          : `📄 ${path.basename(pf.uri.fsPath)}`;

        const item = new TestTreeItem(
          label,
          (pf.tests.length + pf.recipes.length) > 0
            ? vscode.TreeItemCollapsibleState.Collapsed
            : vscode.TreeItemCollapsibleState.None,
        );
        item.type = 'file';
        item.filePath = pf.uri.fsPath;
        item.contextValue = 'testFile';
        item.command = {
          command: 'vscode.open',
          title: 'Open',
          arguments: [pf.uri],
        };
        items.push(item);
      }
    }

    // Also add subfolders
    const ws = getWorkspaceRoot()!;
    const subfolders = new Set<string>();
    for (const pf of this.probeFiles) {
      const dir = path.dirname(pf.uri.fsPath);
      if (dir !== folderPath && dir.startsWith(folderPath + path.sep)) {
        const relative = path.relative(folderPath, dir);
        const topLevel = relative.split(path.sep)[0];
        subfolders.add(topLevel);
      }
    }

    for (const sub of [...subfolders].sort()) {
      const subPath = path.join(folderPath, sub);
      const item = new TestTreeItem(
        `📁 ${sub}/`,
        vscode.TreeItemCollapsibleState.Collapsed,
      );
      item.type = 'folder';
      item.folderPath = subPath;
      item.contextValue = 'testFolder';
      items.push(item);
    }

    return items;
  }

  private getChildrenForFile(filePath: string): TestTreeItem[] {
    const pf = this.probeFiles.find(f => f.uri.fsPath === filePath);
    if (!pf) { return []; }

    const items: TestTreeItem[] = [];

    for (const test of pf.tests) {
      const tags = test.tags.length > 0 ? `  ${test.tags.map(t => `@${t}`).join(' ')}` : '';
      const item = new TestTreeItem(
        `● ${test.name}${tags}`,
        vscode.TreeItemCollapsibleState.None,
      );
      item.type = 'test';
      item.testDef = test;
      item.contextValue = 'test';
      item.command = {
        command: 'vscode.open',
        title: 'Go to test',
        arguments: [vscode.Uri.file(test.file), {
          selection: new vscode.Range(test.line - 1, 0, test.line - 1, 0),
        }],
      };
      items.push(item);
    }

    for (const recipe of pf.recipes) {
      const params = recipe.params.length > 0 ? ` (${recipe.params.join(', ')})` : '';
      const item = new TestTreeItem(
        `⚙ ${recipe.name}${params}`,
        vscode.TreeItemCollapsibleState.None,
      );
      item.type = 'recipe';
      item.command = {
        command: 'vscode.open',
        title: 'Go to recipe',
        arguments: [vscode.Uri.file(recipe.file), {
          selection: new vscode.Range(recipe.line - 1, 0, recipe.line - 1, 0),
        }],
      };
      items.push(item);
    }

    return items;
  }

  private async scanWorkspace(): Promise<void> {
    const ws = getWorkspaceRoot();
    if (!ws) { return; }

    const files = await vscode.workspace.findFiles('**/*.probe', '**/node_modules/**', 500);
    this.probeFiles = [];

    for (const uri of files) {
      try {
        const content = fs.readFileSync(uri.fsPath, 'utf-8');
        const pf = this.parseProbeFile(uri, content);
        this.probeFiles.push(pf);
      } catch {
        // Skip unreadable files
      }
    }

    this.refresh();
  }

  private parseProbeFile(uri: vscode.Uri, content: string): ProbeFile {
    const tests: TestDefinition[] = [];
    const recipes: RecipeDefinition[] = [];
    const lines = content.split('\n');

    for (let i = 0; i < lines.length; i++) {
      const line = lines[i];

      const testMatch = line.match(/^test\s+"([^"]+)"/);
      if (testMatch) {
        const tags: string[] = [];
        // Check next line for tags
        if (i + 1 < lines.length) {
          const tagLine = lines[i + 1];
          const tagMatches = tagLine.matchAll(/@(\w+)/g);
          for (const m of tagMatches) {
            tags.push(m[1]);
          }
        }
        tests.push({
          name: testMatch[1],
          file: uri.fsPath,
          line: i + 1,
          tags,
        });
      }

      const recipeMatch = line.match(/^recipe\s+"([^"]+)"(?:\s*\(([^)]*)\))?/);
      if (recipeMatch) {
        const params = recipeMatch[2]
          ? recipeMatch[2].split(',').map(p => p.trim()).filter(Boolean)
          : [];
        recipes.push({
          name: recipeMatch[1],
          file: uri.fsPath,
          line: i + 1,
          params,
        });
      }
    }

    return { uri, tests, recipes };
  }

  private runTest(test: TestDefinition): void {
    const args = buildTestArgs(test.file, { autoConfirm: true });
    args.push('--name', test.name);
    runInTerminal('Probe: Test', getProbeCommand(), args, getWorkspaceRoot());
  }

  private runFile(filePath: string): void {
    const args = buildTestArgs(filePath, { autoConfirm: true });
    runInTerminal('Probe: Test', getProbeCommand(), args, getWorkspaceRoot());
  }

  private runFolder(folderPath: string): void {
    const args = buildTestArgs(folderPath, { autoConfirm: true });
    runInTerminal('Probe: Test', getProbeCommand(), args, getWorkspaceRoot());
  }
}

export class TestTreeItem extends vscode.TreeItem {
  type?: 'folder' | 'file' | 'test' | 'recipe';
  folderPath?: string;
  filePath?: string;
  testDef?: TestDefinition;
}
