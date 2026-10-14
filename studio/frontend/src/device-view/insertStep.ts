// Turning a selected element into a ProbeScript `tap` step at the editor cursor. The selector comes from
// selectorFor (escaped, never raw device text) and this module only places it, so text on the device screen cannot
// add a statement to the line. The VS Code extension has the same rule in vscode/src/deviceView/insertStep.ts.

export interface InsertPlan {
  /** Zero-based column in the cursor's line where the text goes (the end of the line). */
  column: number;
  text: string;
}

/**
 * A blank cursor line receives the step in place (keeping its indentation); otherwise the step goes on a new line
 * below, indented like the current line. `selector` must come from selectorFor.
 */
export function planTapInsert(lineText: string, selector: string): InsertPlan | undefined {
  // Defence in depth: a selector is one token run on one line.
  if (selector === "" || /[\r\n]/.test(selector)) return undefined;
  const step = `tap ${selector}`;
  const indent = /^[ \t]*/.exec(lineText)?.[0] ?? "";
  if (lineText.trim() === "") return { column: lineText.length, text: `${lineText.length ? "" : indent}${step}` };
  return { column: lineText.length, text: `\n${indent}${step}` };
}
