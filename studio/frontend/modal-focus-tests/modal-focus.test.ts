import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { nextFocusIndex } from "../src/modal-focus.ts";

describe("nextFocusIndex", () => {
  it("goes forward and wraps from the last to the first", () => {
    assert.equal(nextFocusIndex(3, 0, false), 1);
    assert.equal(nextFocusIndex(3, 1, false), 2);
    assert.equal(nextFocusIndex(3, 2, false), 0);
  });
  it("goes backwards and wraps from the first to the last", () => {
    assert.equal(nextFocusIndex(3, 2, true), 1);
    assert.equal(nextFocusIndex(3, 1, true), 0);
    assert.equal(nextFocusIndex(3, 0, true), 2);
  });
  it("brings the focus in from outside: the first stop going forward, the last going backwards", () => {
    assert.equal(nextFocusIndex(3, -1, false), 0);
    assert.equal(nextFocusIndex(3, -1, true), 2);
  });
  it("stays on the only stop", () => {
    assert.equal(nextFocusIndex(1, 0, false), 0);
    assert.equal(nextFocusIndex(1, 0, true), 0);
  });
  it("has nowhere to go when there is no stop", () => {
    assert.equal(nextFocusIndex(0, -1, false), -1);
    assert.equal(nextFocusIndex(0, 0, true), -1);
  });
});
