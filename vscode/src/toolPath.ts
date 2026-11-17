import * as fs from 'fs';

// Which binary the extension runs. This module has no vscode import, so the tests can load it.
//
// The answer is always an absolute path to a regular file, or "not found". A bare name is never handed to the system to look up: on
// Windows, Node's spawn (libuv's search_path) and cmd.exe look in the current directory before PATH, and the extension starts its
// children in the workspace, so a probe.exe in a repository would win over the one on PATH. This module searches only the absolute
// entries of PATH itself.
//
// - The configured path (probePath, convertPath) comes from user or remote settings only (machine scope, package.json). An absolute path
//   is used when it is a regular file; a bare name is looked up on PATH; a relative path with folders in it is refused, since it would
//   be relative to the workspace.
// - bin/<name> inside the workspace is used only when the person turned on flutterprobe.useWorkspaceBinaries (machine scope, off by
//   default) and the workspace is trusted. Trusting a folder is not consent to run its binaries.
// - Otherwise the first match on PATH.

// The settings that name a binary, and the opt-in for the workspace's own: all machine-scoped (package.json).
export const TOOL_SETTINGS = ['probePath', 'convertPath', 'adbPath', 'flutterPath', 'useWorkspaceBinaries'] as const;

export interface ToolPathInput {
  configured: string | undefined;  // the setting's value
  defaultName: string;             // 'probe' or 'probe-convert'
  workspaceRoot: string | undefined;
  useWorkspaceBinaries: boolean;   // flutterprobe.useWorkspaceBinaries
  trusted: boolean;                // vscode.workspace.isTrusted
  platform: string;                // process.platform
  pathEnv: string | undefined;     // the PATH the children get
  isFile: (p: string) => boolean;  // a regular file (a directory is not one)
  isAbsolute: (p: string) => boolean;
  join: (...parts: string[]) => string;
}

export type ToolPath =
  | { path: string }
  | { notFound: string; reason: 'not-on-path' | 'not-a-file' | 'relative' };

// candidates are the files a name stands for in one folder. On Windows, as libuv does for spawn: the name as given when it already ends in
// .com or .exe, otherwise the name with .com, then with .exe (CreateProcess runs only those; a .cmd or .bat needs a shell).
export function candidates(dir: string, name: string, platform: string, join: ToolPathInput['join']): string[] {
  if (platform !== 'win32') { return [join(dir, name)]; }
  if (/\.(com|exe)$/i.test(name)) { return [join(dir, name)]; }
  return [join(dir, `${name}.com`), join(dir, `${name}.exe`)];
}

// pathDirs are the absolute folders of a PATH value. Empty and relative entries ("", ".", "bin") are dropped: they would be resolved
// against the child's working directory, which is the workspace.
export function pathDirs(pathEnv: string | undefined, platform: string, isAbsolute: ToolPathInput['isAbsolute']): string[] {
  const sep = platform === 'win32' ? ';' : ':';
  return (pathEnv ?? '')
    .split(sep)
    .map(d => (platform === 'win32' ? d.replace(/^"(.*)"$/, '$1') : d))
    .filter(d => d !== '' && isAbsolute(d));
}

function onPath(name: string, i: ToolPathInput): string | undefined {
  for (const dir of pathDirs(i.pathEnv, i.platform, i.isAbsolute)) {
    for (const c of candidates(dir, name, i.platform, i.join)) {
      if (i.isFile(c)) { return c; }
    }
  }
  return undefined;
}

export function resolveToolPath(i: ToolPathInput): ToolPath {
  const configured = i.configured?.trim();
  if (configured && configured !== i.defaultName) {
    if (i.isAbsolute(configured)) {
      return i.isFile(configured) ? { path: configured } : { notFound: configured, reason: 'not-a-file' };
    }
    if (/[\\/]/.test(configured)) {
      return { notFound: configured, reason: 'relative' };
    }
    const found = onPath(configured, i);
    return found ? { path: found } : { notFound: configured, reason: 'not-on-path' };
  }
  if (i.useWorkspaceBinaries && i.trusted && i.workspaceRoot) {
    for (const c of candidates(i.join(i.workspaceRoot, 'bin'), i.defaultName, i.platform, i.join)) {
      if (i.isFile(c)) { return { path: c }; }
    }
  }
  const found = onPath(i.defaultName, i);
  return found ? { path: found } : { notFound: i.defaultName, reason: 'not-on-path' };
}

// notFoundPath is what the extension runs when resolveToolPath found nothing. A missing absolute path is returned as it is (running it
// fails with "not found"). A name that is on no absolute folder of PATH, and a relative path, are an error that says what to do, on every
// system: a bare name could only be found through an empty or relative PATH entry, which is the workspace.
export function notFoundPath(r: Extract<ToolPath, { notFound: string }>, setting: string): string {
  if (r.reason === 'not-a-file') { return r.notFound; }
  if (r.reason === 'relative') {
    throw new Error(`flutterprobe.${setting} is "${r.notFound}": use an absolute path or a command name on PATH`);
  }
  // On every system: the bare name could only be found through an empty or relative entry of PATH, which is the workspace.
  throw new Error(`${r.notFound} was not found on PATH: install it, or set flutterprobe.${setting} to its absolute path in your user settings`);
}

// isRegularFile says whether p is a regular file (a directory, a missing path or one that cannot be read is not). Symlinks are followed.
export function isRegularFile(p: string): boolean {
  try {
    return fs.statSync(p).isFile();
  } catch {
    return false;
  }
}

// quotePosix quotes one word for a POSIX shell: it is left as it is when it holds only safe characters, else single-quoted.
export function quotePosix(word: string): string {
  if (/^[a-zA-Z0-9_./:=-]+$/.test(word)) { return word; }
  return `'${word.replace(/'/g, "'\\''")}'`;
}

