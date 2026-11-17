// The name a new file may be given when an unsaved buffer (a recording, a scratch buffer) is saved. Pure: no DOM, so the node tests run it directly.

export type SaveAsName = { ok: true; name: string } | { ok: false; reason: string };

const MAX_NAME = 100; // code points

// Path separators and the characters some file systems refuse, controls, line and paragraph separators, direction controls, zero-width characters and
// the byte order mark: none may be in a file name (they are not shown, or they change what is shown).
const FORBIDDEN = /[\\/:*?"<>|\u0000-\u001f\u007f-\u009f\u00ad\u061c\u180e\u200b-\u200f\u2028-\u202e\u2060-\u206f\ufeff]/;

/**
 * Checks a typed name against the names that already exist in the folder. A name without an extension gets `.probe`; any other extension is refused.
 * Names are compared without case, so a name that would replace a file on a case-insensitive file system is refused as well.
 */
export function saveAsName(raw: string, existing: readonly string[]): SaveAsName {
  const typed = raw.trim();
  if (typed === "") return { ok: false, reason: "Type a file name." };
  if (FORBIDDEN.test(typed)) return { ok: false, reason: "A file name cannot have a slash, a colon, a wildcard, a quote or a hidden character." };
  if (typed.startsWith(".")) return { ok: false, reason: "A file name cannot start with a dot." };
  const hasExt = /\.[^.]+$/.test(typed);
  if (hasExt && !/\.probe$/i.test(typed)) return { ok: false, reason: "A test file ends in .probe." };
  const name = hasExt ? typed : `${typed}.probe`;
  if (Array.from(name).length > MAX_NAME) return { ok: false, reason: `A file name is at most ${MAX_NAME} characters.` };
  if (/^\.probe$/i.test(name)) return { ok: false, reason: "Type a name before .probe." };
  const lower = name.toLowerCase();
  if (existing.some((e) => e.toLowerCase() === lower)) return { ok: false, reason: `${name} already exists here: choose another name (Save never replaces a file).` };
  return { ok: true, name };
}
