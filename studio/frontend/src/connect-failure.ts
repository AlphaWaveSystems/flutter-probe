// What the status area says after a Connect that failed. The reason stays until the person acts (another device, another Connect): it used to
// go back to "disconnected" after 2.5 seconds, and the toast with the reason is gone after 8, so a person who looked away saw no reason at all.

const MAX = 90;

/** The one line for the status: "connection failed: <first line of the reason>", cut at MAX characters with an ellipsis. The full text is the tooltip. */
export function connectFailureLine(detail: string): string {
  const first = (detail.split("\n").find((l) => l.trim() !== "") ?? "").replace(/\s+/g, " ").trim();
  if (first === "") return "connection failed";
  const line = `connection failed: ${first}`;
  return line.length <= MAX ? line : `${line.slice(0, MAX - 1)}…`;
}
