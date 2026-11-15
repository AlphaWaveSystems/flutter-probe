// Where Save as step puts its lines: inside a test (or a recipe or a hook), after the line the cursor is on, at the indent of the steps
// there. A cursor that is not inside one (a blank or comment line before the first test, a `use` line, a file with no test) gets no
// insertion and a sentence that says why, so the step never lands where ProbeScript would read it as something else.

const HEADER = /^(?:test|recipe|before|after|composite)\b/i;

export interface Place {
  after: number; // the 1-based line the new lines go after
  indent: string;
}

/** The place for a step, or a sentence that says why there is none. cursor is a 1-based line number. */
export function placeStep(lines: string[], cursor: number): Place | { reason: string } {
  const at = Math.min(Math.max(cursor, 1), Math.max(lines.length, 1));
  const code = (i: number) => lines[i] !== undefined && lines[i].trim() !== "" && !lines[i].trimStart().startsWith("#");
  // The nearest line at or above the cursor that holds code.
  let anchor = -1;
  for (let i = at - 1; i >= 0; i--) {
    if (code(i)) {
      anchor = i;
      break;
    }
  }
  if (anchor < 0) return { reason: "Put the cursor inside a test first: there is no test above it." };
  // The header of the block that line is in: the nearest line at column 0 that holds code.
  let header = anchor;
  while (header >= 0 && !(code(header) && !/^\s/.test(lines[header]))) header--;
  if (header < 0 || !HEADER.test(lines[header])) return { reason: "Put the cursor inside a test first: the line above it is not in a test." };
  const lead = /^\s*/.exec(lines[anchor])![0];
  return { after: at, indent: lead.length > 0 ? lead : "  " };
}
