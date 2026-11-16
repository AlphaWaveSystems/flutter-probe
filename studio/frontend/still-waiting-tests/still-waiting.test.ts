import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { formatElapsed, progressText, signalNote, STILL_WAITING_AFTER_MS, SHOW_ELAPSED_AFTER_MS, waitingLine } from "../src/still-waiting.ts";

describe("formatElapsed", () => {
  it("reads as a person says it", () => {
    assert.equal(formatElapsed(0), "0 s");
    assert.equal(formatElapsed(4999), "4 s");
    assert.equal(formatElapsed(65_000), "1 min 5 s");
    assert.equal(formatElapsed(120_000), "2 min");
    assert.equal(formatElapsed(-5), "0 s");
  });
});

describe("progressText", () => {
  it("shows the step count alone for a step that has just started", () => {
    assert.equal(progressText(1, 5, null), "step 2 of 5");
    assert.equal(progressText(1, 5, SHOW_ELAPSED_AFTER_MS - 1), "step 2 of 5");
    assert.equal(progressText(0, 0, 100), "step 1");
  });
  it("adds the elapsed time once the step has been going for a few seconds", () => {
    assert.equal(progressText(1, 5, SHOW_ELAPSED_AFTER_MS), "step 2 of 5 · 3 s");
    assert.equal(progressText(4, 5, 12_000), "step 5 of 5 · 12 s");
    assert.equal(progressText(9, 5, 4000), "step 5 of 5 · 4 s"); // never past the last step
  });
});

describe("waitingLine", () => {
  it("is empty before ten seconds and when no step is running", () => {
    assert.equal(waitingLine('tap "Pay"', 9_999, false), ""); // the literal: the constant alone would move with a mutation of itself
    assert.equal(waitingLine('tap "Pay"', null, false), "");
  });
  it("says Studio is still waiting for the step, and that Cancel stops the run", () => {
    assert.equal(waitingLine('tap "Pay"', 10_000, false), 'Still waiting for tap "Pay": Cancel stops the run.');
    assert.equal(STILL_WAITING_AFTER_MS, 10_000);
    assert.equal(waitingLine("", 20_000, false), "Still waiting for the current step: Cancel stops the run.");
  });
  it("says when the device has stopped sending its screen", () => {
    assert.match(waitingLine('tap "Pay"', 15_000, true), /the device has stopped sending its screen\. Cancel stops the run\.$/);
  });
  it("does not change as the seconds pass (a screen reader hears it once)", () => {
    assert.equal(waitingLine("x", 11_000, false), waitingLine("x", 55_000, false));
  });
});

describe("signalNote", () => {
  it("says no signal only when the device sends nothing", () => {
    assert.equal(signalNote(true), "no signal");
    assert.equal(signalNote(false), "");
  });
});
