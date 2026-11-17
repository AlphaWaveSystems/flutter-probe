// Whether the focused element takes typed text. A key that is also a printable character (the `?` that opens the help card) must be left to such an element: the
// global handler used to exempt only the editor, so a `?` typed in the chat input or in any field opened the help card and was swallowed ("why?" left "why").

/** The part of an element this needs, so it can be tested without a document. */
export interface TypingTarget {
  tagName?: string;
  type?: string;
  isContentEditable?: boolean;
}

// The input types that take text; a checkbox, a radio button, a button or a slider does not.
const TEXT_INPUT_TYPES = new Set(["", "text", "search", "url", "tel", "email", "password", "number", "date", "datetime-local", "month", "time", "week"]);

export function isTextEntry(el: TypingTarget | null | undefined): boolean {
  if (!el) return false;
  if (el.isContentEditable) return true;
  switch ((el.tagName ?? "").toUpperCase()) {
    case "TEXTAREA":
    case "SELECT": // typing in a select picks an option by its first letters
      return true;
    case "INPUT":
      return TEXT_INPUT_TYPES.has((el.type ?? "").toLowerCase());
    default:
      return false;
  }
}
