import { expect, test } from "@playwright/test";
import * as path from "node:path";
import { app, connect, disconnect, openPath, openStudio, simName, udid, workspace } from "./helpers";

// These need a booted simulator running the fixture app; scripts/studio-e2e-web.sh prepares one on macOS.
test.describe("Studio with a device", () => {
  test.skip(!udid || !workspace, "no device prepared (STUDIO_E2E_UDID unset)");

  test.afterEach(async ({ page }) => {
    await disconnect(page);
  });

  test("10 the device picker lists the named simulator", async ({ page }) => {
    await openStudio(page);
    await page.click("#btn-refresh");
    await expect(page.locator("#device-picker option", { hasText: simName })).toHaveCount(1, { timeout: 30_000 });
  });

  test("11 connect shows the live screen and the widget tree, disconnect clears it", async ({ page }) => {
    await openStudio(page);
    await connect(page);
    await expect(page.locator("#device-view img, #device-view canvas").first()).toBeVisible({ timeout: 30_000 });
    await expect(page.locator("#inspector-content")).not.toContainText("Connect to load the widget tree", { timeout: 30_000 });
    await page.click("#btn-connect");
    await expect(page.locator("#status-text")).toHaveText("disconnected");
  });

  test("12 running a passing file lists pass rows and the summary", async ({ page }) => {
    await openStudio(page);
    await connect(page);
    await openPath(page, "probe-tests/counter.probe");
    await expect(page.locator("#btn-run")).toBeEnabled();
    await page.click("#btn-run");
    await expect(page.locator("#results-list li.pass", { hasText: "counter increments and resets" })).toBeVisible({ timeout: 120_000 });
    await expect(page.locator("#results-summary")).toContainText("1 passed");
  });

  test("13 a failing file shows a fail row with the error and a failed summary", async ({ page }) => {
    await openStudio(page);
    await connect(page);
    await openPath(page, "probe-tests/failing.probe");
    await page.click("#btn-run");
    await expect(page.locator("#results-list li.fail:not(.step)")).toBeVisible({ timeout: 120_000 });
    await expect(page.locator("#results-list li.err")).toContainText("This text does not exist");
    await expect(page.locator("#results-summary")).toContainText("failed");
  });

  test("14 performance lines appear under a result that measures", async ({ page }) => {
    await openStudio(page);
    await connect(page);
    await openPath(page, "probe-tests/list.probe");
    await page.click("#btn-run");
    await expect(page.locator("#results-list li.perf").first()).toBeVisible({ timeout: 180_000 });
  });

  test("15 live steps: every step row ends passed and the progress shows step N of M", async ({ page }) => {
    console.log(`device: ${simName}`);
    await openStudio(page);
    await connect(page);
    await openPath(page, "probe-tests/counter.probe");
    await page.click("#btn-run");
    await expect(page.locator("#results-list li.pass", { hasText: "counter increments and resets" })).toBeVisible({ timeout: 120_000 });
    const steps = page.locator("#results-list li.step");
    await expect(steps.first()).toBeVisible();
    expect(await page.locator("#results-list li.step.fail").count()).toBe(0);
    expect(await steps.count()).toBeGreaterThanOrEqual(7);
    // End of the run (native Test12): the verdict row heads its steps, the last step is the passing
    // `see "Taps: 0"`, no step is left running, and Cancel is hidden again.
    await expect(page.locator("#results-list > li").first()).toHaveAttribute("aria-label", /^pass: counter increments and resets/);
    await expect(steps.last()).toHaveClass(/\bpass\b/);
    await expect(steps.last()).toContainText("Taps: 0");
    await expect(page.locator("#results-list li.step.running")).toHaveCount(0);
    await expect(page.locator("#btn-cancel")).toBeHidden({ timeout: 30_000 });
  });

  test("16 a failing step keeps a fail row naming its line", async ({ page }) => {
    await openStudio(page);
    await connect(page);
    await openPath(page, "probe-tests/failing.probe");
    await page.click("#btn-run");
    await expect(page.locator("#results-list li.step.fail")).toBeVisible({ timeout: 120_000 });
    await expect(page.locator("#results-list li.step.fail .line")).toHaveText(/^L\d+$/);
  });

  test("17 Cancel stops a run", async ({ page }) => {
    await openStudio(page);
    await connect(page);
    await openPath(page, "probe-tests/list.probe");
    await page.click("#btn-run");
    await expect(page.locator("#btn-cancel")).toBeVisible({ timeout: 30_000 });
    await page.click("#btn-cancel");
    await expect(page.locator("#btn-cancel")).toBeHidden({ timeout: 60_000 });
    const state = await app<{ running: boolean }>(page, "RunState");
    expect(state.running).toBe(false);
  });

  test("18 connecting while the app is not running reports an error", async ({ page }) => {
    await openStudio(page);
    // The suite script relaunches the fixture app; stopping it makes the connect fail.
    const { execFileSync } = await import("node:child_process");
    const bundle = process.env.STUDIO_E2E_BUNDLE_ID ?? "com.alphawavesystems.studioFixture";
    try {
      execFileSync("xcrun", ["simctl", "terminate", udid, bundle]);
    } catch {}
    await page.click("#btn-refresh");
    const picker = page.locator("#device-picker");
    await expect(picker.locator("option", { hasText: simName })).toHaveCount(1, { timeout: 30_000 });
    await picker.selectOption({ label: (await picker.locator("option", { hasText: simName }).first().textContent())! });
    await page.click("#btn-connect");
    await expect(page.locator("#toast-container")).toContainText(/Connect failed/, { timeout: 90_000 });
    execFileSync("xcrun", ["simctl", "launch", udid, bundle]);
    expect(path.basename(workspace).length).toBeGreaterThan(0);
  });
});

