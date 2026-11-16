import { expect, Page, test } from "@playwright/test";
import { openStudio, workspace } from "./helpers";

// What the toolbar says while a run goes on: how long the running step has been going, a "still waiting" line after ten seconds (Cancel stops the
// run), and "no signal" next to Cancel when the device sends no screen. The `run:*` and `device:*` events a real run fires are fired by hand and the
// page's clock is faked, so no device and no waiting are needed.

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

const step = (status: string, line = 3, description = 'tap "Pay"') => ({ file: "probe-tests/a.probe", test: "t", line, description, status, durationMs: 0, attempt: 0, depth: 0 });

async function startRun(page: Page) {
  await fire(page, "run:started", "probe-tests/a.probe");
  await fire(page, "run:plan", { tests: [{ name: "t", line: 1, steps: 5 }] });
}

test.describe("A run that takes long", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");
  test.beforeEach(async ({ page }) => {
    await captureEvents(page);
    await page.clock.install({ time: new Date("2026-01-01T00:00:00Z") });
    await openStudio(page);
    await page.clock.pauseAt(new Date("2026-01-01T00:00:10Z")); // from here only runFor moves the page's time
  });
  const progress = (page: Page) => page.locator("#run-progress");
  const waiting = (page: Page) => page.locator("#run-waiting");

  test("sw-01 the elapsed time of the running step shows after a few seconds, and goes with the step", async ({ page }) => {
    await startRun(page);
    await fire(page, "run:step", step("started"));
    await page.clock.runFor(1000);
    await expect(progress(page)).toHaveText("step 1 of 5"); // no elapsed time yet
    await page.clock.runFor(4000);
    await expect(progress(page)).toHaveText("step 1 of 5 · 5 s");
    await fire(page, "run:step", step("passed"));
    await fire(page, "run:step", step("started", 4, 'tap "Next"'));
    await page.clock.runFor(1000);
    await expect(progress(page)).toHaveText("step 1 of 5"); // the next step starts from zero (the step count follows the open file's steps only)
  });

  test("sw-02 after ten seconds on one step the line says Studio is still waiting for it and that Cancel stops the run; before, nothing", async ({ page }) => {
    await startRun(page);
    await fire(page, "run:step", step("started"));
    await page.clock.runFor(9000);
    await expect(waiting(page)).toBeHidden();
    await page.clock.runFor(2000);
    await expect(waiting(page)).toBeVisible();
    await expect(waiting(page)).toHaveText('Still waiting for tap "Pay": Cancel stops the run.');
    await page.clock.runFor(30_000);
    await expect(waiting(page)).toHaveText('Still waiting for tap "Pay": Cancel stops the run.'); // it does not change as the seconds pass
  });

  test("sw-03 a step that finishes takes the waiting line with it, and so does the end of the run", async ({ page }) => {
    await startRun(page);
    await fire(page, "run:step", step("started"));
    await page.clock.runFor(12_000);
    await expect(waiting(page)).toBeVisible();
    await fire(page, "run:step", step("passed"));
    await expect(waiting(page)).toBeHidden();
    await fire(page, "run:step", step("started", 5, 'see "Done"'));
    await page.clock.runFor(11_000);
    await expect(waiting(page)).toContainText('see "Done"');
    await fire(page, "run:finished", []);
    await expect(waiting(page)).toBeHidden();
    await expect(page.locator("#run-signal")).toBeHidden();
  });

  test("sw-04 no signal from the device shows next to Cancel at once, and in the waiting line after ten seconds; frames bring it back", async ({ page }) => {
    await startRun(page);
    await fire(page, "run:step", step("started"));
    await fire(page, "device:stream-error", "screenshot: probelink: use of closed network connection");
    await expect(page.locator("#run-signal")).toHaveText("no signal");
    await expect(waiting(page)).toBeHidden(); // not yet ten seconds
    await page.clock.runFor(11_000);
    await expect(waiting(page)).toContainText("the device has stopped sending its screen. Cancel stops the run.");
    // a frame is the signal back (it needs the page connected to paint, so the event is the one the stream sends on success)
    await page.evaluate(() => document.dispatchEvent(new CustomEvent("studio:signal", { detail: { lost: false } })));
    await fire(page, "run:finished", []);
    await expect(page.locator("#run-signal")).toBeHidden();
  });

  test("sw-05 the lines are text: markup in a step's text is not interpreted", async ({ page }) => {
    await startRun(page);
    await fire(page, "run:step", step("started", 3, 'tap "<img src=x onerror=alert(1)>"'));
    await page.clock.runFor(11_000);
    await expect(waiting(page)).toContainText('<img src=x onerror=alert(1)>');
    expect(await waiting(page).evaluate((el) => el.children.length)).toBe(0);
  });

  test("sw-06 the progress text is not a live region (it changes every second); the waiting line is", async ({ page }) => {
    await expect(page.locator("#run-progress")).not.toHaveAttribute("aria-live", /.+/);
    await expect(page.locator("#run-waiting")).toHaveAttribute("aria-live", "polite");
    await expect(page.locator("#run-waiting")).toHaveAttribute("role", "status");
  });
});
