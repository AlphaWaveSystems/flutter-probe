// What the status area says after a Connect that failed. The reason stays until the person acts (another device, another Connect): it used to
// go back to "disconnected" after 2.5 seconds, and the toast with the reason is gone after 8, so a person who looked away saw no reason at all.

const MAX = 90; // code points

// Controls, line and paragraph separators, bidirectional controls and isolates, zero-width characters, the byte order mark and the soft hyphen: none
// of them is shown, and a reason made of them would read as an empty one, or turn the text around.
// Joiners (U+200C and U+200D) are not in the list: between letters or emoji they are part of the text (a family emoji, a Persian word), and a joiner that
// touches a space, an edge or another hidden character is dropped below.
const HIDDEN = /[\u0000-\u001f\u007f-\u009f\u00ad\u061c\u180e\u200b\u200e\u200f\u2028-\u202e\u2060-\u206f\ufeff]/g;

/** One line of the reason: hidden and direction characters become a space, a joiner stays only between two visible characters, runs of spaces become one. */
function tidy(line: string): string {
  const spaced = line.replace(HIDDEN, " ").replace(/\s+/g, " ");
  return spaced.replace(/[\u200c\u200d]+/g, (m, offset: number, all: string) => {
    const before = all[offset - 1];
    const after = all[offset + m.length];
    return before !== undefined && after !== undefined && !/\s/.test(before) && !/\s/.test(after) ? m : "";
  }).replace(/\s+/g, " ").trim();
}

/** The one line for the status: "connection failed: <first line of the reason>", cut at MAX code points with an ellipsis. The full text is the tooltip. */
export function connectFailureLine(detail: string): string {
  const lines = detail.split(/\r\n|[\n\r\u2028\u2029]/);
  const first = lines.map(tidy).find((l) => l !== "") ?? "";
  if (first === "") return "connection failed: see the tooltip for the reason";
  const chars = Array.from(`connection failed: ${first}`);
  return chars.length <= MAX ? chars.join("") : `${chars.slice(0, MAX - 1).join("")}…`;
}
