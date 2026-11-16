import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { formatLimit, resultNotes } from "../src/result-notes.ts";

const failed = { passed: false, skipped: false };

describe("formatLimit", () => {
  it("reads as a person says it", () => {
    assert.equal(formatLimit(90), "1 min 30 s");
    assert.equal(formatLimit(45), "45 s");
    assert.equal(formatLimit(120), "2 min");
    assert.equal(formatLimit(59.6), "1 min");
  });
  it("is empty for no limit", () => {
    assert.equal(formatLimit(0), "");
    assert.equal(formatLimit(-5), "");
    assert.equal(formatLimit(NaN), "");
  });
});

describe("resultNotes", () => {
  it("has no note for a test that passed or was skipped, whatever the fields say", () => {
    for (const base of [{ passed: true, skipped: false }, { passed: false, skipped: true }]) {
      assert.deepEqual(resultNotes({ ...base, crash: "x", timedOut: true, timeLimitSeconds: 9, crashHint: true }), []);
    }
  });
  it("says a test timed out, and the limit", () => {
    const n = resultNotes({ ...failed, timedOut: true, timeLimitSeconds: 90 });
    assert.equal(n.length, 1);
    assert.equal(n[0].kind, "timeout");
    assert.match(n[0].text, /Timed out: the test ran past its 1 min 30 s limit \(defaults\.test_timeout/);
    assert.match(resultNotes({ ...failed, timedOut: true })[0].text, /past its time limit/);
  });
  it("shows the crash line as it is, in place of the hint", () => {
    const n = resultNotes({ ...failed, crash: "the app died during this test: oom", crashHint: true });
    assert.deepEqual(n, [{ kind: "crash", text: "the app died during this test: oom" }]);
  });
  it("shows the hint only when no evidence came with the result", () => {
    const n = resultNotes({ ...failed, crashHint: true });
    assert.equal(n.length, 1);
    assert.equal(n[0].kind, "hint");
    assert.match(n[0].text, /defaults\.crash_evidence: true/);
    assert.match(n[0].text, /Android/);
    assert.deepEqual(resultNotes({ ...failed }), []);
  });
  it("orders the timeout before the crash line", () => {
    assert.deepEqual(resultNotes({ ...failed, timedOut: true, timeLimitSeconds: 30, crash: "c" }).map((x) => x.kind), ["timeout", "crash"]);
  });
});
