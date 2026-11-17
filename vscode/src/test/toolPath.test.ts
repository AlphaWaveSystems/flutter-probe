import { test } from 'node:test';
import * as assert from 'node:assert/strict';
import * as fs from 'node:fs';
import * as path from 'node:path';
import { TOOL_SETTINGS, ToolPathInput, candidates, commandWord, isRegularFile, notFoundPath, pathDirs, quoteArg, quotePosix, resolveToolPath } from '../toolPath';
import * as os from 'node:os';
import { execFileSync } from 'node:child_process';

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

test('not found on PATH is an error on every system, and so is a relative path', () => {
  // A bare name could only be found through an empty or relative PATH entry, which is the workspace: no fallback to it.
  assert.throws(() => notFoundPath({ notFound: 'probe', reason: 'not-on-path' }, 'probePath'), /probe was not found on PATH.*flutterprobe\.probePath/);
  assert.throws(() => notFoundPath({ notFound: 'tools/probe', reason: 'relative' }, 'probePath'), /absolute path/);
  assert.equal(notFoundPath({ notFound: '/opt/missing', reason: 'not-a-file' }, 'probePath'), '/opt/missing');
});

// The real check against the file system: a file is one, a directory, a missing path and a link to a directory are not.
test('isRegularFile on real files', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'toolpath-'));
  const file = path.join(dir, 'probe');
  fs.writeFileSync(file, '#!/bin/sh\n');
  fs.mkdirSync(path.join(dir, 'probe-dir'));
  assert.equal(isRegularFile(file), true);
  assert.equal(isRegularFile(path.join(dir, 'probe-dir')), false);
  assert.equal(isRegularFile(path.join(dir, 'missing')), false);
  if (process.platform !== 'win32') {
    fs.symlinkSync(path.join(dir, 'probe-dir'), path.join(dir, 'link-to-dir'));
    fs.symlinkSync(file, path.join(dir, 'link-to-file'));
    assert.equal(isRegularFile(path.join(dir, 'link-to-dir')), false);
    assert.equal(isRegularFile(path.join(dir, 'link-to-file')), true);
  }
});

test('the command is quoted for the shell of the terminal', () => {
  const ps = 'C:\\Program Files\\PowerShell\\7\\pwsh.exe';
  const cmd = 'C:\\Windows\\System32\\cmd.exe';
  assert.equal(commandWord('/usr/local/bin/probe', 'darwin', '/bin/zsh'), '/usr/local/bin/probe');
  assert.equal(commandWord('/Users/a b/probe', 'darwin', '/bin/zsh'), "'/Users/a b/probe'");
  assert.equal(commandWord('C:\\tools\\probe.exe', 'win32', ps), 'C:\\tools\\probe.exe');
  assert.equal(commandWord("C:\\Program Files\\o'p\\probe.exe", 'win32', ps), "& 'C:\\Program Files\\o''p\\probe.exe'");
  assert.equal(commandWord('C:\\Program Files\\probe\\probe.exe', 'win32', cmd), '"C:\\Program Files\\probe\\probe.exe"');
  assert.equal(commandWord('C:\\Program Files\\probe\\probe.exe', 'win32', 'C:\\Program Files\\Git\\bin\\bash.exe'), "'C:\\Program Files\\probe\\probe.exe'");
  assert.equal(commandWord('C:\\Program Files\\probe\\probe.exe', 'win32', ''), 'C:\\Program Files\\probe\\probe.exe');
  assert.equal(quotePosix("it's"), "'it'\\''s'");
});

