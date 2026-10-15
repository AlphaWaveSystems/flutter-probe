import { test } from 'node:test';
import * as assert from 'node:assert/strict';
import { fileRunArgs, lastLines, outcomeOf, parseReport, parseTestDeclarations } from '../testing/probeTests';

test('finds each test block with its line and the tags on the line after it', () => {
  const content = [
    'app "com.example"',
    '',
    'test "sign in"',
    '  @smoke @auth',
    '  tap "Sign in"',
    '',
    'test "shows the list"',
    '  see "Items"',
  ].join('\n');
  assert.deepEqual(parseTestDeclarations(content), [
    { name: 'sign in', line: 3, tags: ['smoke', 'auth'] },
    { name: 'shows the list', line: 7, tags: [] },
  ]);
});

test('CRLF line endings do not change the names or the tags', () => {
  const content = 'test "a"\r\n  @x\r\n  tap "b"\r\n';
  assert.deepEqual(parseTestDeclarations(content), [{ name: 'a', line: 1, tags: ['x'] }]);
});

test('reads the rows of a probe JSON report', () => {
  const report = JSON.stringify({
    total_tests: 2,
    results: [
      { name: 'sign in', file: 'tests/a.probe', passed: true, skipped: false, duration_ms: 120 },
      { name: 'shows the list', file: 'tests/a.probe', passed: false, skipped: false, duration_ms: 40, error: 'Items not found' },
    ],
  });
  const rows = parseReport(report);
  assert.equal(rows.length, 2);
  assert.deepEqual(outcomeOf(rows, 'sign in'), { status: 'passed', durationMs: 120 });
  assert.deepEqual(outcomeOf(rows, 'shows the list'), { status: 'failed', durationMs: 40, message: 'Items not found' });
});

test('refuses output that is not a probe test report', () => {
  assert.throws(() => parseReport('{"hello":1}'), /not a probe test report/);
  assert.throws(() => parseReport('not json'));
});

test('a data-driven test is failed when any row failed, and skipped only when every row was skipped', () => {
  const rows = parseReport(JSON.stringify({
    results: [
      { name: 'login', passed: true, skipped: false, duration_ms: 10 },
      { name: 'login', passed: false, skipped: false, duration_ms: 20, error: 'wrong password' },
      { name: 'logout', passed: false, skipped: true, duration_ms: 0 },
      { name: 'logout', passed: false, skipped: true, duration_ms: 0 },
    ],
  }));
  assert.equal(outcomeOf(rows, 'login')?.status, 'failed');
  assert.equal(outcomeOf(rows, 'logout')?.status, 'skipped');
  assert.equal(outcomeOf(rows, 'missing'), undefined);
});

test('a row that failed without an error text still reports a failure', () => {
  const rows = parseReport(JSON.stringify({ results: [{ name: 'x', passed: false, skipped: false }] }));
  assert.deepEqual(outcomeOf(rows, 'x'), { status: 'failed', durationMs: 0, message: 'the test failed' });
});

test('the run command runs the file unattended, with the JSON report written to the given path', () => {
  assert.deepEqual(fileRunArgs('/p/tests/a.probe', '/tmp/r/report.json'), [
    '/p/tests/a.probe', '--yes', '--format', 'json', '--output', '/tmp/r/report.json',
  ]);
  assert.deepEqual(fileRunArgs('/p/tests/a.probe', '/tmp/r/report.json', 'emulator-5554'), [
    '/p/tests/a.probe', '--yes', '--format', 'json', '--output', '/tmp/r/report.json', '--device', 'emulator-5554',
  ]);
});

test('lastLines keeps the end of the output, without blank lines', () => {
  assert.equal(lastLines('a\n\n b \nc\nd', 2), 'c\nd');
  assert.equal(lastLines('  \n'), '');
});
