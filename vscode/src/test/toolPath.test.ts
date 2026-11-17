import { test } from 'node:test';
import * as assert from 'node:assert/strict';
import * as fs from 'node:fs';
import * as path from 'node:path';
import { TOOL_SETTINGS, ToolPathInput, candidates, notFoundPath, pathDirs, resolveToolPath } from '../toolPath';

const posix = path.posix;
const win = path.win32;

// A POSIX machine with probe in /usr/local/bin and a repository at /repo that has bin/probe.
function posixInput(files: string[], over: Partial<ToolPathInput> = {}): ToolPathInput {
  return {
    configured: undefined,
    defaultName: 'probe',
    workspaceRoot: '/repo',
    useWorkspaceBinaries: false,
    trusted: true,
    platform: 'darwin',
    pathEnv: '/usr/local/bin:/usr/bin',
    isFile: (p: string) => files.includes(p),
    isAbsolute: posix.isAbsolute,
    join: posix.join,
    ...over,
  };
}

function winInput(files: string[], over: Partial<ToolPathInput> = {}): ToolPathInput {
  return {
    configured: undefined,
    defaultName: 'probe',
    workspaceRoot: 'C:\\repo',
    useWorkspaceBinaries: false,
    trusted: true,
    platform: 'win32',
    pathEnv: 'C:\\Windows\\System32;"C:\\Program Files\\probe";.',
    isFile: (p: string) => files.includes(p),
    isAbsolute: win.isAbsolute,
    join: win.join,
    ...over,
  };
}

test('the binary on PATH is found and returned as an absolute path', () => {
  assert.deepEqual(resolveToolPath(posixInput(['/usr/bin/probe'])), { path: '/usr/bin/probe' });
  assert.deepEqual(resolveToolPath(posixInput(['/usr/local/bin/probe', '/usr/bin/probe'])), { path: '/usr/local/bin/probe' });
});

test('a directory named like the binary is not the binary', () => {
  // isFile is false for a directory; only /usr/bin/probe is a file here.
  assert.deepEqual(resolveToolPath(posixInput(['/usr/bin/probe'])), { path: '/usr/bin/probe' });
  assert.deepEqual(resolveToolPath(posixInput([])), { notFound: 'probe', reason: 'not-on-path' });
});

test('empty and relative PATH entries are never searched', () => {
  assert.deepEqual(pathDirs('/a::.:bin:/b', 'linux', posix.isAbsolute), ['/a', '/b']);
  assert.deepEqual(pathDirs('C:\\a;;.;"C:\\Program Files\\x";bin', 'win32', win.isAbsolute), ['C:\\a', 'C:\\Program Files\\x']);
  // "." is on PATH and the working directory has a probe: it is not used.
  assert.deepEqual(resolveToolPath(posixInput(['probe'], { pathEnv: '.:/usr/bin' })), { notFound: 'probe', reason: 'not-on-path' });
});

test('on Windows the name is tried with .com, then .exe, as spawn does, and never in the current directory', () => {
  assert.deepEqual(candidates('C:\\bin', 'probe', 'win32', win.join), ['C:\\bin\\probe.com', 'C:\\bin\\probe.exe']);
  assert.deepEqual(candidates('C:\\bin', 'probe.exe', 'win32', win.join), ['C:\\bin\\probe.exe']);
  assert.deepEqual(candidates('/bin', 'probe', 'linux', posix.join), ['/bin/probe']);
  assert.deepEqual(resolveToolPath(winInput(['C:\\Program Files\\probe\\probe.exe'])), { path: 'C:\\Program Files\\probe\\probe.exe' });
  // A probe.exe in the repository (the working directory) is not found: "." on PATH is dropped.
  assert.deepEqual(resolveToolPath(winInput(['C:\\repo\\probe.exe', '.\\probe.exe'])), { notFound: 'probe', reason: 'not-on-path' });
});

test('a configured absolute path is used when it is a file', () => {
  assert.deepEqual(resolveToolPath(posixInput(['/opt/p/probe'], { configured: '/opt/p/probe' })), { path: '/opt/p/probe' });
  assert.deepEqual(resolveToolPath(posixInput([], { configured: '/opt/p' })), { notFound: '/opt/p', reason: 'not-a-file' });
  assert.deepEqual(resolveToolPath(winInput(['D:\\tools\\probe.exe'], { configured: 'D:\\tools\\probe.exe' })), { path: 'D:\\tools\\probe.exe' });
});

