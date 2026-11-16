// What the toolbar says while a run goes on: how long the running step has been going, and, after a while, that Studio is still waiting for it and
// that Cancel stops the run. Pure: no DOM and no clock of its own (the caller passes the times), so the node tests run it directly.

/** A step is "slow" after this long: the line that says Studio is still waiting appears. */
export const STILL_WAITING_AFTER_MS = 10_000;

/** The elapsed time shown beside the step count, only once it is worth saying. */
export const SHOW_ELAPSED_AFTER_MS = 3_000;

/** "4 s", "1 min 5 s", "2 min". */
export function formatElapsed(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000));
  if (s < 60) return `${s} s`;
  const m = Math.floor(s / 60);
  const rest = s % 60;
  return rest === 0 ? `${m} min` : `${m} min ${rest} s`;
}

/** "step 2 of 5", or "step 2 of 5 · 7 s" once the running step has been going for a few seconds. */
export function progressText(done: number, total: number, runningMs: number | null): string {
  const base = total > 0 ? `step ${Math.min(done + 1, total)} of ${total}` : `step ${done + 1}`;
  return runningMs !== null && runningMs >= SHOW_ELAPSED_AFTER_MS ? `${base} · ${formatElapsed(runningMs)}` : base;
}

/**
 * The line under the toolbar for a step that is taking long, or "" before that. It holds no elapsed time (that is in the progress text), so it
 * does not change every second and a screen reader hears it once. step is the step's own text, shown as text.
 */
export function waitingLine(step: string, runningMs: number | null, noSignal: boolean): string {
  if (runningMs === null || runningMs < STILL_WAITING_AFTER_MS) return "";
  const what = step ? `Still waiting for ${step}` : "Still waiting for the current step";
  return noSignal ? `${what}: the device has stopped sending its screen. Cancel stops the run.` : `${what}: Cancel stops the run.`;
}

/** The note next to Cancel while the device sends no screen, at any time of the run. */
export function signalNote(noSignal: boolean): string {
  return noSignal ? "no signal" : "";
}
