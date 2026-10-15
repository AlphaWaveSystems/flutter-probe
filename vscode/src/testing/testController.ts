import * as vscode from 'vscode';
import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';
import { spawn } from 'child_process';
import { getProbeCommand } from '../probe';
import { getWorkspaceRoot } from '../config';
import {
  ReportRow,
  TestDeclaration,
  TestOutcome,
  fileRunArgs,
  lastLines,
  outcomeOf,
  parseReport,
  parseTestDeclarations,
} from './probeTests';

// The native Testing view (VS Code Testing API). Tests are the `test "<name>"` blocks the Tests view finds. A run executes
// each file once with the CLI and reads the outcomes from its JSON report, so the pass or fail comes from probe itself.

const CONTROLLER_ID = 'flutterprobe';

interface FileRun {
  file: vscode.Uri;
  tests: vscode.TestItem[];
}

export function registerTestingController(context: vscode.ExtensionContext): void {
  const controller = vscode.tests.createTestController(CONTROLLER_ID, 'FlutterProbe');
  context.subscriptions.push(controller);
  controller.refreshHandler = () => discoverAll(controller);
  controller.createRunProfile(
    'Run',
    vscode.TestRunProfileKind.Run,
    (request, token) => runRequest(controller, request, token),
    true,
  );

  const watcher = vscode.workspace.createFileSystemWatcher('**/*.probe');
  context.subscriptions.push(
    watcher,
    watcher.onDidCreate(uri => updateFile(controller, uri)),
    watcher.onDidChange(uri => updateFile(controller, uri)),
    watcher.onDidDelete(uri => controller.items.delete(uri.toString())),
  );
  void discoverAll(controller);
}

async function discoverAll(controller: vscode.TestController): Promise<void> {
  controller.items.replace([]);
  if (!getWorkspaceRoot()) {
    return;
  }
  const files = await vscode.workspace.findFiles('**/*.probe', '**/node_modules/**', 500);
  for (const uri of files) {
    updateFile(controller, uri);
  }
}

// updateFile makes the file's item match its current tests. A file with no tests has no item.
function updateFile(controller: vscode.TestController, uri: vscode.Uri): void {
  let declarations: TestDeclaration[] = [];
  try {
    declarations = parseTestDeclarations(fs.readFileSync(uri.fsPath, 'utf-8'));
  } catch {
    // Unreadable files have no tests in the explorer.
  }
  if (declarations.length === 0) {
    controller.items.delete(uri.toString());
    return;
  }
  let fileItem = controller.items.get(uri.toString());
  if (!fileItem) {
    fileItem = controller.createTestItem(uri.toString(), path.basename(uri.fsPath), uri);
    controller.items.add(fileItem);
  }
  fileItem.children.replace(
    declarations.map(d => {
      const child = controller.createTestItem(`${uri.toString()}::${d.line}`, d.name, uri);
      child.range = new vscode.Range(d.line - 1, 0, d.line - 1, 0);
      return child;
    }),
  );
}

async function runRequest(
  controller: vscode.TestController,
  request: vscode.TestRunRequest,
  token: vscode.CancellationToken,
): Promise<void> {
  const run = controller.createTestRun(request);
  for (const group of selectedByFile(controller, request)) {
    if (token.isCancellationRequested) {
      break;
    }
    for (const item of group.tests) {
      run.started(item);
    }
    try {
      const rows = await runFile(group.file, run, token);
      for (const item of group.tests) {
        reportOutcome(run, item, outcomeOf(rows, item.label));
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      for (const item of group.tests) {
        run.errored(item, new vscode.TestMessage(message));
      }
    }
  }
  run.end();
}

// selectedByFile groups the requested tests by file. Requesting a file requests all of its tests.
function selectedByFile(controller: vscode.TestController, request: vscode.TestRunRequest): FileRun[] {
  const excluded = new Set<vscode.TestItem>(request.exclude ?? []);
  const included: vscode.TestItem[] = [];
  if (request.include) {
    included.push(...request.include);
  } else {
    controller.items.forEach(item => included.push(item));
  }

  const groups = new Map<string, FileRun>();
  const addTest = (item: vscode.TestItem, file: vscode.Uri) => {
    const key = file.toString();
    let group = groups.get(key);
    if (!group) {
      group = { file, tests: [] };
      groups.set(key, group);
    }
    if (!group.tests.includes(item)) {
      group.tests.push(item);
    }
  };
  for (const item of included) {
    if (excluded.has(item) || !item.uri) {
      continue;
    }
    const fileUri = item.uri;
    if (item.id === fileUri.toString()) {
      item.children.forEach(child => {
        if (!excluded.has(child)) {
          addTest(child, fileUri);
        }
      });
    } else {
      addTest(item, fileUri);
    }
  }
  return [...groups.values()];
}

function reportOutcome(run: vscode.TestRun, item: vscode.TestItem, outcome: TestOutcome | undefined): void {
  if (!outcome) {
    run.errored(item, new vscode.TestMessage('the report has no result for this test'));
    return;
  }
  switch (outcome.status) {
    case 'passed':
      run.passed(item, outcome.durationMs);
      return;
    case 'skipped':
      run.skipped(item);
      return;
    case 'failed': {
      const message = new vscode.TestMessage(outcome.message);
      if (item.uri && item.range) {
        message.location = new vscode.Location(item.uri, item.range);
      }
      run.failed(item, message, outcome.durationMs);
      return;
    }
  }
}

// runFile runs one file with the CLI and returns the rows of its report. The output goes to the run's output, and the
// report is read from a temporary directory that is removed afterwards. A run that writes no report fails with the end of
// its output, which is where probe says why it stopped.
function runFile(file: vscode.Uri, run: vscode.TestRun, token: vscode.CancellationToken): Promise<ReportRow[]> {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'flutterprobe-run-'));
  const reportPath = path.join(dir, 'report.json');
  const device = vscode.workspace.getConfiguration('flutterprobe').get<string>('defaultDevice', '');
  const child = spawn(getProbeCommand(), fileRunArgs(file.fsPath, reportPath, device || undefined), {
    cwd: getWorkspaceRoot(),
  });
  let output = '';
  const onOutput = (chunk: Buffer) => {
    const text = chunk.toString('utf-8');
    output = (output + text).slice(-4000);
    run.appendOutput(text.replace(/\r?\n/g, '\r\n'));
  };
  child.stdout.on('data', onOutput);
  child.stderr.on('data', onOutput);
  const cancel = token.onCancellationRequested(() => child.kill());

  return new Promise<ReportRow[]>((resolve, reject) => {
    child.on('error', err => {
      cancel.dispose();
      cleanup(dir);
      reject(err);
    });
    child.on('close', () => {
      cancel.dispose();
      try {
        resolve(parseReport(fs.readFileSync(reportPath, 'utf-8')));
      } catch {
        reject(new Error(lastLines(output) || 'probe wrote no report'));
      } finally {
        cleanup(dir);
      }
    });
  });
}

function cleanup(dir: string): void {
  fs.rmSync(dir, { recursive: true, force: true });
}