test('a configured bare name is looked up on PATH, and a configured relative path is refused', () => {
  assert.deepEqual(resolveToolPath(posixInput(['/usr/bin/probe-dev'], { configured: 'probe-dev' })), { path: '/usr/bin/probe-dev' });
  assert.deepEqual(resolveToolPath(posixInput(['/repo/tools/probe'], { configured: 'tools/probe' })), { notFound: 'tools/probe', reason: 'relative' });
  assert.deepEqual(resolveToolPath(winInput([], { configured: '.\\probe.exe' })), { notFound: '.\\probe.exe', reason: 'relative' });
});

test('bin/probe of the workspace is used only when turned on and trusted', () => {
  const files = ['/repo/bin/probe', '/usr/bin/probe'];
  assert.deepEqual(resolveToolPath(posixInput(files)), { path: '/usr/bin/probe' });
  assert.deepEqual(resolveToolPath(posixInput(files, { useWorkspaceBinaries: true })), { path: '/repo/bin/probe' });
  assert.deepEqual(resolveToolPath(posixInput(files, { useWorkspaceBinaries: true, trusted: false })), { path: '/usr/bin/probe' });
  assert.deepEqual(resolveToolPath(winInput(['C:\\repo\\bin\\probe.exe'], { useWorkspaceBinaries: true })), { path: 'C:\\repo\\bin\\probe.exe' });
});

test('not found: a bare name on macOS and Linux, an error on Windows, an error for a relative path', () => {
  assert.equal(notFoundPath({ notFound: 'probe', reason: 'not-on-path' }, 'probePath', 'darwin'), 'probe');
  assert.throws(() => notFoundPath({ notFound: 'probe', reason: 'not-on-path' }, 'probePath', 'win32'), /probe was not found on PATH.*flutterprobe\.probePath/);
  assert.throws(() => notFoundPath({ notFound: 'tools/probe', reason: 'relative' }, 'probePath', 'linux'), /absolute path/);
  assert.equal(notFoundPath({ notFound: '/opt/missing', reason: 'not-a-file' }, 'probePath', 'win32'), '/opt/missing');
});

// The manifest: the settings that name a binary, and the opt-in, are machine-scoped; the extension needs a trusted workspace.
test('the binary settings and the opt-in are machine-scoped, the opt-in is off, and the extension needs trust', () => {
  const pkg = JSON.parse(fs.readFileSync(path.join(__dirname, '..', '..', 'package.json'), 'utf8'));
  const conf = pkg.contributes.configuration;
  const props = Array.isArray(conf) ? Object.assign({}, ...conf.map((c: { properties: object }) => c.properties)) : conf.properties;
  assert.deepEqual([...TOOL_SETTINGS], ['probePath', 'convertPath', 'adbPath', 'flutterPath', 'useWorkspaceBinaries']);
  for (const key of TOOL_SETTINGS) {
    assert.equal(props[`flutterprobe.${key}`]?.scope, 'machine', `flutterprobe.${key} must be machine-scoped`);
  }
  assert.equal(props['flutterprobe.useWorkspaceBinaries'].default, false);
  assert.equal(pkg.capabilities?.untrustedWorkspaces?.supported, false);
  assert.ok((pkg.capabilities?.untrustedWorkspaces?.description ?? '').length > 0);
});

// config.ts and the terminal import vscode, so their use of the resolver is checked in their source.
test('config.ts gives the resolver the opt-in, the trust, the platform and PATH, and checks for a regular file', () => {
  const text = fs.readFileSync(path.join(__dirname, '..', '..', 'src', 'config.ts'), 'utf8');
  assert.match(text, /useWorkspaceBinaries: getSetting<boolean>\('useWorkspaceBinaries'\) === true/);
  assert.match(text, /trusted: vscode\.workspace\.isTrusted/);
  assert.match(text, /platform: process\.platform/);
  assert.match(text, /pathEnv: process\.env\.PATH/);
  assert.match(text, /fs\.statSync\(p\)\.isFile\(\)/);
  assert.match(text, /return notFoundPath\(r, setting, process\.platform\)/);
  assert.doesNotMatch(text, /path\.join\(ws, 'bin'/);
  const term = fs.readFileSync(path.join(__dirname, '..', '..', 'src', 'terminal', 'probeTerminal.ts'), 'utf8');
  assert.match(term, /const shown = process\.platform === 'win32' \? command : shellEscape\(command\);/);
  assert.match(term, /\[shown, \.\.\.args\.map\(shellEscape\)\]/);
  const probe = fs.readFileSync(path.join(__dirname, '..', '..', 'src', 'probe.ts'), 'utf8');
  assert.match(probe, /try \{\n    probePath = getProbePath\(\);\n  \} catch \{\n    return \[\]; \/\/ probe was not found/);
  assert.match(probe, /try \{\n    probePath = getProbePath\(\);\n  \} catch \{\n    return \[\]; \/\/ lint on save stays quiet/);
});
