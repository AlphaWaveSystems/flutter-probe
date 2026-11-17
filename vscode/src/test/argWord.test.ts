import { test } from 'node:test';
import * as assert from 'node:assert/strict';
import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import { execFileSync } from 'node:child_process';
import { argWord } from '../toolPath';

// The arguments typed after the command, quoted for the terminal's shell (argWord). Hostile text is built from code points (no raw format
// characters in the source).
const HOSTILE = [
  "'; touch PWNED; '", '$(touch PWNED)', '`touch PWNED`', 'a\\b', '"x;y|z&w"', '\u202Eevil\u200B', "it's here", '--name=a b',
  '*', '~', '#not a comment', '!!', '{a,b}', 'a  b',
];
const CONTROL = ['a\nb', 'a\rb', 'a\u0000b', 'a\u001bb', 'a\u007fb', 'a\u0085b', 'a\u009bb'];
const SHELLS = ['/bin/bash', '/bin/zsh', 'C:\\Windows\\System32\\cmd.exe', 'pwsh', 'powershell.exe', '/usr/bin/fish', ''];

test('a plain argument is typed as it is, on every shell', () => {
  for (const shell of SHELLS) {
    for (const platform of ['linux', 'darwin', 'win32']) {
      for (const a of ['run', '--device', 'emulator-5554', 'tests/login.probe', '--tag=smoke', 'http://localhost:8080/x']) {
        assert.equal(argWord(a, platform, shell), a, `${platform} ${shell}: ${a}`);
      }
    }
  }
  assert.equal(argWord('C:\\tests\\a.probe', 'win32', 'cmd.exe'), 'C:\\tests\\a.probe');
});

test('a control character is refused on every shell and system (C0, DEL and C1)', () => {
  for (const shell of SHELLS) {
    for (const platform of ['linux', 'darwin', 'win32']) {
      for (const a of CONTROL) {
        assert.throws(() => argWord(a, platform, shell), /control character/, `${platform} ${shell}: ${JSON.stringify(a)}`);
      }
    }
  }
});

test('POSIX shells, and WSL or Git Bash on Windows: single quotes', () => {
  assert.equal(argWord("it's here", 'linux', '/bin/bash'), "'it'\\''s here'");
  assert.equal(argWord('a b', 'win32', 'C:\\Windows\\System32\\wsl.exe'), "'a b'");
  assert.equal(argWord('a b', 'win32', 'C:\\Program Files\\Git\\bin\\bash.exe'), "'a b'");
});

test('PowerShell: single quotes with the single quotes doubled; quote-like characters, " and a trailing \\ refused', () => {
  for (const [platform, shell] of [['win32', 'powershell.exe'], ['win32', 'pwsh.exe'], ['linux', '/usr/bin/pwsh'], ['darwin', 'pwsh']]) {
    assert.equal(argWord('a b', platform, shell), "'a b'");
    assert.equal(argWord("it's", platform, shell), "'it''s'");
    assert.equal(argWord('\u2018x\u2019', platform, shell), "'\u2018\u2018x\u2019\u2019'");
    assert.equal(argWord('$(calc); `x`', platform, shell), "'$(calc); `x`'");
    for (const bad of ['a\u201Ab', 'a\u201Bb', 'say "hi"', 'C:\\a b\\']) {
      assert.throws(() => argWord(bad, platform, shell), /argument/, `${platform} ${shell}: ${JSON.stringify(bad)}`);
    }
  }
});

test('cmd.exe: double quotes with a trailing run of backslashes doubled; % and " refused', () => {
  const cmd = 'C:\\Windows\\System32\\cmd.exe';
  assert.equal(argWord('a b', 'win32', cmd), '"a b"');
  assert.equal(argWord('a&b|c', 'win32', cmd), '"a&b|c"');
  assert.equal(argWord('C:\\my tests\\', 'win32', cmd), '"C:\\my tests\\\\"');
  assert.equal(argWord('C:\\my tests\\\\', 'win32', cmd), '"C:\\my tests\\\\\\\\"');
  for (const bad of ['%PATH%', '50%', 'say "hi"']) {
    assert.throws(() => argWord(bad, 'win32', cmd), /% or "/, bad);
  }
});

test('another shell: a metacharacter is refused; on macOS and Linux a backslash too; on Windows anything that needs quoting', () => {
  assert.equal(argWord('a b', 'linux', '/usr/bin/fish'), "'a b'");
  for (const bad of ['$(x)', 'a;b', "it's", 'a\\b']) {
    assert.throws(() => argWord(bad, 'linux', '/usr/bin/fish'), /may act on/, bad);
  }
  assert.throws(() => argWord('a b', 'win32', 'nu.exe'), /cannot be quoted/);
  assert.throws(() => argWord('a;b', 'win32', ''), /not detected/);
});

// The verified POSIX shells, run for real where installed: every hostile argument comes back as one word, unchanged, and nothing runs.
test('real POSIX shells keep each hostile argument one word and run nothing', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'probe-argword-'));
  let ran = 0;
  for (const shell of ['bash', 'sh', 'zsh', 'dash', 'ksh']) {
    const bin = ['/bin', '/usr/bin'].map(d => path.join(d, shell)).find(p => fs.existsSync(p));
    if (!bin) { continue; }
    const words = HOSTILE.map(a => argWord(a, 'linux', bin));
    const out = execFileSync(bin, ['-c', `printf '%s\\0' ${words.join(' ')}`], { cwd: dir }).toString();
    assert.deepEqual(out.split('\0').slice(0, -1), HOSTILE, shell);
    ran++;
  }
  assert.equal(fs.existsSync(path.join(dir, 'PWNED')), false, 'nothing in an argument may run');
  if (process.platform !== 'win32') { assert.ok(ran > 0, 'no POSIX shell found'); }
});
