// Pure helpers for the Testing API explorer. Nothing here imports vscode, so the unit tests run under node --test.

/** One `test "<name>"` block in a .probe file. */
export interface TestDeclaration {
  name: string;
  /** 1-based line of the `test "<name>"` line. */
  line: number;
  tags: string[];
}

/** One row of the CLI's JSON report (`probe test --format json`). A data-driven test has one row per row of data. */
export interface ReportRow {
  name: string;
  passed: boolean;
  skipped: boolean;
  durationMs: number;
  error?: string;
}

export type TestOutcome =
  | { status: 'passed'; durationMs: number }
  | { status: 'failed'; durationMs: number; message: string }
  | { status: 'skipped' };

const TEST_LINE = /^test\s+"([^"]+)"/;

// parseTestDeclarations finds the test blocks of one .probe file the way the Tests view does: a line that starts with
// test "<name>", and its tags on the line after it.
export function parseTestDeclarations(content: string): TestDeclaration[] {
  const lines = content.split('\n');
  const declarations: TestDeclaration[] = [];
  lines.forEach((line, i) => {
    const match = TEST_LINE.exec(line);
    if (!match) {
      return;
    }
    const next = i + 1 < lines.length ? lines[i + 1] : '';
    const tags = [...next.matchAll(/@(\w+)/g)].map(m => m[1]);
    declarations.push({ name: match[1], line: i + 1, tags });
  });
  return declarations;
}

// parseReport reads the JSON report that `probe test --format json -o <file>` writes. A document without a results
// array is not a report, and it is refused rather than read as "no results".
export function parseReport(text: string): ReportRow[] {
  const doc: unknown = JSON.parse(text);
  const results = typeof doc === 'object' && doc !== null ? (doc as { results?: unknown }).results : undefined;
  if (!Array.isArray(results)) {
    throw new Error('the output is not a probe test report');
  }
  return results.map(toRow);
}

function toRow(value: unknown): ReportRow {
  const r = (typeof value === 'object' && value !== null ? value : {}) as Record<string, unknown>;
  return {
    name: typeof r.name === 'string' ? r.name : '',
    passed: r.passed === true,
    skipped: r.skipped === true,
    durationMs: typeof r.duration_ms === 'number' ? r.duration_ms : 0,
    error: typeof r.error === 'string' && r.error !== '' ? r.error : undefined,
  };
}

// outcomeOf combines the rows of one test: failed when any row failed, skipped when every row was skipped, otherwise
// passed. It returns undefined when the report has no row for the test.
export function outcomeOf(rows: ReportRow[], name: string): TestOutcome | undefined {
  const mine = rows.filter(r => r.name === name);
  if (mine.length === 0) {
    return undefined;
  }
  const durationMs = mine.reduce((sum, r) => sum + r.durationMs, 0);
  const failed = mine.find(r => !r.passed && !r.skipped);
  if (failed) {
    return { status: 'failed', durationMs, message: failed.error ?? 'the test failed' };
  }
  if (mine.every(r => r.skipped)) {
    return { status: 'skipped' };
  }
  return { status: 'passed', durationMs };
}

// fileRunArgs is the probe test command for one file: unattended (--yes), with the JSON report written to reportPath.
// The CLI has no per-test filter in this version, so a run executes the whole file and the caller reads the rows it needs.
export function fileRunArgs(file: string, reportPath: string, device?: string): string[] {
  const args = [file, '--yes', '--format', 'json', '--output', reportPath];
  if (device) {
    args.push('--device', device);
  }
  return args;
}

// lastLines keeps the end of a run's output for an error message: the reason a run stopped is at the end.
export function lastLines(text: string, count = 6): string {
  return text
    .split('\n')
    .map(line => line.trim())
    .filter(line => line !== '')
    .slice(-count)
    .join('\n');
}
