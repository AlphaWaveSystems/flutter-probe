// What the hub said the device can do, in words for the pane header. Studio shows it so a read-only view (an older
// agent without the input calls) is not mistaken for a broken one.
import type { Caps } from "./protocol.ts";

export interface CapsNote {
  /** Short text appended to the header label; empty when everything is available. */
  label: string;
  /** Longer explanation for the tooltip; empty when everything is available. */
  title: string;
}

export function capsNote(caps: Caps): CapsNote {
  const short: string[] = [];
  const long: string[] = [];
  if (!caps.input) {
    short.push("view only");
    long.push("Taps, swipes and typing are off: the agent on the device does not support the input calls (update the app's flutter_probe_agent), or the view was started view-only.");
  }
  if (!caps.hit) {
    short.push("no element select");
    long.push("Hover outline and Alt+click selection are off: the agent cannot say which widget is at a point.");
  }
  return { label: short.join(", "), title: long.join(" ") };
}
