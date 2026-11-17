// The name a new file may be given when an unsaved buffer (a recording, a scratch buffer) is saved. Pure: no DOM, so the node tests run it directly.

export type SaveAsName = { ok: true; name: string } | { ok: false; reason: string };

const MAX_NAME = 100; // code points

// Path separators and the characters some file systems refuse, controls, line and paragraph separators, direction controls, zero-width characters and
// the byte order mark: none may be in a file name (they are not shown, or they change what is shown).
const FORBIDDEN = /[\\/:*?"<>|\u0000-\u001f\u007f-\u009f\u00ad\u061c\u180e\u200b-\u200f\u2028-\u202e\u2060-\u206f\ufeff]/;

/** A name as a file system that ignores case and Unicode normalisation sees it (APFS, NTFS): composed form, lower case. */
function fold(s: string): string {
  return s.normalize("NFC").toLowerCase().normalize("NFC");
}

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
  // the extension is written in lower case, as the rest of Studio and the file system tools expect it (x.PROBE is saved as x.probe)
  const stem = hasExt ? typed.replace(/\.probe$/i, "") : typed;
  const name = `${stem}.probe`;
  if (Array.from(name).length > MAX_NAME) return { ok: false, reason: `A file name is at most ${MAX_NAME} characters.` };
  if (stem === "") return { ok: false, reason: "Type a name before .probe." };
  if (/[. ]$/.test(stem)) return { ok: false, reason: "A file name cannot end with a dot or a space before .probe." };
  const base = stem.split(".")[0].toUpperCase();
  if (/^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])$/.test(base)) return { ok: false, reason: `${base} is a name Windows reserves for a device: choose another name.` };
  const folded = fold(name);
  if (existing.some((e) => fold(e) === folded)) return { ok: false, reason: `${name} already exists here: choose another name (Save never replaces a file).` };
  return { ok: true, name };
}
