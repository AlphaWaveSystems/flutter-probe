/**
 * Pure logic for turning a selector chip into a ProbeScript `tap` step.
 */

export function normalizeSelector(selector: string): string | undefined {
  const s = selector.trim();
  if (!s || s.length > 200 || /[\u0000-\u001f]/.test(s)) { return undefined; }
  if (s.startsWith('#') || (s.length >= 2 && s.startsWith('"') && s.endsWith('"'))) { return s; }
  // Label or widget type chip: quote it so it parses as a text selector.
  return `"${s.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`;
}

export function buildTapStep(selector: string): string | undefined {
  const sel = normalizeSelector(selector);
  return sel ? `tap ${sel}` : undefined;
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
export function planInsert(lineText: string, selector: string): InsertPlan | undefined {
  const step = buildTapStep(selector);
  if (!step) { return undefined; }
  const indent = /^[ \t]*/.exec(lineText)?.[0] ?? '';
  if (lineText.trim() === '') {
    return { text: `${lineText.length ? '' : indent}${step}` };
  }
  return { text: `\n${indent}${step}` };
}
