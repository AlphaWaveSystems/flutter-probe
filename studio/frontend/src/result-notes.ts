// The notes under a failed test that come from what the engine reports about it: why the app died, that the test timed out, and how to find out
// why the app died when no evidence was kept. Pure: no DOM, so the node tests run it. Every text is shown with textContent.

export interface NoteInput {
  passed: boolean;
  skipped: boolean;
  crash?: string;
  timedOut?: boolean;
  timeLimitSeconds?: number;
  crashHint?: boolean;
  attempts?: number; // how many times the test ran, set when a retry ran it more than once
  earlierFailures?: { attempt: number; error: string }[];
}

export interface ResultNote {
  kind: "crash" | "timeout" | "hint" | "retry" | "attempt";
  text: string;
}

/** "90 s", "1 min 30 s", "2 min": the limit as a person reads it. */
export function formatLimit(seconds: number): string {
  const s = Math.round(seconds);
  if (!(s > 0)) return "";
  if (s < 60) return `${s} s`;
  const m = Math.floor(s / 60);
  const rest = s % 60;
  return rest === 0 ? `${m} min` : `${m} min ${rest} s`;
}

/** What a retried test says about its attempts: a flaky pass, or a test that failed every time. */
function retryNotes(r: NoteInput): ResultNote[] {
  const n = r.attempts ?? 0;
  if (!(n > 1)) return [];
  const out: ResultNote[] = [];
  out.push({
    kind: "retry",
    text: r.passed
      ? `Passed on attempt ${n}, after ${n - 1} failed ${n - 1 === 1 ? "attempt" : "attempts"} (defaults.retry_failed_tests re-ran it): the test is flaky.`
      : `Failed on all ${n} attempts (defaults.retry_failed_tests re-ran it).`,
  });
  for (const f of r.earlierFailures ?? []) out.push({ kind: "attempt", text: `Attempt ${f.attempt} failed: ${f.error}` });
  return out;
}

/** The notes for a finished test, in the order they are shown. A skipped test has none; a passed one has only what its retries say. */
export function resultNotes(r: NoteInput): ResultNote[] {
  if (r.skipped) return [];
  if (r.passed) return retryNotes(r);
  const out: ResultNote[] = [];
  if (r.timedOut) {
    const limit = formatLimit(r.timeLimitSeconds ?? 0);
    out.push({ kind: "timeout", text: limit ? `Timed out: the test ran past its ${limit} limit (defaults.test_timeout in probe.yaml).` : "Timed out: the test ran past its time limit (defaults.test_timeout in probe.yaml)." });
  }
  if (r.crash) out.push({ kind: "crash", text: r.crash });
  else if (r.crashHint) out.push({ kind: "hint", text: "The connection to the app was lost. To find out why the app died, run with defaults.crash_evidence: true in probe.yaml (Android)." });
  out.push(...retryNotes(r));
  return out;
}
