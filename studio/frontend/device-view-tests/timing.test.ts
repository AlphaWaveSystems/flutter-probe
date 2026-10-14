import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { LatestSlot, RateGate, TextBatcher, backoffDelay } from "../src/device-view/timing.ts";
import { FakeTimers } from "./helpers.ts";

describe("backoffDelay", () => {
  it("doubles from 500 ms and caps at 8 s", () => {
    assert.deepEqual([0, 1, 2, 3, 4, 5, 6, 50].map(backoffDelay), [500, 1000, 2000, 4000, 8000, 8000, 8000, 8000]);
  });
  it("treats negative or fractional attempts safely", () => {
    assert.equal(backoffDelay(-3), 500);
    assert.equal(backoffDelay(1.9), 1000);
  });
});

describe("RateGate (hover throttle, 10/s)", () => {
  it("allows at most one call per interval", () => {
    let t = 1000;
    const g = new RateGate(100, () => t);
    assert.equal(g.tryAcquire(), true);
    t += 50;
    assert.equal(g.tryAcquire(), false);
    t += 50;
    assert.equal(g.tryAcquire(), true);
    let allowed = 0;
    for (let i = 0; i < 200; i++) {
      t += 5; // 200 events over one second
      if (g.tryAcquire()) allowed++;
    }
    assert.ok(allowed <= 10, `allowed ${allowed}`);
    assert.ok(allowed >= 9);
  });
});

describe("TextBatcher", () => {
  it("batches pushes within the interval into one flush", () => {
    const tm = new FakeTimers();
    const out: string[] = [];
    const b = new TextBatcher((s) => out.push(s), 50, tm);
    b.push("h");
    b.push("e");
    b.push("llo");
    assert.deepEqual(tm.delays(), [50]);
    assert.deepEqual(out, []);
    tm.runAll();
    assert.deepEqual(out, ["hello"]);
    b.push("!");
    tm.runAll();
    assert.deepEqual(out, ["hello", "!"]);
  });
  it("flush() sends immediately and cancels the timer; cancel() discards", () => {
    const tm = new FakeTimers();
    const out: string[] = [];
    const b = new TextBatcher((s) => out.push(s), 50, tm);
    b.push("x");
    b.flush();
    assert.deepEqual(out, ["x"]);
    assert.equal(tm.pending.size, 0);
    b.push("y");
    b.cancel();
    tm.runAll();
    assert.deepEqual(out, ["x"]);
  });
  it("ignores empty pushes", () => {
    const tm = new FakeTimers();
    const b = new TextBatcher(() => assert.fail("no flush expected"), 50, tm);
    b.push("");
    assert.equal(tm.pending.size, 0);
  });
});

describe("LatestSlot", () => {
  it("keeps only the newest offered value and reports the dropped one", () => {
    const s = new LatestSlot<number>();
    assert.equal(s.offer(1), undefined);
    assert.equal(s.offer(2), 1);
    assert.equal(s.offer(3), 2);
    assert.equal(s.take(), 3);
    assert.equal(s.take(), undefined);
  });
  it("clear() empties the slot", () => {
    const s = new LatestSlot<number>();
    s.offer(1);
    s.clear();
    assert.equal(s.take(), undefined);
  });
});
