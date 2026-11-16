// The Problems list and the lint markers of the open file. Nothing here touches the DOM or Monaco, so the node tests run it directly.
// The text of a finding comes from the person's own file and recipes: it is shown as text only.

export interface Problem {
  line: number; // 1-based, 0 where the rule has no line
  warning: boolean;
  message: string;
  source: string; // "reference" | "program"
}

export interface Marker {
  severity: number; // Monaco: 8 error, 4 warning
  message: string;
  startLineNumber: number;
  startColumn: number;
  endLineNumber: number;
  endColumn: number;
}

export const SEVERITY_ERROR = 8;
export const SEVERITY_WARNING = 4;

/** One marker per finding that has a line: the whole line, clamped into the text. Findings with no line are not markers. */
export function toMarkers(problems: Problem[], lineCount: number, maxColumn: (line: number) => number): Marker[] {
  const out: Marker[] = [];
  for (const p of problems) {
    if (!Number.isInteger(p.line) || p.line < 1 || lineCount < 1) continue;
    const line = Math.min(p.line, lineCount);
    out.push({
      severity: p.warning ? SEVERITY_WARNING : SEVERITY_ERROR,
      message: p.message,
      startLineNumber: line,
      startColumn: 1,
      endLineNumber: line,
      endColumn: Math.max(2, maxColumn(line)),
    });
  }
  return out;
}

/** The label of a row: "line 3: message", or the message alone for a finding with no line. */
export function rowLabel(p: Problem): string {
  return p.line >= 1 ? `line ${p.line}: ${p.message}` : p.message;
}

/** Errors first, then warnings, each group in line order (findings with no line last in their group). */
export function sortProblems(problems: Problem[]): Problem[] {
  const key = (p: Problem) => [p.warning ? 1 : 0, p.line >= 1 ? p.line : Number.MAX_SAFE_INTEGER] as const;
  return [...problems].sort((a, b) => {
    const [aw, al] = key(a);
    const [bw, bl] = key(b);
    return aw - bw || al - bl;
  });
}

/** The count shown on the tab: "Problems", or "Problems (3)". */
export function tabLabel(count: number): string {
  return count > 0 ? `Problems (${count})` : "Problems";
}

/**
 * Keeps a late answer for old text from painting over new text. begin(text) starts a request and cancels every earlier one; accept(token,
 * text) says whether the answer to that request may still be shown: it is the latest request, and the editor still holds the text it was for.
 */
export class LintSession {
  private latest = 0;
  begin(): number {
    return ++this.latest;
  }
  cancel(): void {
    this.latest++;
  }
  accept(token: number, askedFor: string, current: string): boolean {
    return token === this.latest && askedFor === current;
  }
}
