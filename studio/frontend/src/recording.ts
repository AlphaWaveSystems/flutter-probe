// recordingHeader is the first lines of the buffer Record puts in the editor: a comment with the time and the test header.
export function recordingHeader(now: string): string {
  return `# Recorded on ${now}\n\ntest "recorded flow"\n  open the app`;
}

// recordingBuffer is the editor text after Stop: the test header followed by the recorded steps, so the file is a complete test.
export function recordingBuffer(now: string, steps: string): string {
  return `${recordingHeader(now)}\n${steps.replace(/\s+$/, "")}\n`;
}
