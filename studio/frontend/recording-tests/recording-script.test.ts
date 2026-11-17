import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { recordedScript, recordingHeader } from "../src/recording-script.ts";

describe("recordedScript", () => {
  const head = recordingHeader("11/16/2026, 9:00:00 PM");
  it("keeps the test block: the recorded steps go under the header, not in its place", () => {
    const out = recordedScript(head, '  tap "Sign In"\n  type "a" into the #email field\n');
    assert.equal(out, `${head}\n  tap "Sign In"\n  type "a" into the #email field\n`);
    assert.equal(out.split("\n").filter((l) => l.startsWith("test ")).length, 1);
    assert.match(out, /test "recorded flow"\n  open the app\n  tap "Sign In"/);
  });
  it("a recording with no steps is the header alone, with a closing newline", () => {
    for (const none of ["", "\n", "  \n \n"]) assert.equal(recordedScript(head, none), `${head}\n`);
  });
  it("blank lines around the steps are dropped, the steps' own indentation is kept", () => {
    assert.equal(recordedScript(head, '\n\n  swipe up\n  go back\n\n'), `${head}\n  swipe up\n  go back\n`);
  });
  it("the header names when it was made, a test and its first step", () => {
    assert.match(head, /^# Recorded on 11\/16\/2026, 9:00:00 PM\n\ntest "recorded flow"\n  open the app$/);
  });
});
