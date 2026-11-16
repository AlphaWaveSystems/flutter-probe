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
  it("has no note for a test that passed or was skipped, whatever the failure fields say", () => {
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

describe("resultNotes for retries", () => {
  const earlier = [{ attempt: 1, error: "not found" }, { attempt: 2, error: "timed out" }];
  it("says a test passed on a later attempt, that it is flaky, and lists the earlier failures", () => {
    const n = resultNotes({ passed: true, skipped: false, attempts: 3, earlierFailures: earlier });
    assert.deepEqual(n.map((x) => x.kind), ["retry", "attempt", "attempt"]);
    assert.match(n[0].text, /^Passed on attempt 3, after 2 failed attempts/);
    assert.match(n[0].text, /flaky/);
    assert.equal(n[1].text, "Attempt 1 failed: not found");
    assert.equal(n[2].text, "Attempt 2 failed: timed out");
  });
  it("uses the singular for one failed attempt", () => {
    assert.match(resultNotes({ passed: true, skipped: false, attempts: 2, earlierFailures: [earlier[0]] })[0].text, /after 1 failed attempt \(/);
  });
  it("says a test failed on all attempts, after the other notes", () => {
    const n = resultNotes({ passed: false, skipped: false, attempts: 2, earlierFailures: [earlier[0]], timedOut: true, timeLimitSeconds: 30 });
    assert.deepEqual(n.map((x) => x.kind), ["timeout", "retry", "attempt"]);
    assert.equal(n[1].text, "Failed on all 2 attempts (defaults.retry_failed_tests re-ran it).");
  });
  it("has nothing for a test that ran once, and none for a skipped one", () => {
    assert.deepEqual(resultNotes({ passed: true, skipped: false, attempts: 1 }), []);
    assert.deepEqual(resultNotes({ passed: true, skipped: false }), []);
    assert.deepEqual(resultNotes({ passed: false, skipped: true, attempts: 3, earlierFailures: earlier }), []);
  });
});
