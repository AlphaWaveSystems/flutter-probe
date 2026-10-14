// Mirrors studio/frontend/src/device-view/selector.ts (rule: `#key`, else
// `"text"`, else label, else type name). Keep the two in step: a test in
// studio/frontend/device-view-tests/selector.test.ts compares both against
// testdata/selector-injection.json, and vscode/src/test/deviceView.test.ts
// checks that quoteString is textually identical in both files.
//
// Unlike the Studio chip, this function feeds a line that is inserted into a
// .probe file, so it also refuses values that could not be written as exactly
// one statement. Widget text comes from the device screen and is untrusted.

export interface SelectorFields {
  key?: string | null;
  text?: string | null;
  label?: string | null;
  type?: string | null;
}

/** Keys the ProbeScript lexer accepts after `#`: letters, digits, underscore. */
const KEY_RE = /^[\p{L}_][\p{L}\p{Nd}_]*$/u;

/** Longest text or label accepted for an inserted step. */
export const MAX_SELECTOR_TEXT = 200;

export function quoteString(s: string): string {
  const body = s
    .replace(/\\/g, "\\\\")
    .replace(/"/g, '\\"')
    .replace(/\r\n|\r|\n/g, "\\n")
    .replace(/\t/g, "\\t");
  return `"${body}"`;
}

function usable(s: string | null | undefined): s is string {
  return typeof s === "string" && s.trim() !== "";
}

/** Pure selector rule; same output as the Studio chip for the same fields. */
export function selectorFor(node: SelectorFields): string {
  if (usable(node.key) && KEY_RE.test(node.key)) return `#${node.key}`;
  if (usable(node.text)) return quoteString(node.text.trim());
  if (usable(node.label)) return quoteString(node.label.trim());
  const type = node.type ?? "";
  return quoteString(type);
}

export type SelectorResult = { ok: true; selector: string } | { ok: false; error: string };

/** C0/C1 controls, DEL and the unicode line and paragraph separators. */
const UNSAFE_RE = /[\u0000-\u001f\u007f-\u009f\u2028\u2029]/;

/**
 * Build the selector for an inserted step, or refuse with a message. Only the
 * value the selector rule would use is checked.
 */
export function buildSelector(node: SelectorFields): SelectorResult {
  const chosen =
    usable(node.key) && KEY_RE.test(node.key) ? node.key
    : usable(node.text) ? node.text.trim()
    : usable(node.label) ? node.label.trim()
    : node.type ?? "";
  if (chosen === "") return { ok: false, error: "The selected element has no key, text, label or type." };
  if (UNSAFE_RE.test(chosen)) {
    return { ok: false, error: "The selected element text contains a control or line-break character, so no step was inserted." };
  }
  if (chosen.length > MAX_SELECTOR_TEXT) {
    return { ok: false, error: `The selected element text is longer than ${MAX_SELECTOR_TEXT} characters, so no step was inserted.` };
  }
  const selector = selectorFor(node);
  // Defence in depth: the result must be a single #key, one quoted string or a bare word.
  if (!/^(#[\p{L}\p{N}_]+|[A-Za-z0-9_]+|"(?:[^"\\]|\\.)*")$/u.test(selector)) {
    return { ok: false, error: "The selector could not be written safely." };
  }
  return { ok: true, selector };
}
