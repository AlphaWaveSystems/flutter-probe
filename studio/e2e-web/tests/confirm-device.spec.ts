import { expect, test } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { app, connect, disconnect, openStudio, simName, udid, workspace } from "./helpers";

// `clear app data` in a Studio run, on a real, named simulator: the window asks, naming the app and the device; Cancel stops the step with a
// refusal and the app's data stays; a yes runs it, and the next `clear app data` of the same run asks again.

test.describe("Clear app data asks in the window", () => {
  test.skip(!udid || !workspace, "no simulator (run scripts/studio-e2e-web.sh without STUDIO_E2E_NO_DEVICE)");

  // A run that stops half way leaves the counter of the fixture app at 1, and the device specs after this one start from 0: put the app back.
  test.afterEach(() => {
    const bundle = process.env.STUDIO_E2E_BUNDLE_ID ?? "";
    if (!bundle) return;
    try {
      execFileSync("xcrun", ["simctl", "terminate", udid, bundle], { stdio: "ignore" });
    } catch {
      /* not running */
    }
    execFileSync("xcrun", ["simctl", "launch", udid, bundle], { stdio: "ignore" });
  });

  test("cd-10 Cancel refuses the step; a yes runs it; the second step asks again", async ({ page }) => {
    test.info().annotations.push({ type: "device", description: simName });
    await openStudio(page);
    await connect(page);
    await expect(page.locator("#status-text")).toContainText(simName);
    await page.locator("#file-list li", { hasText: "probe-tests" }).first().click();
    await page.locator("#file-list li", { hasText: "clear-data.probe" }).first().click();
    await expect(page.locator("#editor-filename")).toContainText("clear-data.probe");

    // run 1: Cancel
    await page.click("#btn-run");
    await expect(page.locator("#confirm-overlay")).toBeVisible({ timeout: 60_000 });
    await expect(page.locator("#confirm-app")).toHaveText(/\./); // the app id of the workspace
    await expect(page.locator("#confirm-device")).toContainText(simName);
    await expect(page.locator("#btn-confirm-cancel")).toBeFocused();
    await page.click("#btn-confirm-cancel");
    await expect(page.locator("#results-list li.fail", { hasText: "clear app data asks first" })).toBeVisible({ timeout: 60_000 });
    await expect(page.locator("#results-list")).toContainText("clear app data");
    await expect(page.locator("#results-list")).not.toContainText("--yes");
    await expect(page.locator("#btn-run")).toBeEnabled({ timeout: 60_000 });

    // run 2: a yes for each of the two steps (each asks)
    await page.click("#btn-run");
    for (const n of [1, 2]) {
      await expect(page.locator("#confirm-overlay"), `question ${n}`).toBeVisible({ timeout: 90_000 });
      await page.click("#btn-confirm-danger");
      await expect(page.locator("#confirm-overlay")).toBeHidden();
    }
    await expect(page.locator("#results-list li.pass", { hasText: "clear app data asks first" })).toBeVisible({ timeout: 120_000 });
    await expect(page.locator("#btn-run")).toBeEnabled({ timeout: 60_000 });
    await disconnect(page);
  });

  test("cd-11 cancelling the run while the question is open closes the question and runs nothing", async ({ page }) => {
    test.info().annotations.push({ type: "device", description: simName });
    await openStudio(page);
    await connect(page);
    await page.locator("#file-list li", { hasText: "probe-tests" }).first().click();
    await page.locator("#file-list li", { hasText: "clear-data.probe" }).first().click();
    await page.click("#btn-run");
    await expect(page.locator("#confirm-overlay")).toBeVisible({ timeout: 60_000 });
    await app(page, "CancelRun"); // the dialog covers the toolbar, so the run is cancelled the way a script or the menu would
    await expect(page.locator("#confirm-overlay")).toBeHidden({ timeout: 15_000 });
    await expect(page.locator("#toast-container .toast", { hasText: "Clear app data was not run" })).toBeVisible();
    await expect(page.locator("#btn-run")).toBeEnabled({ timeout: 60_000 });
    await disconnect(page);
  });
});
