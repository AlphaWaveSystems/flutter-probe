// Opening the failing line: which file and line a click in the results asks for, and what the window may do about it. Nothing here touches
// the DOM, so the node tests run it directly. The window opens a file only through ReadFile (Studio refuses a path outside the workspace, a
// `..` segment, a link out of it and a file that is not there), and moves the cursor only on a click.

export interface JumpTarget {
  file?: string;
  line?: number;
}

export interface JumpContext {
  openFile: string | null;
  workspaceRoot: string;
}

export type JumpPlan =
  | { kind: "same"; line: number }
  | { kind: "open"; path: string; line: number }
  | { kind: "refuse"; reason: string };

/** Slashes made one kind, repeated and `.` segments dropped, no trailing slash: enough to tell whether two spellings name one file. */
export function normalizePath(p: string): string {
  const parts = p.replace(/\\/g, "/").split("/").filter((seg, i) => seg !== "." && (seg !== "" || i === 0));
  const joined = parts.join("/");
  return joined === "" && p.startsWith("/") ? "/" : joined;
}

function hasUnsafeSegment(p: string): boolean {
  return p.replace(/\\/g, "/").split("/").some((s) => s === "..") || /[\u0000-\u001f\u007f]/.test(p);
}

/**
 * What a click on a failing row should do. A file name with `..` in it, a path outside the workspace, a file that is not a `.probe` file,
 * and a target with no file when none is open are refused, with a sentence. The same file as the open one is not opened again (nothing is
 * reloaded, and a person's unsaved edits are not asked about). Another file is opened by the window, which asks about unsaved changes.
 */
export function planJump(t: JumpTarget, c: JumpContext): JumpPlan {
  const line = Number.isInteger(t.line) && (t.line as number) >= 1 ? (t.line as number) : 0;
  if (!t.file) {
    return c.openFile ? { kind: "same", line } : { kind: "refuse", reason: "There is no file open, and this result names none." };
  }
  if (hasUnsafeSegment(t.file)) return { kind: "refuse", reason: "A file name with .. in it, or with a control character, is not opened." };
  const isAbs = t.file.startsWith("/") || /^[A-Za-z]:[\\/]/.test(t.file);
  let path = t.file;
  if (!isAbs) {
    if (!c.workspaceRoot) return { kind: "refuse", reason: "No workspace is open, so a file named relative to it cannot be found." };
    path = `${c.workspaceRoot.replace(/[\\/]+$/, "")}/${t.file}`;
  }
  path = normalizePath(path);
  const root = normalizePath(c.workspaceRoot);
  if (!root || !(path === root || path.startsWith(`${root}/`))) {
    return { kind: "refuse", reason: "That file is outside the open workspace, so it was not opened." };
  }
  if (!path.endsWith(".probe")) return { kind: "refuse", reason: "Only .probe files are opened." };
  if (c.openFile && normalizePath(c.openFile) === path) return { kind: "same", line };
  return { kind: "open", path, line };
}

/** Where to put the cursor in a file of `lineCount` lines: the line, or the last one with a sentence when the file is shorter. */
export function clampLine(line: number, lineCount: number): { line: number; note: string } {
  if (line < 1) return { line: 0, note: "" };
  if (line > lineCount) return { line: lineCount, note: `The file has only ${lineCount} lines; showing the last one.` };
  return { line, note: "" };
}

/**
 * The words on a button that opens a failing place: `L12` and `Open line 12 of login.probe`, or just the file when no line is known. The
 * folder is never shown (only the file's name), and nothing is made of a place with no file.
 */
export function placeLabel(file: string | undefined, line: number | undefined): { text: string; title: string } | null {
  if (!file) return null;
  const name = file.replace(/\\/g, "/").split("/").filter(Boolean).pop() ?? "";
  if (!name) return null;
  const n = Number.isInteger(line) && (line as number) >= 1 ? (line as number) : 0;
  return n > 0 ? { text: `L${n}`, title: `Open line ${n} of ${name}` } : { text: name, title: `Open ${name}` };
}
