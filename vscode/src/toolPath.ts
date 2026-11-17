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
