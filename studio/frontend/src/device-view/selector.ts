// Selector chip rule: `#key`, else `"text"`, else label, else type name.
// String escaping follows the ProbeScript lexer: backslash, double quote,
// newline and tab are escaped with a backslash.

import type { DeviceNode } from "./protocol.ts";

/** Keys the ProbeScript lexer accepts after `#`: letters, digits, underscore. */
const KEY_RE = /^[\p{L}\p{N}_]+$/u;

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

export function selectorFor(node: Pick<DeviceNode, "key" | "text" | "label" | "type">): string {
  if (usable(node.key) && KEY_RE.test(node.key)) return `#${node.key}`;
  if (usable(node.text)) return quoteString(node.text.trim());
  if (usable(node.label)) return quoteString(node.label.trim());
  return node.type;
}
