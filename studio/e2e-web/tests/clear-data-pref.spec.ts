import { expect, Page, test } from "@playwright/test";
import { app, openStudio, workspace } from "./helpers";

// The person's setting "run clear app data on simulators and emulators without asking": a box in Settings, saved the moment it is pressed, kept
// in the person's own preferences file (never in probe.yaml), off by default. The enforcement (a simulator is not asked, anything else is) is
// covered by the Go tests; here the box.

async function openSettings(page: Page) {
  await openStudio(page);
  await page.click("#btn-settings");
  await expect(page.locator("#settings-overlay")).toBeVisible();
  await expect(page.locator("#settings-port")).toHaveValue(/^\d+$/);
}

test.describe("Clear app data without asking (simulators and emulators)", () => {
  test.skip(!workspace, "STUDIO_WORKSPACE is not set (run scripts/studio-e2e-web.sh)");
  test.afterEach(async ({ page }) => {
    await app(page, "SetClearDataWithoutAsking", false).catch(() => undefined); // leave the run's preferences as they were found
  });
  const box = (page: Page) => page.locator("#settings-clear-data-sim");

  test("cp-01 the box is off by default, says what it does, and a project's probe.yaml is not where it lives", async ({ page }) => {
    await openSettings(page);
    await expect(box(page)).not.toBeChecked();
    await expect(page.locator("#settings-studio-prefs")).toContainText("always ask");
    await expect(page.locator("#settings-studio-prefs")).toContainText("a probe.yaml cannot change it");
    expect(await app<{ clearDataWithoutAsking: boolean }>(page, "GetStudioPrefs")).toEqual({ clearDataWithoutAsking: false });
  });

  test("cp-02 pressing the box saves it at once, and it is still on when Settings is reopened; pressing it again turns it off", async ({ page }) => {
    await openSettings(page);
    await box(page).check();
    await expect(page.locator("#toast-container .toast", { hasText: "runs without asking on simulators and emulators" })).toBeVisible();
    expect(await app<{ clearDataWithoutAsking: boolean }>(page, "GetStudioPrefs")).toEqual({ clearDataWithoutAsking: true });
    await page.click("#btn-settings-close");
    await page.click("#btn-settings");
    await expect(box(page)).toBeChecked();
    await box(page).uncheck();
    await expect(page.locator("#toast-container .toast", { hasText: "asks again" })).toBeVisible();
    expect(await app<{ clearDataWithoutAsking: boolean }>(page, "GetStudioPrefs")).toEqual({ clearDataWithoutAsking: false });
  });
});
