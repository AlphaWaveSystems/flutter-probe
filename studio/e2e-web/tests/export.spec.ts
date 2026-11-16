import { expect, Page, test } from "@playwright/test";
import { openStudio, workspace } from "./helpers";

// The Export buttons of the Results pane. The save dialog is native, so over the headless bridge a click is refused with a reason, and the
// file part (the engine's report, the path checks, the atomic write) is covered by the Go tests. Here: when the buttons are on, and what a
// click shows. The `run:*` events are fired by hand.

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

const result = { name: "t", file: "probe-tests/a.probe", passed: true, skipped: false, durationMs: 10, error: "" };

test.describe("Export of run results", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");
  test.beforeEach(async ({ page }) => {
    await captureEvents(page);
    await openStudio(page);
  });
  const json = (page: Page) => page.locator("#btn-export-json");
  const junit = (page: Page) => page.locator("#btn-export-junit");

  test("xr-01 both buttons are off before a run, on after a run with a result, and off again while the next run goes", async ({ page }) => {
    await expect(json(page)).toBeDisabled();
    await expect(junit(page)).toBeDisabled();
    await fire(page, "run:started", "probe-tests/a.probe");
    await fire(page, "run:finished", [result]);
    await expect(json(page)).toBeEnabled();
    await expect(junit(page)).toBeEnabled();
    await fire(page, "run:started", "probe-tests/a.probe");
    await expect(json(page)).toBeDisabled();
    await expect(junit(page)).toBeDisabled();
    await fire(page, "run:finished", []); // a run that ended with no result leaves nothing to export
    await expect(json(page)).toBeDisabled();
  });

  test("xr-02 without the native window a click says so, writes nothing, and the buttons follow what Studio holds (nothing), not what the run event said", async ({ page }) => {
    await fire(page, "run:started", "probe-tests/a.probe");
    await fire(page, "run:finished", [result]); // the page believes there is a report; Studio, which ran nothing, holds none
    await expect(json(page)).toBeEnabled();
    await json(page).click();
    await expect(page.locator("#toast-container .toast", { hasText: "Export failed" })).toBeVisible();
    await expect(json(page)).toBeDisabled();
    await expect(junit(page)).toBeDisabled();
  });

  test("xr-03 the buttons say where the report goes and that the person chooses", async ({ page }) => {
    await expect(json(page)).toHaveAttribute("title", /You choose where/);
    await expect(junit(page)).toHaveAttribute("title", /probe test --format junit/);
  });
});
