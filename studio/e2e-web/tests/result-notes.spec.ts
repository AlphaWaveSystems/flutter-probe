import { expect, Page, test } from "@playwright/test";
import { openStudio, workspace } from "./helpers";

// The notes under a failed test from what the engine reports about it: why the app died, that the test timed out, and how to find out why the app
// died when no evidence was kept. The `run:result` events a real run fires are fired by hand (the handlers the page registered), so no device is
// needed. Every note is text.

async function captureEvents(page: Page) {
  await page.addInitScript(() => {
    const handlers: Record<string, Array<(...a: unknown[]) => void>> = {};
    (window as any).__handlers = handlers;
    let rt: any;
    Object.defineProperty(window, "runtime", {
      configurable: true,
      get: () => rt,
      set: (v) => {
        rt = v;
        const orig = v.EventsOnMultiple.bind(v);
        v.EventsOnMultiple = (name: string, cb: (...a: unknown[]) => void, max: number) => {
          (handlers[name] = handlers[name] || []).push(cb);
          return orig(name, cb, max);
        };
      },
    });
  });
}

const fire = (page: Page, name: string, ...args: unknown[]) =>
  page.evaluate(([n, a]) => (((window as any).__handlers[n as string]) || []).forEach((cb: (...x: unknown[]) => void) => cb(...(a as unknown[]))), [name, args] as const);

type Result = Record<string, unknown>;
const result = (extra: Result): Result => ({ name: "t", file: "probe-tests/a.probe", passed: false, skipped: false, durationMs: 5, error: "rpc error: connection closed", ...extra });

async function runWith(page: Page, ...results: Result[]) {
  await fire(page, "run:started", "probe-tests/a.probe");
  for (const r of results) await fire(page, "run:result", r);
  await fire(page, "run:finished", []);
}

const notes = (page: Page) => page.locator("#results-list li.note");

test.describe("Result notes", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");
  test.beforeEach(async ({ page }) => {
    await captureEvents(page);
    await openStudio(page);
  });

  test("rn-01 a failed test with crash evidence shows the one line why the app died, under its error", async ({ page }) => {
    await runWith(page, result({ crash: "the app died during this test: OutOfMemoryError" }));
    await expect(notes(page)).toHaveCount(1);
    await expect(notes(page).first()).toHaveClass(/note-crash/);
    await expect(notes(page).first()).toHaveText("the app died during this test: OutOfMemoryError");
    // it follows the error row of the same test
    expect(await page.locator("#results-list li.err + li.note-crash").count()).toBe(1);
  });

  test("rn-02 a test that timed out says so and names the limit and where to change it", async ({ page }) => {
    await runWith(page, result({ error: "test exceeded 1m30s", timedOut: true, timeLimitSeconds: 90 }));
    await expect(notes(page)).toHaveCount(1);
    await expect(notes(page).first()).toHaveClass(/note-timeout/);
    await expect(notes(page).first()).toContainText("Timed out: the test ran past its 1 min 30 s limit");
    await expect(notes(page).first()).toContainText("defaults.test_timeout");
  });

  test("rn-03 a lost connection on Android with evidence off says how to turn evidence on; with a crash line there is no hint", async ({ page }) => {
    await runWith(page, result({ name: "no evidence", crashHint: true }), result({ name: "with evidence", crashHint: true, crash: "the app died during this test: x" }));
    await expect(page.locator("#results-list li.note-hint")).toHaveCount(1);
    await expect(page.locator("#results-list li.note-hint")).toContainText("defaults.crash_evidence: true");
    await expect(page.locator("#results-list li.note-hint")).toContainText("Android");
    await expect(page.locator("#results-list li.note-crash")).toHaveCount(1);
  });

  test("rn-04 a test that passed or was skipped has no note, whatever the fields say", async ({ page }) => {
    await runWith(page, result({ passed: true, error: "", crash: "c", timedOut: true, timeLimitSeconds: 5, crashHint: true }), result({ name: "s", skipped: true, error: "", crash: "c" }));
    await expect(notes(page)).toHaveCount(0);
  });

  test("rn-05 a note is text: markup in the crash line is not interpreted", async ({ page }) => {
    await runWith(page, result({ crash: "the app died during this test: <img src=x onerror=alert(1)><b>x</b>" }));
    await expect(notes(page).first()).toContainText("<img src=x onerror=alert(1)>");
    expect(await page.locator("#results-list li.note img, #results-list li.note b").count()).toBe(0);
  });

  test("rn-07 a test that passed on a later attempt says so, that it is flaky, and lists the earlier failures under it", async ({ page }) => {
    await runWith(page, result({ passed: true, error: "", attempts: 3, earlierFailures: [{ attempt: 1, error: "line 3: not found" }, { attempt: 2, error: "connection reset" }] }));
    await expect(page.locator("#results-list li.pass")).toHaveCount(1);
    await expect(page.locator("#results-list li.note-retry")).toHaveText(/Passed on attempt 3, after 2 failed attempts .*flaky/);
    await expect(page.locator("#results-list li.note-attempt")).toHaveCount(2);
    await expect(page.locator("#results-list li.note-attempt").first()).toHaveText("Attempt 1 failed: line 3: not found");
    expect(await page.locator("#results-list li.pass + li.note-retry + li.note-attempt + li.note-attempt").count()).toBe(1); // in order, under the pass row
  });

  test("rn-08 a test that failed every attempt says so once, with the earlier attempts, after its error", async ({ page }) => {
    await runWith(page, result({ attempts: 2, earlierFailures: [{ attempt: 1, error: "first" }] }));
    await expect(page.locator("#results-list li.note-retry")).toHaveText("Failed on all 2 attempts (defaults.retry_failed_tests re-ran it).");
    await expect(page.locator("#results-list li.note-attempt")).toHaveText("Attempt 1 failed: first");
    expect(await page.locator("#results-list li.err + li.note-retry + li.note-attempt").count()).toBe(1);
  });

  test("rn-09 a test that ran once has no retry note, and an earlier failure is text", async ({ page }) => {
    await runWith(page, result({ passed: true, error: "", attempts: 1 }), result({ name: "markup", attempts: 2, earlierFailures: [{ attempt: 1, error: "<img src=x onerror=alert(1)>" }] }));
    await expect(page.locator("#results-list li.note-retry")).toHaveCount(1); // only the second test
    await expect(page.locator("#results-list li.note-attempt")).toContainText("<img src=x onerror=alert(1)>");
    expect(await page.locator("#results-list li.note img").count()).toBe(0);
  });

  test("rn-06 notes stay with their test when failures are listed first", async ({ page }) => {
    await runWith(page, result({ name: "passes", passed: true, error: "" }), result({ name: "fails", crash: "the app died during this test: z" }));
    const order = await page.locator("#results-list > li").evaluateAll((els) => els.map((e) => e.className));
    const crashAt = order.findIndex((c) => c.includes("note-crash"));
    const errAt = order.findIndex((c) => c.includes("err"));
    expect(crashAt).toBeGreaterThan(errAt);
    expect(order.findIndex((c) => c.includes("pass"))).toBeGreaterThan(crashAt); // the failure's block comes first, with its note
  });
});