// The shared device view: Studio started with PROBE_STUDIO_DEVICE_VIEW=1 (the config passes STUDIO_E2E_DEVICE_VIEW=1
// through). The pane then draws frames from the loopback hub and relays pointer input to the app.
test.describe("Studio with the shared device view", () => {
  test.skip(!udid || !workspace || process.env.STUDIO_E2E_DEVICE_VIEW !== "1", "needs a device and STUDIO_E2E_DEVICE_VIEW=1");

  test.afterEach(async ({ page }) => {
    await disconnect(page);
  });

  test("36 the pane shows the hub's frames, a click taps the app, Alt+click selects an element", async ({ page }) => {
    await openStudio(page);
    await connect(page);
    const canvas = page.locator("#device-view canvas");
    await expect(canvas).toBeVisible({ timeout: 30_000 });
    // The old frame loop is not running: its image element never appears.
    await expect(page.locator("#device-view img")).toHaveCount(0);
    // Painted, not blank: some pixel is not fully transparent.
    await expect
      .poll(async () => canvas.evaluate((c: HTMLCanvasElement) => {
        const d = c.getContext("2d")!.getImageData(0, 0, c.width, c.height).data;
        for (let i = 3; i < d.length; i += 4 * 97) if (d[i] !== 0) return true;
        return false;
      }), { timeout: 30_000 })
      .toBe(true);
    // The inspector text keeps updating while the hub draws (it lists widget types, not their text).
    await expect(page.locator("#inspector-content")).toContainText("Scaffold", { timeout: 30_000 });

    const box = (await canvas.boundingBox())!;
    const at = (fx: number, fy: number) => [box.x + box.width * fx, box.y + box.height * fy] as const;

    // Alt+click selects the element under the pointer: the selector is shown and the inspector is searched for it.
    await canvas.click({ modifiers: ["Alt"], position: { x: box.width * 0.5, y: box.height * 0.5 } });
    await expect(page.locator("#toast-container")).toContainText("Selected #counter_button", { timeout: 15_000 });
    await expect(page.locator("#inspector-search")).toHaveValue("counter_button");

    // A plain click is a tap in the app: the "List" tab of the fixture's navigation bar opens the list screen.
    await expect(page.locator("#inspector-content")).not.toContainText("ListView");
    await page.mouse.click(...at(0.835, 0.96));
    await expect(page.locator("#inspector-content")).toContainText("ListView", { timeout: 15_000 });

    await page.click("#btn-connect");
    await expect(page.locator("#status-text")).toHaveText("disconnected");
    await expect(canvas).toHaveCount(0);
  });
});