// PowerShell reads the typographic quotes ‘ ’ as single quotes (about_Quoting_Rules): they are doubled, so the string cannot end early.
test('PowerShell: typographic single quotes are doubled, and quotes it may read are refused', () => {
  const ps = 'powershell.exe';
  assert.equal(commandWord('C:\\Users\\O\u2019Brien\\probe.exe', 'win32', ps), "& 'C:\\Users\\O\u2019\u2019Brien\\probe.exe'");
  assert.equal(commandWord('C:\\x\u2019; calc; \u2018\\probe.exe', 'win32', ps), "& 'C:\\x\u2019\u2019; calc; \u2018\u2018\\probe.exe'");
  assert.equal(commandWord("C:\\a'b c\\probe.exe", 'win32', ps), "& 'C:\\a''b c\\probe.exe'");
  assert.throws(() => commandWord('C:\\x\u201A y\\probe.exe', 'win32', ps), /may read as a quote/);
  assert.throws(() => commandWord('C:\\x\u201B y\\probe.exe', 'win32', ps), /may read as a quote/);
});

test('cmd.exe: a path with % is refused, other characters are inside double quotes', () => {
  const cmd = 'cmd.exe';
  assert.throws(() => commandWord('C:\\%COMSPEC% x\\probe.exe', 'win32', cmd), /% or "/);
  assert.equal(commandWord('C:\\a&calc b\\probe.exe', 'win32', cmd), '"C:\\a&calc b\\probe.exe"');
});

// Under a shell whose quoting is not handled (or none detected), a path with a metacharacter is refused, never typed.
test('an unknown shell gets no path with a metacharacter', () => {
  for (const shell of ['', 'C:\\tools\\nu.exe', 'fish.exe']) {
    for (const bad of ['C:\\a&calc\\probe.exe', 'C:\\a|b c\\probe.exe', 'C:\\a;b c\\probe.exe', 'C:\\a$(x) b\\probe.exe', 'C:\\a%x% b\\probe.exe', "C:\\a'b c\\probe.exe"]) {
      assert.throws(() => commandWord(bad, 'win32', shell), /may act on/, `${shell} ${bad}`);
    }
    assert.equal(commandWord('C:\\Program Files\\probe.exe', 'win32', shell), 'C:\\Program Files\\probe.exe');
  }
  for (const shell of ['/usr/local/bin/fish', '/bin/csh', '']) {
    for (const bad of ['/a;b/probe', '/a b/$(id)/probe', '/a\\b c/probe', "/a'b c/probe", '/a!b c/probe', '/a`id` b/probe']) {
      assert.throws(() => commandWord(bad, 'darwin', shell), /may act on/, `${shell} ${bad}`);
    }
    assert.equal(commandWord('/Users/a b/probe', 'linux', shell), "'/Users/a b/probe'");
  }
});

// The verified POSIX shells, run for real where installed: each hostile path comes back as one word, unchanged, and nothing runs.
test('bash, sh, zsh, dash and ksh read each quoted path as one word', () => {
  if (process.platform === 'win32') { return; }
  const hostile = ["/x'; touch PWNED; '/probe", '/a b/$(touch PWNED)/probe', '/a`touch PWNED`/probe', '/a\\b/probe', '/a"b;c|d&e/probe', '/\u202Eevil\u200B/probe', "/it's here/probe"];
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'quote-'));
  for (const shell of ['bash', 'sh', 'zsh', 'dash', 'ksh']) {
    const bin = ['/bin/', '/usr/bin/'].map(d => d + shell).find(p => fs.existsSync(p));
    if (!bin) { continue; }
    for (const p of hostile) {
      const word = commandWord(p, 'linux', bin);
      const out = execFileSync(bin, ['-c', `printf '%s\\0' ${word}`], { cwd: dir }).toString();
      assert.equal(out, p + '\0', `${shell}: ${JSON.stringify(p)}`);
    }
  }
  assert.equal(fs.existsSync(path.join(dir, 'PWNED')), false, 'nothing in a path may run');
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
  assert.match(text, /isFile: isRegularFile,/);
  assert.match(text, /return notFoundPath\(r, setting\);/);
  assert.doesNotMatch(text, /path\.join\(ws, 'bin'/);
  const term = fs.readFileSync(path.join(__dirname, '..', '..', 'src', 'terminal', 'probeTerminal.ts'), 'utf8');
  assert.match(term, /const shown = commandWord\(command, process\.platform, vscode\.env\.shell\);/);
  assert.match(term, /\[shown, \.\.\.args\.map\(shellEscape\)\]/);
  const probe = fs.readFileSync(path.join(__dirname, '..', '..', 'src', 'probe.ts'), 'utf8');
  assert.match(probe, /try \{\n    probePath = getProbePath\(\);\n  \} catch \{\n    return \[\]; \/\/ probe was not found/);
  assert.match(probe, /try \{\n    probePath = getProbePath\(\);\n  \} catch \{\n    return \[\]; \/\/ lint on save stays quiet/);
});

// A line-editing shell (readline, zle) acts on a control character while the text is typed: ^U deletes back to the start of the line,
// so a quoted path could lose its opening quote and run what follows. Refused for every shell and system, in the command and in arguments.
test('a control character is refused in the command and in every argument, for every shell', () => {
  const shells: Array<[string, string]> = [['darwin', '/bin/zsh'], ['linux', '/bin/bash'], ['linux', '/usr/bin/fish'], ['linux', ''],
    ['win32', 'powershell.exe'], ['win32', 'cmd.exe'], ['win32', 'C:\\Windows\\System32\\wsl.exe'], ['win32', '']];
  for (const c of [0x00, 0x01, 0x03, 0x09, 0x0a, 0x0d, 0x15, 0x1b, 0x1f, 0x7f]) {
    const ch = String.fromCharCode(c);
    for (const [platform, shell] of shells) {
      const p = platform === 'win32' ? `C:\\a${ch}b\\probe.exe` : `/a${ch}touch PWNED;#/probe`;
      assert.throws(() => commandWord(p, platform, shell), /control character/, `${platform} ${shell} U+${c.toString(16)}`);
    }
    assert.throws(() => quoteArg(`tests/a${ch}b.probe`), /control character/, `argument U+${c.toString(16)}`);
  }
  assert.equal(quoteArg('tests/a b.probe'), "'tests/a b.probe'");
  assert.equal(quoteArg('tests/plain.probe'), 'tests/plain.probe');
});

// The backslash separates folders only on Windows. On macOS and Linux a shell drops it from an unquoted word, so it is never typed
// unquoted there: quoted for a known POSIX shell, refused for an unknown one.
test('a backslash in a path is quoted or refused on macOS and Linux, typed as it is only on Windows', () => {
  assert.equal(commandWord('/x/back\\slash/probe', 'linux', '/bin/bash'), "'/x/back\\slash/probe'");
  assert.equal(commandWord('/x/back\\slash/probe', 'darwin', '/bin/zsh'), "'/x/back\\slash/probe'");
  for (const shell of ['/bin/fish', '/bin/csh', '']) {
    assert.throws(() => commandWord('/a\\b/p', 'linux', shell), /may act on/, shell);
  }
  assert.equal(commandWord('C:\\tools\\probe.exe', 'win32', ''), 'C:\\tools\\probe.exe');
});

// WSL on Windows runs a POSIX shell: the path is single-quoted.
test('wsl on Windows quotes the POSIX way', () => {
  assert.equal(commandWord('C:\\Program Files\\probe.exe', 'win32', 'C:\\Windows\\System32\\wsl.exe'), "'C:\\Program Files\\probe.exe'");
  assert.equal(commandWord("/mnt/c/o'p/probe", 'win32', 'wsl.exe'), "'/mnt/c/o'\\''p/probe'");
});

// PowerShell on macOS or Linux runs a quoted path as a string, not a command: only a path of safe characters is typed, others are refused.
test('PowerShell on macOS and Linux: only a plain path is typed', () => {
  for (const shell of ['/usr/local/bin/pwsh', '/opt/microsoft/powershell/7/pwsh']) {
    assert.equal(commandWord('/usr/local/bin/probe', 'darwin', shell), '/usr/local/bin/probe');
    for (const bad of ['/Users/a b/probe', '/Users/O\u2019Brien/probe', '/Users/\u2018x/probe', "/a'b/probe"]) {
      assert.throws(() => commandWord(bad, 'linux', shell), /cannot be typed into PowerShell/, `${shell} ${bad}`);
    }
  }
});
