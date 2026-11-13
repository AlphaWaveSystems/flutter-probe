import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { latestWins } from "../src/latest-wins.ts";

/** A promise that settles when the test says so. */
function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe("latestWins", () => {
  it("shows the latest result even when an older run finishes after it", async () => {
    const first = deferred<string>();
    const second = deferred<string>();
    const queue = [first, second];
    const shown: string[] = [];
    const run = latestWins(() => queue.shift()!.promise, (v) => shown.push(v), () => shown.push("failed"));

    const a = run(); // starts first, finishes last
    const b = run(); // starts second, finishes first
    second.resolve("new");
    await b;
    first.resolve("old");
    await a;
    assert.deepEqual(shown, ["new"]);
  });

  it("shows each result when the runs do not overlap", async () => {
    const results = ["one", "two"];
    const shown: string[] = [];
    const run = latestWins(async () => results.shift()!, (v) => shown.push(v), () => shown.push("failed"));
    await run();
    await run();
    assert.deepEqual(shown, ["one", "two"]);
  });

  it("shows the failure of the latest run only", async () => {
    const first = deferred<string>();
    const second = deferred<string>();
    const queue = [first, second];
    const shown: string[] = [];
    const run = latestWins(() => queue.shift()!.promise, (v) => shown.push(v), () => shown.push("failed"));

    const a = run();
    const b = run();
    first.reject(new Error("old run failed")); // an older failure must not blank a newer result
    await a;
    second.resolve("new");
    await b;
    assert.deepEqual(shown, ["new"]);
  });

  it("shows a failure when the latest run fails", async () => {
    const shown: string[] = [];
    const run = latestWins(async () => {
      throw new Error("no");
    }, (v) => shown.push(String(v)), () => shown.push("failed"));
    await run();
    assert.deepEqual(shown, ["failed"]);
  });
});
