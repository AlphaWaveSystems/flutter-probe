// Which binary the extension runs. This module has no vscode import, so the tests can load it.
//
// The configured path comes from user or remote settings only: the path settings are machine-scoped (package.json), so a repository's
// .vscode/settings.json cannot point them at a binary of its own. A binary inside the workspace (bin/probe) is used only in a trusted
// workspace. The extension declares that it does not run in an untrusted one (capabilities.untrustedWorkspaces), and this check keeps the
// rule if that ever changes.

export interface ToolPathInput {
  configured: string | undefined; // the setting's value
  defaultName: string;            // the command on PATH: 'probe' or 'probe-convert'
  workspaceRoot: string | undefined;
  trusted: boolean;               // vscode.workspace.isTrusted
  exists: (path: string) => boolean;
  join: (...parts: string[]) => string;
}

export function resolveToolPath(i: ToolPathInput): string {
  if (i.configured && i.configured !== i.defaultName) {
    return i.configured;
  }
  if (i.workspaceRoot && i.trusted) {
    const localBin = i.join(i.workspaceRoot, 'bin', i.defaultName);
    if (i.exists(localBin)) {
      return localBin;
    }
  }
  return i.defaultName;
}
