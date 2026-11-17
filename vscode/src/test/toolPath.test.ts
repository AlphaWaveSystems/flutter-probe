import { test } from 'node:test';
import * as assert from 'node:assert/strict';
import * as fs from 'node:fs';
import * as path from 'node:path';
import { ToolPathInput, resolveToolPath } from '../toolPath';

const ws = '/repo';
const local = path.join(ws, 'bin', 'probe');
const input = (over: Partial<ToolPathInput>): ToolPathInput => ({
  configured: undefined,
  defaultName: 'probe',
  workspaceRoot: ws,
  trusted: true,
  exists: p => p === local,
  join: path.join,
  ...over,
});

test('a configured path is used as it is', () => {
  assert.equal(resolveToolPath(input({ configured: '/opt/probe/bin/probe' })), '/opt/probe/bin/probe');
});

test('bin/probe of a trusted workspace is used when no path is configured', () => {
  assert.equal(resolveToolPath(input({})), local);
  assert.equal(resolveToolPath(input({ configured: 'probe' })), local);
});

test('bin/probe of an untrusted workspace is never used', () => {
  assert.equal(resolveToolPath(input({ trusted: false })), 'probe');
});

test('without a workspace binary, the command on PATH', () => {
  assert.equal(resolveToolPath(input({ exists: () => false })), 'probe');
  assert.equal(resolveToolPath(input({ workspaceRoot: undefined })), 'probe');
  assert.equal(resolveToolPath(input({ defaultName: 'probe-convert', exists: () => false })), 'probe-convert');
});

// The manifest: the settings that name a binary are machine-scoped (a workspace cannot set them), and the extension does not run in
// an untrusted workspace.
test('the binary path settings are machine-scoped and the extension needs a trusted workspace', () => {
  const pkg = JSON.parse(fs.readFileSync(path.join(__dirname, '..', '..', 'package.json'), 'utf8'));
  const conf = pkg.contributes.configuration;
  const props = Array.isArray(conf) ? Object.assign({}, ...conf.map((c: { properties: object }) => c.properties)) : conf.properties;
  for (const key of ['probePath', 'convertPath', 'adbPath', 'flutterPath']) {
    assert.equal(props[`flutterprobe.${key}`]?.scope, 'machine', `flutterprobe.${key} must be machine-scoped`);
  }
  assert.equal(pkg.capabilities?.untrustedWorkspaces?.supported, false);
  assert.ok((pkg.capabilities?.untrustedWorkspaces?.description ?? '').length > 0);
});

// config.ts passes the workspace's trust to the resolver: it imports vscode, so this checks its source.
test('config.ts resolves the binaries with the workspace trust', () => {
  const text = fs.readFileSync(path.join(__dirname, '..', '..', 'src', 'config.ts'), 'utf8');
  assert.match(text, /trusted: vscode\.workspace\.isTrusted/);
  assert.match(text, /return toolPath\('probePath', 'probe'\)/);
  assert.match(text, /return toolPath\('convertPath', 'probe-convert'\)/);
  assert.doesNotMatch(text, /path\.join\(ws, 'bin'/);
});
