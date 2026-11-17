// The text of a recording. Pure: no DOM, no imports, so the node tests run it directly.

/** The head of a new recording: when it was made, the test block and its first step. The recorded steps go under it. */
export function recordingHeader(now: string): string {
  return `# Recorded on ${now}\n\ntest "recorded flow"\n  open the app`;
}

/**
 * The script when a recording stops: the header the recording started with, then the steps the engine recorded (each already indented under the test).
 * Stopping used to put the recorded steps in the editor alone, which dropped the `test "recorded flow"` line and left steps outside any test.
 */
export function recordedScript(header: string, recorded: string): string {
  const steps = recorded.replace(/^\s*\n/, "").replace(/\s+$/, "");
  return steps === "" ? `${header}\n` : `${header}\n${steps}\n`;
}
