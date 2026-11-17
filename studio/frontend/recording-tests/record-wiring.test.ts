import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";

// main.ts cannot be loaded by node (it needs the editor and the page), so the wiring of the Record button is read from its text: the script of a
// stopped recording goes into the editor through recordedScript with the header the recording started with. Putting the engine's steps in alone
// (model.setValue(script)) drops the `test "recorded flow"` line and leaves steps outside any test; the pure functions pass either way.

const main = readFileSync(join(import.meta.dirname, "..", "src", "main.ts"), "utf8");
const code = main.split("\n").map((l) => l.replace(/\/\/.*$/, ""));

/** The lines of the Stop branch: from the StopRecording call to the end of its try block. */
function stopBranch(): string {
  const at = code.findIndex((l) => l.includes("await StopRecording()"));
  assert.ok(at >= 0, "main.ts no longer calls StopRecording where the test expects it");
  const end = code.findIndex((l, i) => i > at && /\bcatch\b/.test(l));
  return code.slice(at, end < 0 ? at + 12 : end).join("\n");
}

describe("the Record button in main.ts", () => {
  it("starts from the recording header, and keeps it for Stop", () => {
    assert.match(main, /recordingHead\s*=\s*recordingHeader\(/);
    assert.match(main, /model\.setValue\(recordingHead\)/);
  });
  it("puts a stopped recording under that header", () => {
    const stop = stopBranch();
    assert.match(stop, /model\.setValue\(recordedScript\(recordingHead,\s*script\)\)/);
    assert.doesNotMatch(stop, /model\.setValue\(\s*script\s*\)/, "the engine's steps alone replace the header");
  });
  it("the check sees the wrong wiring", () => {
    assert.doesNotMatch("model.setValue(recordedScript(recordingHead, script))", /model\.setValue\(\s*script\s*\)/);
    assert.match("model.setValue(script)", /model\.setValue\(\s*script\s*\)/);
  });
});
