import { test } from 'node:test';
import * as assert from 'node:assert/strict';
import * as fs from 'fs';
import * as path from 'path';

// probe test has no per-test filter in this version: it defines no --name flag, so any run that passes one fails with
// "unknown flag". This guard keeps the extension from passing it again. This file is skipped: it names the flag itself.
const SELF = 'noPerTestFlag.test.ts';

function sourceFiles(dir: string): string[] {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap(entry => {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      return sourceFiles(full);
    }
    return entry.name.endsWith('.ts') && entry.name !== SELF ? [full] : [];
  });
}

test('the extension never passes --name to probe test', () => {
  const src = path.resolve(__dirname, '..', '..', 'src');
  const flag = /['"]--name['"]/;
  const offenders = sourceFiles(src)
    .filter(file => flag.test(fs.readFileSync(file, 'utf-8')))
    .map(file => path.relative(src, file));
  assert.deepEqual(offenders, []);
});