// CONTROL is any control character: C0 (U+0000 to U+001F), DEL (U+007F) and C1 (U+0080 to U+009F). A line-editing shell acts on them
// while the text is typed into the terminal (readline and zle: ^U deletes back to the start of the line, ^A moves to it, ^C cancels it;
// under a single-byte locale such as LC_ALL=C, readline reads a C1 character as a meta key: U+0092 is M-C-r, revert-line), so a quoted
// path that holds one can lose its opening quote and run what follows. Refused for every shell, in the command and in every argument.
export const CONTROL = /[\u0000-\u001f\u007f-\u009f]/;

// shown is text quoted for a message, with every control character and the line and paragraph separators written as \uXXXX, so the
// message itself carries none of them.
function shown(text: string): string {
  return JSON.stringify(text).replace(/[\u007f-\u009f\u2028\u2029]/g, c => '\\u' + c.charCodeAt(0).toString(16).padStart(4, '0'));
}

// refuseControl throws when text holds a control character.
export function refuseControl(text: string, what: string): void {
  if (CONTROL.test(text)) {
    throw new Error(`${what} ${shown(text)} has a control character, which the terminal's shell would act on as it is typed: rename it, or install the tool in a folder without one`);
  }
}

// quoteArg quotes one argument of the command the POSIX way (shellEscape in probeTerminal.ts). An argument with a control character is
// refused (CONTROL): a line-editing shell would act on it as it is typed.
export function quoteArg(arg: string): string {
  refuseControl(arg, 'the argument');
  if (/^[a-zA-Z0-9_./:=-]+$/.test(arg)) { return arg; }
  return `'${arg.replace(/'/g, "'\\''")}'`;
}

// The POSIX shells whose single quotes were checked to keep any text as one word with nothing expanded.
const POSIX_SHELLS = new Set(['bash', 'sh', 'zsh', 'dash', 'ksh']);

// Characters a shell can act on. Under a shell whose quoting is not handled here, a path that holds one is refused rather than typed.
// The backslash is one too on macOS and Linux; on Windows it separates folders and every path has it.
const METACHARACTERS = /[&|;<>^`$()'"%!\n\r]/;

// commandWord is the command as typed into the terminal, quoted for the shell the terminal runs (vscode.env.shell), or an error when the
// path cannot be typed safely for that shell. Only the command is handled here: the arguments are still quoted the POSIX way on every
// system (shellEscape in probeTerminal.ts).
// - bash, sh, zsh, dash, ksh (also Git Bash or WSL on Windows): POSIX single quotes.
// - PowerShell: the call operator and single quotes, & '...'. PowerShell also reads the typographic quotes ‘ ’ “ ” as quotation marks
//   (about_Quoting_Rules), so ' ‘ ’ are doubled inside the string, and a path with ‚ or ‛, which that page does not list, is refused.
// - cmd.exe: double quotes. %NAME% still expands inside them and cmd has no escape for it on a command line, so a path with % is refused.
// - PowerShell on macOS or Linux: a single-quoted path is a string there, not a command, so only a path of safe characters is typed;
//   any other is refused.
// - Any other shell (fish, nu, csh, none detected): a path of safe characters is typed as it is, one with a shell metacharacter (or, on
//   macOS and Linux, a backslash) is refused, and any other (a space) is single-quoted on macOS and Linux and typed as it is on Windows.
// On every system and shell a path with a control character is refused (CONTROL). The backslash counts as safe only on Windows, where it
// separates folders; on macOS and Linux a shell would drop it from an unquoted word.
export function commandWord(command: string, platform: string, shell: string): string {
  refuseControl(command, 'the path of the command,');
  const safe = platform === 'win32' ? /^[a-zA-Z0-9_.:\\/-]+$/ : /^[a-zA-Z0-9_.:/-]+$/;
  if (safe.test(command)) { return command; }
  const name = (shell.split(/[\\/]/).pop() ?? '').toLowerCase().replace(/\.exe$/, '');
  if (POSIX_SHELLS.has(name) || (name === 'wsl' && platform === 'win32')) { return quotePosix(command); }
  if (platform === 'win32' && (name === 'powershell' || name === 'pwsh')) {
    if (/[\u201A\u201B\n\r]/.test(command)) {
      throw new Error(`the path of the command, ${JSON.stringify(command)}, has a character PowerShell may read as a quote: install it in a folder without one, or set flutterprobe.probePath to such a path`);
    }
    return `& '${command.replace(/['\u2018\u2019]/g, c => c + c)}'`;
  }
  if (platform !== 'win32' && (name === 'powershell' || name === 'pwsh')) {
    throw new Error(`the path of the command, ${JSON.stringify(command)}, cannot be typed into PowerShell on this system, where a quoted path is not run: install it in a folder whose path has only letters, digits and . _ - / :, or use bash or zsh as the terminal`);
  }
  if (platform === 'win32' && name === 'cmd') {
    if (/[%"\n\r]/.test(command)) {
      throw new Error(`the path of the command, ${JSON.stringify(command)}, has a % or " that cmd.exe would read: install it in a folder without one, or use PowerShell as the terminal`);
    }
    return `"${command}"`;
  }
  if (METACHARACTERS.test(command) || (platform !== 'win32' && command.includes('\\'))) {
    throw new Error(`the path of the command, ${JSON.stringify(command)}, has a character the terminal's shell (${name || 'not detected'}) may act on: install it in a folder without one, or use bash, zsh, PowerShell or cmd.exe as the terminal`);
  }
  return platform === 'win32' ? command : quotePosix(command);
}
