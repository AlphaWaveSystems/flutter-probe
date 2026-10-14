/**
 * Pure logic for turning a selected element into a ProbeScript `tap` step.
 * The webview sends the element's fields; the selector is built here, once,
 * with escaping, so device-screen text cannot add statements to the line.
 */
import { buildSelector, SelectorFields } from './selector';

export type StepResult = { ok: true; step: string; selector: string } | { ok: false; error: string };

export function buildTapStep(fields: SelectorFields): StepResult {
  const r = buildSelector(fields);
  return r.ok ? { ok: true, step: `tap ${r.selector}`, selector: r.selector } : r;
}

export interface InsertPlan {
  /** Text to insert at the end of the cursor's line. */
  text: string;
}

/**
 * Decide what to insert for the cursor line. A blank line receives the step
 * in place (keeping its indentation); otherwise the step goes on a new line
 * below, with the current line's indentation.
 */
export function planInsert(lineText: string, fields: SelectorFields): InsertPlan | undefined {
  const r = buildTapStep(fields);
  if (!r.ok) { return undefined; }
  const indent = /^[ \t]*/.exec(lineText)?.[0] ?? '';
  if (lineText.trim() === '') {
    return { text: `${lineText.length ? '' : indent}${r.step}` };
  }
  return { text: `\n${indent}${r.step}` };
}
